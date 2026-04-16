package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"

	"name-profile-api/internal/model"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// mockRepo is an in-memory ProfileRepository for testing.
type mockRepo struct {
	mu       sync.Mutex
	byID     map[string]*model.Profile
	byName   map[string]*model.Profile
	inserted int
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		byID:   make(map[string]*model.Profile),
		byName: make(map[string]*model.Profile),
	}
}

func (r *mockRepo) Insert(_ context.Context, p *model.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[p.Name]; exists {
		return model.ErrAlreadyExists
	}
	cp := *p
	r.byID[p.ID] = &cp
	r.byName[p.Name] = &cp
	r.inserted++
	return nil
}

func (r *mockRepo) FindByID(_ context.Context, id string) (*model.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byID[id]
	if !ok {
		return nil, model.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *mockRepo) FindByName(_ context.Context, name string) (*model.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byName[name]
	if !ok {
		return nil, model.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *mockRepo) List(_ context.Context, _ model.ProfileFilters) ([]model.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	profiles := make([]model.Profile, 0, len(r.byID))
	for _, p := range r.byID {
		profiles = append(profiles, *p)
	}
	return profiles, nil
}

func (r *mockRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byID[id]
	if !ok {
		return model.ErrNotFound
	}
	delete(r.byName, p.Name)
	delete(r.byID, id)
	return nil
}

// mockEnrichment is a configurable EnrichmentClient for testing.
type mockEnrichment struct {
	genderErr      error
	ageErr         error
	nationalityErr error
	gender         *model.GenderResult
	age            *model.AgeResult
	nationality    *model.NationalityResult
}

func (m *mockEnrichment) FetchGender(_ context.Context, _ string) (*model.GenderResult, error) {
	if m.genderErr != nil {
		return nil, m.genderErr
	}
	if m.gender != nil {
		return m.gender, nil
	}
	return &model.GenderResult{Gender: "female", Probability: 0.99, Count: 1000}, nil
}

func (m *mockEnrichment) FetchAge(_ context.Context, _ string) (*model.AgeResult, error) {
	if m.ageErr != nil {
		return nil, m.ageErr
	}
	if m.age != nil {
		return m.age, nil
	}
	return &model.AgeResult{Age: 30, Count: 500}, nil
}

func (m *mockEnrichment) FetchNationality(_ context.Context, _ string) (*model.NationalityResult, error) {
	if m.nationalityErr != nil {
		return nil, m.nationalityErr
	}
	if m.nationality != nil {
		return m.nationality, nil
	}
	return &model.NationalityResult{CountryID: "US", Probability: 0.8}, nil
}

// ---------------------------------------------------------------------------
// Property 7: Profile creation idempotency
// Validates: Requirements 11.1, 11.2
// ---------------------------------------------------------------------------

func TestProperty7_CreateProfile_Idempotency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		name := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{1,20}`).Draw(rt, "name")

		repo := newMockRepo()
		enrich := &mockEnrichment{}
		svc := NewProfileService(repo, enrich)
		ctx := context.Background()

		p1, err1 := svc.CreateProfile(ctx, name)
		if err1 != nil {
			rt.Fatalf("first CreateProfile failed: %v", err1)
		}

		p2, err2 := svc.CreateProfile(ctx, name)
		if !errors.Is(err2, model.ErrAlreadyExists) {
			rt.Fatalf("second CreateProfile: expected ErrAlreadyExists, got %v", err2)
		}

		// Both calls must return the same id.
		if p1.ID != p2.ID {
			rt.Fatalf("idempotency violated: first id=%q, second id=%q", p1.ID, p2.ID)
		}

		// Exactly one record in the repository.
		if repo.inserted != 1 {
			rt.Fatalf("expected 1 insert, got %d", repo.inserted)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 4: Upstream errors never result in a stored profile
// Validates: Requirements 5.4, 5.5, 5.6, 5.7
// ---------------------------------------------------------------------------

func TestProperty4_UpstreamError_NoInsert(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Pick which enrichment call fails (0=gender, 1=age, 2=nationality).
		failIdx := rapid.IntRange(0, 2).Draw(rt, "fail_idx")

		repo := newMockRepo()
		enrich := &mockEnrichment{}
		upstreamErr := &model.UpstreamError{Source: "TestSource"}

		switch failIdx {
		case 0:
			enrich.genderErr = upstreamErr
		case 1:
			enrich.ageErr = upstreamErr
		case 2:
			enrich.nationalityErr = upstreamErr
		}

		svc := NewProfileService(repo, enrich)
		_, err := svc.CreateProfile(context.Background(), "testname")

		if err == nil {
			rt.Fatal("expected error from upstream failure, got nil")
		}
		if repo.inserted != 0 {
			rt.Fatalf("expected 0 inserts on upstream error, got %d", repo.inserted)
		}
	})
}

// ---------------------------------------------------------------------------
// Unit tests for service layer (Task 8.4)
// ---------------------------------------------------------------------------

func TestCreateProfile_Success(t *testing.T) {
	repo := newMockRepo()
	enrich := &mockEnrichment{
		gender:      &model.GenderResult{Gender: "female", Probability: 0.98, Count: 5000},
		age:         &model.AgeResult{Age: 25, Count: 1000},
		nationality: &model.NationalityResult{CountryID: "NG", Probability: 0.85},
	}
	svc := NewProfileService(repo, enrich)

	p, err := svc.CreateProfile(context.Background(), "ella")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "ella" {
		t.Errorf("Name: want ella, got %s", p.Name)
	}
	if p.Gender != "female" {
		t.Errorf("Gender: want female, got %s", p.Gender)
	}
	if p.AgeGroup != "adult" {
		t.Errorf("AgeGroup: want adult, got %s", p.AgeGroup)
	}
	if p.CountryID != "NG" {
		t.Errorf("CountryID: want NG, got %s", p.CountryID)
	}
	if p.ID == "" {
		t.Error("ID should not be empty")
	}
	if p.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if p.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt should be UTC, got %v", p.CreatedAt.Location())
	}
}

func TestCreateProfile_GenderUpstreamError(t *testing.T) {
	repo := newMockRepo()
	enrich := &mockEnrichment{genderErr: &model.UpstreamError{Source: "Genderize"}}
	svc := NewProfileService(repo, enrich)

	_, err := svc.CreateProfile(context.Background(), "ella")
	var upErr *model.UpstreamError
	if !errors.As(err, &upErr) {
		t.Fatalf("expected UpstreamError, got %v", err)
	}
	if upErr.Source != "Genderize" {
		t.Errorf("Source: want Genderize, got %s", upErr.Source)
	}
	if repo.inserted != 0 {
		t.Errorf("expected 0 inserts, got %d", repo.inserted)
	}
}

func TestCreateProfile_AgeUpstreamError(t *testing.T) {
	repo := newMockRepo()
	enrich := &mockEnrichment{ageErr: &model.UpstreamError{Source: "Agify"}}
	svc := NewProfileService(repo, enrich)

	_, err := svc.CreateProfile(context.Background(), "ella")
	var upErr *model.UpstreamError
	if !errors.As(err, &upErr) {
		t.Fatalf("expected UpstreamError, got %v", err)
	}
	if repo.inserted != 0 {
		t.Errorf("expected 0 inserts, got %d", repo.inserted)
	}
}

func TestCreateProfile_NationalityUpstreamError(t *testing.T) {
	repo := newMockRepo()
	enrich := &mockEnrichment{nationalityErr: &model.UpstreamError{Source: "Nationalize"}}
	svc := NewProfileService(repo, enrich)

	_, err := svc.CreateProfile(context.Background(), "ella")
	var upErr *model.UpstreamError
	if !errors.As(err, &upErr) {
		t.Fatalf("expected UpstreamError, got %v", err)
	}
	if repo.inserted != 0 {
		t.Errorf("expected 0 inserts, got %d", repo.inserted)
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	svc := NewProfileService(newMockRepo(), &mockEnrichment{})
	_, err := svc.GetProfile(context.Background(), "nonexistent-id")
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteProfile_NotFound(t *testing.T) {
	svc := NewProfileService(newMockRepo(), &mockEnrichment{})
	err := svc.DeleteProfile(context.Background(), "nonexistent-id")
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateProfile_MissingName(t *testing.T) {
	svc := NewProfileService(newMockRepo(), &mockEnrichment{})
	_, err := svc.CreateProfile(context.Background(), "   ")
	if !errors.Is(err, model.ErrMissingName) {
		t.Fatalf("expected ErrMissingName, got %v", err)
	}
}
