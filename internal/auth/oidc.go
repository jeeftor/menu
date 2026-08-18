package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig holds the Authentik/OpenID Connect provider settings.
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// OIDCProvider wraps the OIDC discovery and OAuth2 config.
type OIDCProvider struct {
	provider     *oidc.Provider
	config       oauth2.Config
	verifier     *oidc.IDTokenVerifier
	 sessions     *SessionManager
	stateTimeout time.Duration
}

// NewOIDCProvider initializes the OIDC provider and OAuth2 config.
// It returns an error if discovery fails.
func NewOIDCProvider(ctx context.Context, cfg OIDCConfig, sm *SessionManager) (*OIDCProvider, error) {
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}

	oauth2Config := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	return &OIDCProvider{
		provider:     provider,
		config:       oauth2Config,
		verifier:     verifier,
		sessions:     sm,
		stateTimeout: 5 * time.Minute,
	}, nil
}

// Enabled returns true when all required OIDC fields are configured.
func (o *OIDCProvider) Enabled() bool {
	return o != nil && o.provider != nil
}

// LoginHandler redirects the browser to the OIDC authorization endpoint.
func (o *OIDCProvider) LoginHandler(w http.ResponseWriter, r *http.Request) {
	state, err := randomState()
	if err != nil {
		http.Error(w, "failed to generate state", http.StatusInternalServerError)
		return
	}
	nonce, err := randomState()
	if err != nil {
		http.Error(w, "failed to generate nonce", http.StatusInternalServerError)
		return
	}

	// Store state + nonce in a short-lived cookie to prevent CSRF/fixation.
	stateData, _ := json.Marshal(map[string]string{
		"state": state,
		"nonce": nonce,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "menu_oidc_state",
		Value:    base64.URLEncoding.EncodeToString(stateData),
		Path:     "/callback",
		MaxAge:   int(o.stateTimeout.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})

	authURL := o.config.AuthCodeURL(state, oidc.Nonce(nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// CallbackHandler handles the OIDC callback, validates tokens, and creates a session.
func (o *OIDCProvider) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stateCookie, err := r.Cookie("menu_oidc_state")
	if err != nil {
		http.Error(w, "missing state cookie", http.StatusBadRequest)
		return
	}
	// Clear state cookie immediately.
	http.SetCookie(w, &http.Cookie{
		Name:     "menu_oidc_state",
		Value:    "",
		Path:     "/callback",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})

	var stateData struct {
		State string `json:"state"`
		Nonce string `json:"nonce"`
	}
	decoded, err := base64.URLEncoding.DecodeString(stateCookie.Value)
	if err != nil {
		http.Error(w, "invalid state cookie", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(decoded, &stateData); err != nil {
		http.Error(w, "invalid state cookie", http.StatusBadRequest)
		return
	}

	if r.URL.Query().Get("state") != stateData.State {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	token, err := o.config.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token", http.StatusInternalServerError)
		return
	}

	idToken, err := o.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Error(w, "invalid id_token", http.StatusUnauthorized)
		return
	}

	if idToken.Nonce != stateData.Nonce {
		http.Error(w, "nonce mismatch", http.StatusUnauthorized)
		return
	}

	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "failed to parse claims", http.StatusInternalServerError)
		return
	}

	session := Session{Subject: claims.Sub, Email: claims.Email, Name: claims.Name}
	cookie, err := o.sessions.CreateSessionCookie(session, 7*24*time.Hour)
	if err != nil {
		http.Error(w, "session creation failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, cookie)

	// Redirect back to the originally requested page or root.
	redirect := "/"
	if next := r.URL.Query().Get("next"); next != "" {
		redirect = next
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

// LogoutHandler clears the local session cookie.
func (o *OIDCProvider) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, o.sessions.ClearSessionCookie())
	http.Redirect(w, r, "/", http.StatusFound)
}

// EndSessionURL returns the Authentik end-session URL if available.
func (o *OIDCProvider) EndSessionURL() string {
	// The OIDC discovery document may contain an end_session_endpoint.
	var raw struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := o.provider.Claims(&raw); err == nil && raw.EndSessionEndpoint != "" {
		return raw.EndSessionEndpoint
	}
	return ""
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
