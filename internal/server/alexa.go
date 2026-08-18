package server

import (
	"net/http"

	"menu/internal/alexa"
)

// newAlexaHandler builds the Alexa HTTP handler for this server.
func newAlexaHandler(s *Server, cfg *AlexaConfig) http.Handler {
	return alexa.New(alexa.Config{
		ApplicationID:  cfg.ApplicationID,
		VerifyRequests: cfg.VerifyRequests,
		DefaultSchool:  cfg.DefaultSchool,
		DefaultMeal:    cfg.DefaultMeal,
		ResolveSummary: s.ResolveSummary,
	})
}

// handleAlexa delegates to the configured Alexa handler.
func (s *Server) handleAlexa(w http.ResponseWriter, r *http.Request) {
	if s.alexaHandler == nil {
		http.Error(w, "alexa endpoint not configured", http.StatusNotFound)
		return
	}
	s.alexaHandler.ServeHTTP(w, r)
}
