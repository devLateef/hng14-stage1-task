package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"name-profile-api/internal/enrichment"
	"name-profile-api/internal/model"
	"name-profile-api/internal/repository"
)

// ProfileService defines the business-logic operations for profiles.
type ProfileService interface {
	CreateProfile(ctx context.Context, name string) (*model.Profile, error)
	GetProfile(ctx context.Context, id string) (*model.Profile, error)
	ListProfiles(ctx context.Context, filters model.ProfileFilters) ([]model.Profile, error)
	DeleteProfile(ctx context.Context, id string) error
}

type profileService struct {
	repo       repository.ProfileRepository
	enrichment enrichment.EnrichmentClient
}

// NewProfileService returns a ProfileService backed by the given repository
// and enrichment client.
func NewProfileService(repo repository.ProfileRepository, enrichmentClient enrichment.EnrichmentClient) ProfileService {
	return &profileService{
		repo:       repo,
		enrichment: enrichmentClient,
	}
}

// CreateProfile creates a new profile for the given name. If a profile with
// that name already exists it returns the existing profile along with
// model.ErrAlreadyExists. Returns model.ErrMissingName for blank names.
func (s *profileService) CreateProfile(ctx context.Context, name string) (*model.Profile, error) {
	// Step 1: Validate name.
	if err := ValidateName(name); err != nil {
		return nil, err
	}

	// Step 2: Idempotency check — return existing profile if found.
	existing, err := s.repo.FindByName(ctx, name)
	if err == nil {
		return existing, model.ErrAlreadyExists
	}
	if !errors.Is(err, model.ErrNotFound) {
		return nil, err
	}

	// Step 3: Enrich — all three calls must succeed.
	genderResult, err := s.enrichment.FetchGender(ctx, name)
	if err != nil {
		return nil, err
	}

	ageResult, err := s.enrichment.FetchAge(ctx, name)
	if err != nil {
		return nil, err
	}

	nationalityResult, err := s.enrichment.FetchNationality(ctx, name)
	if err != nil {
		return nil, err
	}

	// Step 4: Classify age.
	ageGroup := ClassifyAge(ageResult.Age)

	// Step 5: Build and persist profile.
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	profile := &model.Profile{
		ID:                 id.String(),
		Name:               name,
		Gender:             genderResult.Gender,
		GenderProbability:  genderResult.Probability,
		SampleSize:         genderResult.Count,
		Age:                ageResult.Age,
		AgeGroup:           ageGroup,
		CountryID:          nationalityResult.CountryID,
		CountryProbability: nationalityResult.Probability,
		CreatedAt:          time.Now().UTC(),
	}

	if err := s.repo.Insert(ctx, profile); err != nil {
		return nil, err
	}

	return profile, nil
}

// GetProfile retrieves a profile by its UUID.
func (s *profileService) GetProfile(ctx context.Context, id string) (*model.Profile, error) {
	return s.repo.FindByID(ctx, id)
}

// ListProfiles returns all profiles matching the supplied filters.
func (s *profileService) ListProfiles(ctx context.Context, filters model.ProfileFilters) ([]model.Profile, error) {
	return s.repo.List(ctx, filters)
}

// DeleteProfile removes a profile by its UUID.
func (s *profileService) DeleteProfile(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
