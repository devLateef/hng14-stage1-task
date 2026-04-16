package service

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"name-profile-api/internal/model"
)

// ---------------------------------------------------------------------------
// Property 1: ClassifyAge covers all non-negative ages with correct labels
// Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5
// ---------------------------------------------------------------------------

func TestClassifyAge_Property1_ChildRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		age := rapid.IntRange(0, 12).Draw(t, "age")
		if got := ClassifyAge(age); got != "child" {
			t.Fatalf("ClassifyAge(%d) = %q, want %q", age, got, "child")
		}
	})
}

func TestClassifyAge_Property1_TeenagerRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		age := rapid.IntRange(13, 19).Draw(t, "age")
		if got := ClassifyAge(age); got != "teenager" {
			t.Fatalf("ClassifyAge(%d) = %q, want %q", age, got, "teenager")
		}
	})
}

func TestClassifyAge_Property1_AdultRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		age := rapid.IntRange(20, 59).Draw(t, "age")
		if got := ClassifyAge(age); got != "adult" {
			t.Fatalf("ClassifyAge(%d) = %q, want %q", age, got, "adult")
		}
	})
}

func TestClassifyAge_Property1_SeniorRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		age := rapid.IntRange(60, 200).Draw(t, "age")
		if got := ClassifyAge(age); got != "senior" {
			t.Fatalf("ClassifyAge(%d) = %q, want %q", age, got, "senior")
		}
	})
}

func TestClassifyAge_Property1_ValidLabel(t *testing.T) {
	validLabels := map[string]bool{
		"child":    true,
		"teenager": true,
		"adult":    true,
		"senior":   true,
	}
	rapid.Check(t, func(t *rapid.T) {
		age := rapid.IntRange(0, 200).Draw(t, "age")
		got := ClassifyAge(age)
		if !validLabels[got] {
			t.Fatalf("ClassifyAge(%d) = %q, not a valid label", age, got)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 2: TopCountry returns the maximum-probability entry
// Validates: Requirements 4.1, 4.2
// ---------------------------------------------------------------------------

func TestTopCountry_Property2_MaxProbability(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a non-empty slice of CountryEntry values.
		entries := rapid.SliceOfN(
			rapid.Custom(func(t *rapid.T) model.CountryEntry {
				return model.CountryEntry{
					CountryID:   rapid.StringN(2, 3, -1).Draw(t, "country_id"),
					Probability: rapid.Float64Range(0, 1).Draw(t, "probability"),
				}
			}),
			1, 20,
		).Draw(t, "entries")

		_, prob, err := TopCountry(entries)
		if err != nil {
			t.Fatalf("TopCountry returned unexpected error: %v", err)
		}

		// Find the actual maximum probability in the slice.
		maxProb := entries[0].Probability
		for _, e := range entries[1:] {
			if e.Probability > maxProb {
				maxProb = e.Probability
			}
		}

		if prob != maxProb {
			t.Fatalf("TopCountry returned probability %v, want max %v", prob, maxProb)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 3: ValidateName rejects exactly the whitespace-only inputs
// Validates: Requirements 1.2, 1.5
// ---------------------------------------------------------------------------

func TestValidateName_Property3_IffWhitespace(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "name")
		err := ValidateName(s)
		isBlank := strings.TrimSpace(s) == ""
		if isBlank && err != model.ErrMissingName {
			t.Fatalf("ValidateName(%q): blank input, want ErrMissingName, got %v", s, err)
		}
		if !isBlank && err != nil {
			t.Fatalf("ValidateName(%q): non-blank input, want nil, got %v", s, err)
		}
	})
}

// ---------------------------------------------------------------------------
// Unit tests: ClassifyAge boundary values (Task 3.5)
// ---------------------------------------------------------------------------

func TestClassifyAge_BoundaryValues(t *testing.T) {
	tests := []struct {
		age  int
		want string
	}{
		{0, "child"},
		{12, "child"},
		{13, "teenager"},
		{19, "teenager"},
		{20, "adult"},
		{59, "adult"},
		{60, "senior"},
		{100, "senior"},
	}
	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			got := ClassifyAge(tc.age)
			if got != tc.want {
				t.Errorf("ClassifyAge(%d) = %q, want %q", tc.age, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unit tests: TopCountry edge cases (Task 3.6)
// ---------------------------------------------------------------------------

func TestTopCountry_EmptySlice(t *testing.T) {
	_, _, err := TopCountry([]model.CountryEntry{})
	if err != model.ErrNoCountryData {
		t.Errorf("TopCountry(empty) error = %v, want ErrNoCountryData", err)
	}
}

func TestTopCountry_SingleEntry(t *testing.T) {
	entries := []model.CountryEntry{
		{CountryID: "US", Probability: 0.9},
	}
	id, prob, err := TopCountry(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "US" || prob != 0.9 {
		t.Errorf("TopCountry = (%q, %v), want (\"US\", 0.9)", id, prob)
	}
}

func TestTopCountry_TieFirstWins(t *testing.T) {
	entries := []model.CountryEntry{
		{CountryID: "US", Probability: 0.5},
		{CountryID: "GB", Probability: 0.5},
		{CountryID: "DE", Probability: 0.3},
	}
	id, prob, err := TopCountry(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "US" || prob != 0.5 {
		t.Errorf("TopCountry = (%q, %v), want (\"US\", 0.5)", id, prob)
	}
}
