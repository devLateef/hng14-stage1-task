package service

import (
	"strings"

	"name-profile-api/internal/model"
)

// ClassifyAge returns the age group label for the given age.
// Precondition: age >= 0
func ClassifyAge(age int) string {
	switch {
	case age <= 12:
		return "child"
	case age <= 19:
		return "teenager"
	case age <= 59:
		return "adult"
	default:
		return "senior"
	}
}

// TopCountry returns the country ID and probability of the entry with the
// highest probability in the slice. Ties are broken by first occurrence.
// Returns model.ErrNoCountryData when the slice is empty.
func TopCountry(countries []model.CountryEntry) (string, float64, error) {
	if len(countries) == 0 {
		return "", 0, model.ErrNoCountryData
	}

	best := countries[0]
	for _, c := range countries[1:] {
		if c.Probability > best.Probability {
			best = c
		}
	}
	return best.CountryID, best.Probability, nil
}

// ValidateName returns model.ErrMissingName when name is empty or whitespace-only.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return model.ErrMissingName
	}
	return nil
}
