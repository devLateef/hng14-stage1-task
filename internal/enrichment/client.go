package enrichment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"name-profile-api/internal/model"
)

// EnrichmentClient defines the interface for fetching demographic data from
// external enrichment APIs.
type EnrichmentClient interface {
	FetchGender(ctx context.Context, name string) (*model.GenderResult, error)
	FetchAge(ctx context.Context, name string) (*model.AgeResult, error)
	FetchNationality(ctx context.Context, name string) (*model.NationalityResult, error)
}

// httpEnrichmentClient implements EnrichmentClient using a real HTTP client.
type httpEnrichmentClient struct {
	httpClient *http.Client
}

// NewEnrichmentClient returns an EnrichmentClient backed by the provided
// *http.Client.
func NewEnrichmentClient(httpClient *http.Client) EnrichmentClient {
	return &httpEnrichmentClient{httpClient: httpClient}
}

// FetchGender calls api.genderize.io and returns the gender prediction for the
// given name. Returns UpstreamError when the API returns null gender or zero
// count.
func (c *httpEnrichmentClient) FetchGender(ctx context.Context, name string) (*model.GenderResult, error) {
	reqURL := "https://api.genderize.io?name=" + url.QueryEscape(name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, &model.UpstreamError{Source: "Genderize"}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &model.UpstreamError{Source: "Genderize"}
	}
	defer resp.Body.Close()

	var payload struct {
		Gender      *string `json:"gender"`
		Probability float64 `json:"probability"`
		Count       int     `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, &model.UpstreamError{Source: "Genderize"}
	}

	if payload.Gender == nil || payload.Count == 0 {
		return nil, &model.UpstreamError{Source: "Genderize"}
	}

	return &model.GenderResult{
		Gender:      *payload.Gender,
		Probability: payload.Probability,
		Count:       payload.Count,
	}, nil
}

// FetchAge calls api.agify.io and returns the age prediction for the given
// name. Returns UpstreamError when the API returns null age.
func (c *httpEnrichmentClient) FetchAge(ctx context.Context, name string) (*model.AgeResult, error) {
	reqURL := "https://api.agify.io?name=" + url.QueryEscape(name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, &model.UpstreamError{Source: "Agify"}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &model.UpstreamError{Source: "Agify"}
	}
	defer resp.Body.Close()

	var payload struct {
		Age   *int `json:"age"`
		Count int  `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, &model.UpstreamError{Source: "Agify"}
	}

	if payload.Age == nil {
		return nil, &model.UpstreamError{Source: "Agify"}
	}

	return &model.AgeResult{
		Age:   *payload.Age,
		Count: payload.Count,
	}, nil
}

// FetchNationality calls api.nationalize.io and returns the top-probability
// country for the given name. Returns UpstreamError when the API returns an
// empty country array. The max-probability selection is inlined here to avoid
// importing the service package.
func (c *httpEnrichmentClient) FetchNationality(ctx context.Context, name string) (*model.NationalityResult, error) {
	reqURL := "https://api.nationalize.io?name=" + url.QueryEscape(name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, &model.UpstreamError{Source: "Nationalize"}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &model.UpstreamError{Source: "Nationalize"}
	}
	defer resp.Body.Close()

	var payload struct {
		Country []model.CountryEntry `json:"country"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, &model.UpstreamError{Source: "Nationalize"}
	}

	if len(payload.Country) == 0 {
		return nil, &model.UpstreamError{Source: "Nationalize"}
	}

	// Inline max-probability selection to avoid circular dependency with service.
	topEntry := payload.Country[0]
	for _, entry := range payload.Country[1:] {
		if entry.Probability > topEntry.Probability {
			topEntry = entry
		}
	}

	return &model.NationalityResult{
		CountryID:   topEntry.CountryID,
		Probability: topEntry.Probability,
	}, nil
}
