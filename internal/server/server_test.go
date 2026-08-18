package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireLAN(t *testing.T) {
	called := false
	handler := (&Server{}).requireLAN(func(w http.ResponseWriter, r *http.Request) {
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

	// WAN: Cf-Connecting-Ip present
	called = false
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.Header.Set("Cf-Connecting-Ip", "1.2.3.4")
	handler(rr, req)
	if rr.Code != http.StatusForbidden || called {
		t.Fatalf("WAN request should be blocked: code=%d called=%v", rr.Code, called)
	}
}
