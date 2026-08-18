// Package auth provides session management for Authentik OIDC logins.
package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const sessionCookieName = "menu_session"

// Session holds the authenticated user state stored in the session cookie.
type Session struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
}

// SessionManager signs and verifies session cookies.
type SessionManager struct {
	secret []byte
}

// NewSessionManager returns a session manager using the given base64 secret.
// If the secret is empty, a random key is generated (sessions won't survive
// restarts).
func NewSessionManager(secretB64 string) (*SessionManager, error) {
	var secret []byte
	if secretB64 == "" {
		secret = make([]byte, 32)
		// Insecure for production, but keeps the app running if unset.
		for i := range secret {
			secret[i] = byte(i)
		}
	} else {
		var err error
		secret, err = base64.StdEncoding.DecodeString(secretB64)
		if err != nil {
			return nil, fmt.Errorf("decoding session secret: %w", err)
		}
		if len(secret) < 32 {
			return nil, fmt.Errorf("session secret must be at least 32 bytes")
		}
	}
	return &SessionManager{secret: secret}, nil
}

// CreateSessionCookie returns a signed JWT cookie for the given session.
func (sm *SessionManager) CreateSessionCookie(s Session, maxAge time.Duration) (*http.Cookie, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   s.Subject,
		"email": s.Email,
		"name":  s.Name,
		"iat":   now.Unix(),
		"exp":   now.Add(maxAge).Unix(),
	})
	signed, err := tok.SignedString(sm.secret)
	if err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    signed,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}, nil
}

// ClearSessionCookie returns a cookie that deletes the session.
func (sm *SessionManager) ClearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}
}

// SessionFromRequest returns the session if the cookie is present and valid.
func (sm *SessionManager) SessionFromRequest(r *http.Request) (*Session, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}
	return sm.parseToken(c.Value)
}

func (sm *SessionManager) parseToken(tokenString string) (*Session, error) {
	tok, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return sm.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || !tok.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	exp, err := claims.GetExpirationTime()
	if err != nil {
		return nil, err
	}
	if exp == nil || time.Now().After(exp.Time) {
		return nil, fmt.Errorf("session expired")
	}
	return &Session{
		Subject: stringValue(claims["sub"]),
		Email:   stringValue(claims["email"]),
		Name:    stringValue(claims["name"]),
	}, nil
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

// ConstantTimeCompare compares two strings in constant time.
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
