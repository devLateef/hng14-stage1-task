package main_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"name-profile-api/internal/enrichment"
	"name-profile-api/internal/handler"
	"name-profile-api/internal/model"
	"name-profile-api/internal/repository"
	"name-profile-api/internal/service"
)

// ---------------------------------------------------------------------------
// Test infrastructure
// ---------------------------------------------------------------------------

// mockTransport intercepts HTTP calls to external APIs and returns canned JSON.
type mockTransport struct {
	genderJSON      string
	ageJSON         string
	nationalityJSON string
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body string
	host := req.URL.Host
	switch {
	case strings.Contains(host, "genderize") || strings.Contains(req.URL.String(), "genderize"):
		body = t.genderJSON
	case strings.Contains(host, "agify") || strings.Contains(req.URL.String(), "agify"):
		body = t.ageJSON
	default:
		body = t.nationalityJSON
	}
	return &http.Response{
		StatusCode: 200,
		Body:       newStringReadCloser(body),
		Header:     make(http.Header),
	}, nil
}

type stringReadCloser struct{ *strings.Reader }

func (s stringReadCloser) Close() error { return nil }
func newStringReadCloser(s string) stringReadCloser {
	return stringReadCloser{strings.NewReader(s)}
}

// newTestServer builds a full server with an in-memory SQLite DB and a mock
// HTTP transport for external API calls.
func newTestServer(t *testing.T, transport http.RoundTripper) *httptest.Server {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repo := repository.NewSQLiteRepository(db)

	// Manually run schema since NewSQLiteRepository uses go-sqlite3 driver name
	// in production; here we use modernc.org/sqlite with driver name "sqlite".
	_, err = db.ExecContext(context.Background(), `
CREATE TABLE IF NOT EXISTS profiles (
    id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, gender TEXT NOT NULL,
    gender_probability REAL NOT NULL, sample_size INTEGER NOT NULL,
    age INTEGER NOT NULL, age_group TEXT NOT NULL, country_id TEXT NOT NULL,
    country_probability REAL NOT NULL, created_at TEXT NOT NULL
);`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}

	httpClient := &http.Client{Transport: transport}
	enrichmentClient := enrichment.NewEnrichmentClient(httpClient)
	profileService := service.NewProfileService(repo, enrichmentClient)
	profileHandler := handler.NewProfileHandler(profileService)

	r := chi.NewRouter()
	r.Post("/api/profiles", profileHandler.CreateProfile)
	r.Get("/api/profiles", profileHandler.ListProfiles)
	r.Get("/api/profiles/{id}", profileHandler.GetProfile)
	r.Delete("/api/profiles/{id}", profileHandler.DeleteProfile)

	return httptest.NewServer(r)
}

func defaultTransport() *mockTransport {
	return &mockTransport{
		genderJSON:      `{"gender":"female","probability":0.99,"count":5000}`,
		ageJSON:         `{"age":30,"count":1000}`,
		nationalityJSON: `{"country":[{"country_id":"US","probability":0.8},{"country_id":"GB","probability":0.2}]}`,
	}
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

// TestIntegration_CreateGetListDelete tests the full CRUD lifecycle.
func TestIntegration_CreateGetListDelete(t *testing.T) {
	srv := newTestServer(t, defaultTransport())
	defer srv.Close()
	client := srv.Client()

	// 1. Create profile.
	body, _ := json.Marshal(map[string]string{"name": "ella"})
	resp, err := client.Post(srv.URL+"/api/profiles", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST: expected 201, got %d", resp.StatusCode)
	}

	var createResp struct {
		Status string        `json:"status"`
		Data   model.Profile `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()

	if createResp.Status != "success" {
		t.Fatalf("POST: expected status=success, got %q", createResp.Status)
	}
	profileID := createResp.Data.ID
	if profileID == "" {
		t.Fatal("POST: expected non-empty id")
	}
	if createResp.Data.AgeGroup != "adult" {
		t.Errorf("POST: expected age_group=adult, got %q", createResp.Data.AgeGroup)
	}
	if createResp.Data.CountryID != "US" {
		t.Errorf("POST: expected country_id=US (highest prob), got %q", createResp.Data.CountryID)
	}

	// 2. Get profile by ID.
	resp, err = client.Get(srv.URL + "/api/profiles/" + profileID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. List profiles.
	resp, err = client.Get(srv.URL + "/api/profiles")
	if err != nil {
		t.Fatalf("LIST: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("LIST: expected 200, got %d", resp.StatusCode)
	}
	var listResp struct {
		Status string          `json:"status"`
		Count  int             `json:"count"`
		Data   []model.Profile `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()
	if len(listResp.Data) != 1 {
		t.Fatalf("LIST: expected 1 profile, got %d", len(listResp.Data))
	}

	// 4. Delete profile.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/profiles/"+profileID, nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Verify profile is gone.
	resp, err = client.Get(srv.URL + "/api/profiles/" + profileID)
	if err != nil {
		t.Fatalf("GET after delete: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestIntegration_Idempotency verifies two POSTs with the same name return the same id.
func TestIntegration_Idempotency(t *testing.T) {
	srv := newTestServer(t, defaultTransport())
	defer srv.Close()
	client := srv.Client()

	post := func() string {
		body, _ := json.Marshal(map[string]string{"name": "ella"})
		resp, err := client.Post(srv.URL+"/api/profiles", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		var r struct {
			Data model.Profile `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&r)
		return r.Data.ID
	}

	id1 := post()
	id2 := post()

	if id1 == "" {
		t.Fatal("first POST returned empty id")
	}
	if id1 != id2 {
		t.Fatalf("idempotency violated: first=%q, second=%q", id1, id2)
	}

	// Verify only one record exists.
	resp, _ := client.Get(srv.URL + "/api/profiles")
	var listResp struct {
		Data []model.Profile `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()
	if len(listResp.Data) != 1 {
		t.Fatalf("expected 1 record after 2 POSTs with same name, got %d", len(listResp.Data))
	}
}

// TestIntegration_GenderFilter verifies the gender query parameter filters correctly.
func TestIntegration_GenderFilter(t *testing.T) {
	srv := newTestServer(t, defaultTransport())
	defer srv.Close()
	client := srv.Client()

	// Create a female profile.
	body, _ := json.Marshal(map[string]string{"name": "ella"})
	resp, _ := client.Post(srv.URL+"/api/profiles", "application/json", bytes.NewReader(body))
	resp.Body.Close()

	// Create a male profile using a different mock transport.
	maleSrv := newTestServer(t, &mockTransport{
		genderJSON:      `{"gender":"male","probability":0.95,"count":3000}`,
		ageJSON:         `{"age":25,"count":800}`,
		nationalityJSON: `{"country":[{"country_id":"NG","probability":0.9}]}`,
	})
	defer maleSrv.Close()
	maleClient := maleSrv.Client()
	body, _ = json.Marshal(map[string]string{"name": "james"})
	resp, _ = maleClient.Post(maleSrv.URL+"/api/profiles", "application/json", bytes.NewReader(body))
	resp.Body.Close()

	// Filter by female on the first server (only ella was created there).
	resp, err := client.Get(srv.URL + "/api/profiles?gender=female")
	if err != nil {
		t.Fatalf("GET with filter: %v", err)
	}
	var listResp struct {
		Data []model.Profile `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()

	if len(listResp.Data) != 1 {
		t.Fatalf("expected 1 female profile, got %d", len(listResp.Data))
	}
	if listResp.Data[0].Gender != "female" {
		t.Errorf("expected gender=female, got %q", listResp.Data[0].Gender)
	}
}

// TestIntegration_UpstreamError_Returns502 verifies 502 when an external API fails.
func TestIntegration_UpstreamError_Returns502(t *testing.T) {
	srv := newTestServer(t, &mockTransport{
		genderJSON:      `{"gender":null,"probability":0,"count":0}`,
		ageJSON:         `{"age":30,"count":100}`,
		nationalityJSON: `{"country":[{"country_id":"US","probability":0.9}]}`,
	})
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"name": "unknown"})
	resp, err := srv.Client().Post(srv.URL+"/api/profiles", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
}
