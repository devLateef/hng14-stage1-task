package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"name-profile-api/internal/model"
	"name-profile-api/internal/service"
)

// ProfileHandler handles HTTP requests for the profiles resource.
type ProfileHandler struct {
	service service.ProfileService
}

// NewProfileHandler returns a ProfileHandler backed by the given service.
func NewProfileHandler(svc service.ProfileService) *ProfileHandler {
	return &ProfileHandler{service: svc}
}

// writeJSON writes a JSON response with the given status code and body.
// It always sets Content-Type and the CORS header.
func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body) //nolint:errcheck
}

// writeError writes a standard error JSON response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, model.ErrorResponse{Status: "error", Message: message})
}

// CreateProfile handles POST /api/profiles.
func (h *ProfileHandler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name interface{} `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid request body")
		return
	}

	// Validate that name is a string (not int, bool, etc.)
	name, ok := req.Name.(string)
	if !ok {
		if req.Name == nil {
			// Missing name field decoded as nil
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "invalid request body")
		return
	}

	profile, err := h.service.CreateProfile(r.Context(), name)
	if err != nil {
		if errors.Is(err, model.ErrMissingName) {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if errors.Is(err, model.ErrAlreadyExists) {
			count := 1
			writeJSON(w, http.StatusOK, model.APIResponse{
				Status:  "success",
				Message: "Profile already exists",
				Data:    profile,
				Count:   &count,
			})
			return
		}
		var upErr *model.UpstreamError
		if errors.As(err, &upErr) {
			writeError(w, http.StatusBadGateway, upErr.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, model.APIResponse{
		Status: "success",
		Data:   profile,
	})
}

// GetProfile handles GET /api/profiles/{id}.
func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	profile, err := h.service.GetProfile(r.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, model.APIResponse{
		Status: "success",
		Data:   profile,
	})
}

// ListProfiles handles GET /api/profiles with optional filter query params.
func (h *ProfileHandler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	filters := model.ProfileFilters{
		Gender:    r.URL.Query().Get("gender"),
		CountryID: r.URL.Query().Get("country_id"),
		AgeGroup:  r.URL.Query().Get("age_group"),
	}

	profiles, err := h.service.ListProfiles(r.Context(), filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	count := len(profiles)
	writeJSON(w, http.StatusOK, model.APIResponse{
		Status: "success",
		Count:  &count,
		Data:   profiles,
	})
}

// DeleteProfile handles DELETE /api/profiles/{id}.
func (h *ProfileHandler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	err := h.service.DeleteProfile(r.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// 204 No Content — set CORS header before WriteHeader since no body follows.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusNoContent)
}
