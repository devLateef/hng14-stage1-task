package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"pgregory.net/rapid"

	"name-profile-api/internal/model"
	"name-profile-api/internal/service"
)

// ---------------------------------------------------------------------------
// Mock service
// ---------------------------------------------------------------------------

type mockService struct {
	createFn func(ctx context.Context, name string) (*model.Profile, error)
	getFn    func(ctx context.Context, id string) (*model.Profile, error)
	listFn   func(ctx context.Context, filters model.ProfileFilters) ([]model.Profile, error)
	deleteFn func(ctx context.Context, id string) error
}

func (m *mockService) CreateProfile(ctx context.Context, name string) (*model.Profile, error) {
	if m.createFn != nil {
		return m.createFn(ctx, name)
	}
	return nil, errors.New("not implemented")
}

func (m *mockService) GetProfile(ctx context.Context, id string) (*model.Profile, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockService) ListProfiles(ctx context.Context, filters model.ProfileFilters) ([]model.Profile, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filters)
	}
	return []model.Profile{}, nil
}

func (m *mockService) DeleteProfile(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return errors.New("not implemented")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newRouter(svc service.ProfileService) http.Handler {
	h := NewProfileHandler(svc)
	r := chi.NewRouter()
	r.Post("/api/profiles", h.CreateProfile)
	r.Get("/api/profiles", h.ListProfiles)
	r.Get("/api/profiles/{id}", h.GetProfile)
	r.Delete("/api/profiles/{id}", h.DeleteProfile)
	return r
}

func sampleProfile() *model.Profile {
	return &model.Profile{
		ID:                 "test-id-123",
		Name:               "ella",
		Gender:             "female",
		GenderProbability:  0.99,
		SampleSize:         1000,
		Age:                30,
		AgeGroup:           "adult",
		CountryID:          "US",
		CountryProbability: 0.8,
		CreatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// ---------------------------------------------------------------------------
// Property 10: Every response carries the CORS header
// Validates: Requirements 10.1
// ---------------------------------------------------------------------------

func TestProperty10_CORSHeaderOnEveryResponse(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Pick a scenario: success or error for each endpoint.
		scenario := rapid.IntRange(0, 7).Draw(rt, "scenario")

		var svc *mockService
		var req *http.Request

		switch scenario {
		case 0: // POST success
			svc = &mockService{createFn: func(_ context.Context, _ string) (*model.Profile, error) {
				return sampleProfile(), nil
			}}
			req = httptest.NewRequest(http.MethodPost, "/api/profiles", strings.NewReader(`{"name":"ella"}`))
		case 1: // POST missing name
			svc = &mockService{createFn: func(_ context.Context, _ string) (*model.Profile, error) {
				return nil, model.ErrMissingName
			}}
			req = httptest.NewRequest(http.MethodPost, "/api/profiles", strings.NewReader(`{"name":""}`))
		case 2: // POST upstream error
			svc = &mockService{createFn: func(_ context.Context, _ string) (*model.Profile, error) {
				return nil, &model.UpstreamError{Source: "Genderize"}
			}}
			req = httptest.NewRequest(http.MethodPost, "/api/profiles", strings.NewReader(`{"name":"ella"}`))
		case 3: // GET success
			svc = &mockService{getFn: func(_ context.Context, _ string) (*model.Profile, error) {
				return sampleProfile(), nil
			}}
			req = httptest.NewRequest(http.MethodGet, "/api/profiles/test-id-123", nil)
		case 4: // GET not found
			svc = &mockService{getFn: func(_ context.Context, _ string) (*model.Profile, error) {
				return nil, model.ErrNotFound
			}}
			req = httptest.NewRequest(http.MethodGet, "/api/profiles/nonexistent", nil)
		case 5: // GET list
			svc = &mockService{listFn: func(_ context.Context, _ model.ProfileFilters) ([]model.Profile, error) {
				return []model.Profile{*sampleProfile()}, nil
			}}
			req = httptest.NewRequest(http.MethodGet, "/api/profiles", nil)
		case 6: // DELETE success
			svc = &mockService{deleteFn: func(_ context.Context, _ string) error { return nil }}
			req = httptest.NewRequest(http.MethodDelete, "/api/profiles/test-id-123", nil)
		case 7: // DELETE not found
			svc = &mockService{deleteFn: func(_ context.Context, _ string) error { return model.ErrNotFound }}
			req = httptest.NewRequest(http.MethodDelete, "/api/profiles/nonexistent", nil)
		}

		rr := httptest.NewRecorder()
		newRouter(svc).ServeHTTP(rr, req)

		cors := rr.Header().Get("Access-Control-Allow-Origin")
		if cors != "*" {
			rt.Fatalf("scenario %d: expected Access-Control-Allow-Origin: *, got %q (status %d)",
				scenario, cors, rr.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 8: Profile JSON round-trip
// Validates: Requirements 12.1, 12.4
// ---------------------------------------------------------------------------

func TestProperty8_ProfileJSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		original := &model.Profile{
			ID:                 rapid.StringMatching(`[a-f0-9-]{36}`).Draw(rt, "id"),
			Name:               rapid.StringMatching(`[a-zA-Z]{3,20}`).Draw(rt, "name"),
			Gender:             rapid.SampledFrom([]string{"male", "female"}).Draw(rt, "gender"),
			GenderProbability:  rapid.Float64Range(0, 1).Draw(rt, "gender_prob"),
			SampleSize:         rapid.IntRange(1, 10000).Draw(rt, "sample_size"),
			Age:                rapid.IntRange(0, 120).Draw(rt, "age"),
			AgeGroup:           rapid.SampledFrom([]string{"child", "teenager", "adult", "senior"}).Draw(rt, "age_group"),
			CountryID:          rapid.StringMatching(`[A-Z]{2}`).Draw(rt, "country_id"),
			CountryProbability: rapid.Float64Range(0, 1).Draw(rt, "country_prob"),
			// Truncate to second precision for RFC3339 round-trip.
			CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).
				Add(time.Duration(rapid.IntRange(0, 1000000).Draw(rt, "ts_offset")) * time.Second),
		}

		data, err := json.Marshal(original)
		if err != nil {
			rt.Fatalf("marshal failed: %v", err)
		}

		var decoded model.Profile
		if err := json.Unmarshal(data, &decoded); err != nil {
			rt.Fatalf("unmarshal failed: %v", err)
		}

		if decoded.ID != original.ID {
			rt.Fatalf("ID mismatch: want %q, got %q", original.ID, decoded.ID)
		}
		if decoded.Name != original.Name {
			rt.Fatalf("Name mismatch")
		}
		if decoded.Gender != original.Gender {
			rt.Fatalf("Gender mismatch")
		}
		if decoded.GenderProbability != original.GenderProbability {
			rt.Fatalf("GenderProbability mismatch")
		}
		if decoded.SampleSize != original.SampleSize {
			rt.Fatalf("SampleSize mismatch")
		}
		if decoded.Age != original.Age {
			rt.Fatalf("Age mismatch")
		}
		if decoded.AgeGroup != original.AgeGroup {
			rt.Fatalf("AgeGroup mismatch")
		}
		if decoded.CountryID != original.CountryID {
			rt.Fatalf("CountryID mismatch")
		}
		if decoded.CountryProbability != original.CountryProbability {
			rt.Fatalf("CountryProbability mismatch")
		}
		if !decoded.CreatedAt.Equal(original.CreatedAt) {
			rt.Fatalf("CreatedAt mismatch: want %v, got %v", original.CreatedAt, decoded.CreatedAt)
		}
	})
}

// ---------------------------------------------------------------------------
// Unit tests for handler error mapping (Task 10.4)
// ---------------------------------------------------------------------------

func TestCreateProfile_MissingName_Returns400(t *testing.T) {
	svc := &mockService{createFn: func(_ context.Context, name string) (*model.Profile, error) {
		return nil, model.ErrMissingName
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profiles", strings.NewReader(`{"name":""}`))
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorBody(t, rr, "name is required")
}

func TestCreateProfile_MalformedJSON_Returns422(t *testing.T) {
	svc := &mockService{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profiles", strings.NewReader(`{bad json`))
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rr.Code)
	}
	assertErrorBody(t, rr, "invalid request body")
}

func TestCreateProfile_NonStringName_Returns422(t *testing.T) {
	svc := &mockService{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profiles", strings.NewReader(`{"name":123}`))
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rr.Code)
	}
	assertErrorBody(t, rr, "invalid request body")
}

func TestCreateProfile_DuplicateName_Returns200WithMessage(t *testing.T) {
	p := sampleProfile()
	svc := &mockService{createFn: func(_ context.Context, _ string) (*model.Profile, error) {
		return p, model.ErrAlreadyExists
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profiles", strings.NewReader(`{"name":"ella"}`))
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["message"] != "Profile already exists" {
		t.Errorf("expected 'Profile already exists' message, got %v", resp["message"])
	}
}

func TestCreateProfile_UpstreamError_Returns502(t *testing.T) {
	svc := &mockService{createFn: func(_ context.Context, _ string) (*model.Profile, error) {
		return nil, &model.UpstreamError{Source: "Genderize"}
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profiles", strings.NewReader(`{"name":"ella"}`))
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rr.Code)
	}
	assertErrorBody(t, rr, "Genderize returned an invalid response")
}

func TestGetProfile_NotFound_Returns404(t *testing.T) {
	svc := &mockService{getFn: func(_ context.Context, _ string) (*model.Profile, error) {
		return nil, model.ErrNotFound
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/profiles/nonexistent", nil)
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
	assertErrorBody(t, rr, "profile not found")
}

func TestDeleteProfile_Success_Returns204(t *testing.T) {
	svc := &mockService{deleteFn: func(_ context.Context, _ string) error { return nil }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/profiles/test-id-123", nil)
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("expected empty body for 204, got %q", rr.Body.String())
	}
}

func TestDeleteProfile_NotFound_Returns404(t *testing.T) {
	svc := &mockService{deleteFn: func(_ context.Context, _ string) error { return model.ErrNotFound }}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/profiles/nonexistent", nil)
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestListProfiles_ReturnsEmptyArray(t *testing.T) {
	svc := &mockService{listFn: func(_ context.Context, _ model.ProfileFilters) ([]model.Profile, error) {
		return []model.Profile{}, nil
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/profiles", nil)
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data to be an array, got %T", resp["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty array, got %v", data)
	}
}

func TestCreateProfile_Success_Returns201(t *testing.T) {
	p := sampleProfile()
	svc := &mockService{createFn: func(_ context.Context, _ string) (*model.Profile, error) {
		return p, nil
	}}
	body, _ := json.Marshal(map[string]string{"name": "ella"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profiles", bytes.NewReader(body))
	newRouter(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "success" {
		t.Errorf("expected status=success, got %v", resp["status"])
	}
}

// assertErrorBody checks that the response body is a JSON error with the given message.
func assertErrorBody(t *testing.T, rr *httptest.ResponseRecorder, wantMsg string) {
	t.Helper()
	var resp model.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Status != "error" {
		t.Errorf("expected status=error, got %q", resp.Status)
	}
	if resp.Message != wantMsg {
		t.Errorf("expected message=%q, got %q", wantMsg, resp.Message)
	}
}
