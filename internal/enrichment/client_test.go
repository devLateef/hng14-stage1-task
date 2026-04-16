package enrichment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"name-profile-api/internal/model"
)

// newTestClient returns an EnrichmentClient whose HTTP requests are routed to
// the provided httptest.Server.
func newTestClient(server *httptest.Server) EnrichmentClient {
	return NewEnrichmentClient(server.Client())
}

// newClientWithBaseURL returns an httpEnrichmentClient whose base URLs are
// overridden by a test server. Because the real client hard-codes the
// production URLs we use a custom RoundTripper that rewrites the host.
func newClientWithRewrite(server *httptest.Server) EnrichmentClient {
	transport := &rewriteTransport{
		base:      server.Client().Transport,
		serverURL: server.URL,
	}
	return NewEnrichmentClient(&http.Client{Transport: transport})
}

// rewriteTransport rewrites every outbound request to point at the test server.
type rewriteTransport struct {
	base      http.RoundTripper
	serverURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Parse the test server URL and replace scheme+host on the request.
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = "http"
	newReq.URL.Host = req.URL.Host // keep original host for routing — not needed here
	// Replace with test server host
	parsed, _ := http.NewRequest(http.MethodGet, t.serverURL, nil)
	newReq.URL.Scheme = parsed.URL.Scheme
	newReq.URL.Host = parsed.URL.Host
	return t.base.RoundTrip(newReq)
}

// --- FetchGender tests ---

func TestFetchGender_NullGender(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"gender":null,"probability":0,"count":100}`))
	}))
	defer server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchGender(context.Background(), "test")

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	upstreamErr, ok := err.(*model.UpstreamError)
	if !ok {
		t.Fatalf("expected *model.UpstreamError, got %T: %v", err, err)
	}
	if upstreamErr.Source != "Genderize" {
		t.Errorf("expected Source=Genderize, got %q", upstreamErr.Source)
	}
}

func TestFetchGender_ZeroCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"gender":"male","probability":0.95,"count":0}`))
	}))
	defer server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchGender(context.Background(), "test")

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	upstreamErr, ok := err.(*model.UpstreamError)
	if !ok {
		t.Fatalf("expected *model.UpstreamError, got %T: %v", err, err)
	}
	if upstreamErr.Source != "Genderize" {
		t.Errorf("expected Source=Genderize, got %q", upstreamErr.Source)
	}
}

func TestFetchGender_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"gender":"female","probability":0.98,"count":5000}`))
	}))
	defer server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchGender(context.Background(), "ella")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Gender != "female" {
		t.Errorf("expected Gender=female, got %q", result.Gender)
	}
	if result.Probability != 0.98 {
		t.Errorf("expected Probability=0.98, got %f", result.Probability)
	}
	if result.Count != 5000 {
		t.Errorf("expected Count=5000, got %d", result.Count)
	}
}

// --- FetchAge tests ---

func TestFetchAge_NullAge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"age":null,"count":0}`))
	}))
	defer server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchAge(context.Background(), "test")

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	upstreamErr, ok := err.(*model.UpstreamError)
	if !ok {
		t.Fatalf("expected *model.UpstreamError, got %T: %v", err, err)
	}
	if upstreamErr.Source != "Agify" {
		t.Errorf("expected Source=Agify, got %q", upstreamErr.Source)
	}
}

func TestFetchAge_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"age":29,"count":12345}`))
	}))
	defer server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchAge(context.Background(), "ella")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Age != 29 {
		t.Errorf("expected Age=29, got %d", result.Age)
	}
	if result.Count != 12345 {
		t.Errorf("expected Count=12345, got %d", result.Count)
	}
}

// --- FetchNationality tests ---

func TestFetchNationality_EmptyCountryArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"country":[]}`))
	}))
	defer server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchNationality(context.Background(), "test")

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	upstreamErr, ok := err.(*model.UpstreamError)
	if !ok {
		t.Fatalf("expected *model.UpstreamError, got %T: %v", err, err)
	}
	if upstreamErr.Source != "Nationalize" {
		t.Errorf("expected Source=Nationalize, got %q", upstreamErr.Source)
	}
}

func TestFetchNationality_ValidResponse_TopCountrySelected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"country":[{"country_id":"US","probability":0.4},{"country_id":"GB","probability":0.7},{"country_id":"AU","probability":0.2}]}`))
	}))
	defer server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchNationality(context.Background(), "ella")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.CountryID != "GB" {
		t.Errorf("expected CountryID=GB (highest probability), got %q", result.CountryID)
	}
	if result.Probability != 0.7 {
		t.Errorf("expected Probability=0.7, got %f", result.Probability)
	}
}

// --- Network failure test ---

func TestFetchGender_NetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler registered but server will be closed before request
	}))
	// Close the server immediately to simulate a network failure.
	server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchGender(context.Background(), "test")

	if result != nil {
		t.Errorf("expected nil result on network failure, got %+v", result)
	}
	if err == nil {
		t.Fatal("expected an error on network failure, got nil")
	}
}

func TestFetchAge_NetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchAge(context.Background(), "test")

	if result != nil {
		t.Errorf("expected nil result on network failure, got %+v", result)
	}
	if err == nil {
		t.Fatal("expected an error on network failure, got nil")
	}
}

func TestFetchNationality_NetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := newClientWithRewrite(server)
	result, err := client.FetchNationality(context.Background(), "test")

	if result != nil {
		t.Errorf("expected nil result on network failure, got %+v", result)
	}
	if err == nil {
		t.Fatal("expected an error on network failure, got nil")
	}
}
