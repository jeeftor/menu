package server

import (
	"log/slog"
	"net/http"

	"menu/internal/alexa"
)

// newAlexaHandler builds the Alexa HTTP handler for this server.
func newAlexaHandler(s *Server, cfg *AlexaConfig) http.Handler {
	slog.Info("alexa: handler configured",
		"application_id", cfg.ApplicationID,
		"verify_requests", cfg.VerifyRequests,
		"default_school", cfg.DefaultSchool,
		"default_meal", cfg.DefaultMeal,
	)
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
