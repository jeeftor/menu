package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// ── /api/v1/food-images ───────────────────────────────────────────────────────

func (s *Server) handleAPIFoodImages(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store not configured — start server with --data-dir", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		imgs, err := s.store.ListFoodImages()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, imgs)
	case http.MethodPost:
		var req struct {
			FoodName string `json:"food_name"`
			ImageURL string `json:"image_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FoodName == "" || req.ImageURL == "" {
			http.Error(w, "body must be JSON {food_name, image_url}", http.StatusBadRequest)
			return
		}
		if err := s.store.UpsertFoodImage(req.FoodName, req.ImageURL); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		foodName := r.URL.Query().Get("food_name")
		if foodName == "" {
			http.Error(w, "missing ?food_name=", http.StatusBadRequest)
			return
		}
		if err := s.store.DeleteFoodImage(foodName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── /api/v1/favorites ─────────────────────────────────────────────────────────

func (s *Server) handleAPIFavorites(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store not configured — start server with --data-dir", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		favs, err := s.store.ListFavorites(r.URL.Query().Get("school"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, favs)
	case http.MethodPost:
		var req struct {
			FoodName   string `json:"food_name"`
			SchoolSlug string `json:"school_slug"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FoodName == "" {
			http.Error(w, "body must be JSON {food_name, school_slug?}", http.StatusBadRequest)
			return
		}
		if err := s.store.AddFavorite(req.FoodName, req.SchoolSlug); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			http.Error(w, "missing or invalid ?id=", http.StatusBadRequest)
			return
		}
		if err := s.store.RemoveFavorite(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── /api/v1/section-includes ──────────────────────────────────────────────────

func (s *Server) handleAPISectionIncludes(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store not configured — start server with --data-dir", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.store.ListSectionIncludes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, items)
	case http.MethodPost:
		var req struct {
			SchoolSlug  string `json:"school_slug"`
			MealType    string `json:"meal_type"`
			SectionName string `json:"section_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.SectionName) == "" {
			http.Error(w, "body must be JSON {school_slug, meal_type, section_name}", http.StatusBadRequest)
			return
		}
		if err := s.store.AddSectionInclude(req.SchoolSlug, req.MealType, req.SectionName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			http.Error(w, "missing or invalid ?id=", http.StatusBadRequest)
			return
		}
		if err := s.store.DeleteSectionInclude(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── /api/v1/exclusions ────────────────────────────────────────────────────────

func (s *Server) handleAPIExclusions(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store not configured — start server with --data-dir", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		exs, err := s.store.ListExclusions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, exs)
	case http.MethodPost:
		var req struct {
			SchoolSlug string `json:"school_slug"`
			Pattern    string `json:"pattern"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Pattern == "" {
			http.Error(w, "body must be JSON {pattern, school_slug?}", http.StatusBadRequest)
			return
		}
		if err := s.store.AddExclusion(req.SchoolSlug, req.Pattern); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			http.Error(w, "missing or invalid ?id=", http.StatusBadRequest)
			return
		}
		if err := s.store.DeleteExclusion(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── /api/v1/missing-images ────────────────────────────────────────────────────

// handleAPIMissingImages scans cached menu JSON files and returns food names
// that have no API-provided image and no custom store override.
func (s *Server) handleAPIMissingImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	customCovered := make(map[string]bool)
	if s.store != nil {
		if imgs, err := s.store.ListFoodImages(); err == nil {
			for _, img := range imgs {
				customCovered[strings.ToLower(strings.TrimSpace(img.FoodName))] = true
			}
		}
	}
	missing, err := s.client.ScanMissingImages(customCovered)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if missing == nil {
		missing = []string{}
	}
	writeJSON(w, missing)
}
