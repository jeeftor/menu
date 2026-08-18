package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionManager(t *testing.T) {
	sm, err := NewSessionManager("")
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	sess := Session{Subject: "abc123", Email: "test@example.com", Name: "Test User"}
	cookie, err := sm.CreateSessionCookie(sess, time.Hour)
	if err != nil {
		t.Fatalf("CreateSessionCookie error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	got, err := sm.SessionFromRequest(req)
	if err != nil {
		t.Fatalf("SessionFromRequest error: %v", err)
	}
	if got.Subject != sess.Subject || got.Email != sess.Email || got.Name != sess.Name {
		t.Fatalf("session mismatch: got %+v, want %+v", got, sess)
	}
}

func TestSessionManagerInvalid(t *testing.T) {
	sm, _ := NewSessionManager("")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-a-valid-token"})
	if _, err := sm.SessionFromRequest(req); err == nil {
		t.Fatal("expected error for invalid token")
	}
}
