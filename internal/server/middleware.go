// Package server provides the HTTP server and calendar web UI.
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// clientIP returns the original client IP from trusted reverse proxies.
// It prefers Cloudflare's Cf-Connecting-Ip, then X-Real-Ip, then X-Forwarded-For,
// falling back to r.RemoteAddr.
func clientIP(r *http.Request) string {
	if cf := r.Header.Get("Cf-Connecting-Ip"); cf != "" {
		return cf
	}
	if real := r.Header.Get("X-Real-Ip"); real != "" {
		return real
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// We trust our edge (Cloudflare/Caddy); use the first value.
		if i := strings.Index(fwd, ","); i != -1 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "" {
		return host
	}
	return r.RemoteAddr
}

// hashIP returns a stable short hash of an IP for use as a rate-limit map key.
func hashIP(ip string) string {
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:])[:16]
}

// rateLimiter provides per-IP token-bucket rate limiting with automatic cleanup.
type rateLimiter struct {
	mu       sync.RWMutex
	visitors map[string]*rateLimitEntry
	limit    rate.Limit
	burst    int
	cleanup  time.Duration
}

type rateLimitEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newRateLimiter(every time.Duration, burst int) *rateLimiter {
	rl := &rateLimiter{
		visitors: make(map[string]*rateLimitEntry),
		limit:    rate.Every(every),
		burst:    burst,
		cleanup:  5 * time.Minute,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	ent, ok := rl.visitors[ip]
	if !ok {
		ent = &rateLimitEntry{limiter: rate.NewLimiter(rl.limit, rl.burst)}
		rl.visitors[ip] = ent
	}
	ent.lastSeen = time.Now()
	return ent.limiter.Allow()
}

func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, ent := range rl.visitors {
			if time.Since(ent.lastSeen) > rl.cleanup {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// rateLimitMiddleware enforces per-IP rate limits. Exceeded requests receive 429.
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := hashIP(clientIP(r))
		if !s.limiter.allow(ip) {
			http.Error(w, fmt.Sprintf("%d Too Many Requests", http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// strictRateLimitMiddleware applies a stricter per-IP limit for sensitive endpoints.
func (s *Server) strictRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}
		ip := hashIP(clientIP(r))
		if !s.authLimiter.allow(ip) {
			http.Error(w, fmt.Sprintf("%d Too Many Requests", http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds defensive HTTP response headers.
func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				// The server-rendered UI uses inline scripts and click handlers.
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: https:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// logMiddleware logs each HTTP request with method, path, status, duration and
// client information.
func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", rec.status,
			"duration", time.Since(start).String(),
			"client_ip", clientIP(r),
		}
		if ua := r.UserAgent(); ua != "" {
			attrs = append(attrs, "user_agent", ua)
		}
		if ref := r.Referer(); ref != "" {
			attrs = append(attrs, "referer", ref)
		}
		if rec.status >= 500 {
			slog.Error("request", attrs...)
		} else if rec.status >= 400 {
			slog.Warn("request", attrs...)
		} else {
			slog.Info("request", attrs...)
		}
	})
}

// statusRecorder captures the written status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}
