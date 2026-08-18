package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireLANOrAuth(t *testing.T) {
	// Without sessions: WAN blocked, LAN allowed.
	called := false
	handler := (&Server{}).requireLANOrAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// LAN: no Cf-Connecting-Ip
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	handler(rr, req)
	if rr.Code != http.StatusOK || !called {
		t.Fatalf("LAN request should pass: code=%d called=%v", rr.Code, called)
	}

	// WAN: Cf-Connecting-Ip present, no session
	called = false
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.Header.Set("Cf-Connecting-Ip", "1.2.3.4")
	handler(rr, req)
	if rr.Code != http.StatusForbidden || called {
		t.Fatalf("WAN request without session should be blocked: code=%d called=%v", rr.Code, called)
	}
}

func TestRequireLANOrAuthForWrites(t *testing.T) {
	handler := (&Server{}).requireLANOrAuthForWrites(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// WAN GET allowed
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil)
	req.Header.Set("Cf-Connecting-Ip", "1.2.3.4")
	handler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("WAN GET should pass: code=%d", rr.Code)
	}

	// WAN POST blocked
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/favorites", nil)
	req.Header.Set("Cf-Connecting-Ip", "1.2.3.4")
	handler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("WAN POST should be blocked: code=%d", rr.Code)
	}
}
