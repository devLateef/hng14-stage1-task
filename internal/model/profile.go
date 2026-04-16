package model

import (
	"errors"
	"time"
)

// Profile represents a persisted name profile with enriched demographic data.
type Profile struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Gender             string    `json:"gender"`
	GenderProbability  float64   `json:"gender_probability"`
	SampleSize         int       `json:"sample_size"`
	Age                int       `json:"age"`
	AgeGroup           string    `json:"age_group"`
	CountryID          string    `json:"country_id"`
	CountryProbability float64   `json:"country_probability"`
	CreatedAt          time.Time `json:"created_at"`
}

// CreateProfileRequest is the request body for POST /api/profiles.
type CreateProfileRequest struct {
	Name string `json:"name"`
}

// APIResponse is the standard envelope for successful responses.
// Count is only included in list responses (non-nil pointer).
type APIResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Count   *int        `json:"count,omitempty"`
}

// ErrorResponse is the standard envelope for error responses.
type ErrorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ProfileFilters holds optional filter values for listing profiles.
// All comparisons are case-insensitive.
type ProfileFilters struct {
	Gender    string
	CountryID string
	AgeGroup  string
}

// Sentinel errors returned by service and repository layers.
var (
	ErrNotFound      = errors.New("profile not found")
	ErrAlreadyExists = errors.New("profile already exists")
	ErrMissingName   = errors.New("name is required")
	ErrNoCountryData = errors.New("no country data available")
)

// UpstreamError is returned when an external enrichment API returns null or
// invalid data. Source identifies which API failed (e.g. "Genderize").
type UpstreamError struct {
	Source string
}

func (e *UpstreamError) Error() string {
	return e.Source + " returned an invalid response"
}

// GenderResult holds the decoded response from the Genderize API.
type GenderResult struct {
	Gender      string
	Probability float64
	Count       int
}

// AgeResult holds the decoded response from the Agify API.
type AgeResult struct {
	Age   int
	Count int
}

// NationalityResult holds the top-country result derived from the Nationalize API.
type NationalityResult struct {
	CountryID   string
	Probability float64
}

// CountryEntry represents a single country entry in the Nationalize API response.
type CountryEntry struct {
	CountryID   string  `json:"country_id"`
	Probability float64 `json:"probability"`
}
