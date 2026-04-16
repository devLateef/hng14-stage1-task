# Requirements Document

## Introduction

The Name Profile API is a Go REST service that accepts a person's name, enriches it by calling three external APIs (Genderize, Agify, Nationalize), applies classification logic to derive an age group and dominant nationality, persists the result in SQLite, and exposes CRUD endpoints to manage the stored profiles. The service is idempotent on name — submitting the same name twice returns the existing profile rather than creating a duplicate. All IDs are UUID v7, all timestamps are UTC ISO 8601, and CORS is open.

## Glossary

- **API**: The Name Profile REST service described in this document.
- **Profile**: A persisted record containing enriched name data (gender, age, nationality, classifications).
- **Enrichment_Client**: The component responsible for calling the three external APIs (Genderize, Agify, Nationalize).
- **Profile_Service**: The component that orchestrates enrichment, applies business logic, and coordinates persistence.
- **Profile_Handler**: The HTTP layer that parses requests, validates input, delegates to the service, and formats responses.
- **Profile_Repository**: The SQLite persistence layer for profiles.
- **ClassifyAge**: The pure function that maps a numeric age to an age-group label.
- **TopCountry**: The pure function that selects the country with the highest probability from a list.
- **ValidateName**: The pure function that checks whether a name string is non-empty after trimming whitespace.
- **UUID_v7**: A time-ordered universally unique identifier (version 7).
- **UTC_ISO_8601**: Timestamps formatted as `YYYY-MM-DDTHH:MM:SSZ` in the UTC timezone.
- **ErrNotFound**: Sentinel error returned when a profile cannot be located by ID or name.
- **ErrAlreadyExists**: Sentinel error returned when a profile with the given name already exists.
- **ErrMissingName**: Sentinel error returned when the request name is empty or whitespace-only.
- **UpstreamError**: Typed error returned when an external API returns null or invalid data.
- **AgeGroup**: One of the four classification labels: `child`, `teenager`, `adult`, `senior`.

---

## Requirements

### Requirement 1: Name Validation

**User Story:** As an API consumer, I want the service to reject requests with missing or empty names, so that only meaningful profiles are created.

#### Acceptance Criteria

1. WHEN a `POST /api/profiles` request is received with a missing `name` field, THEN THE Profile_Handler SHALL return HTTP 400 with `{"status":"error","message":"name is required"}`.
2. WHEN a `POST /api/profiles` request is received with a `name` field whose value is an empty string or contains only whitespace, THEN THE Profile_Handler SHALL return HTTP 400 with `{"status":"error","message":"name is required"}`.
3. WHEN a `POST /api/profiles` request body contains a `name` field with a non-string value (e.g., integer, boolean), THEN THE Profile_Handler SHALL return HTTP 422 with `{"status":"error","message":"invalid request body"}`.
4. WHEN a `POST /api/profiles` request body is malformed JSON, THEN THE Profile_Handler SHALL return HTTP 422 with `{"status":"error","message":"invalid request body"}`.
5. THE ValidateName function SHALL return `ErrMissingName` if and only if `strings.TrimSpace(name)` is the empty string.

---

### Requirement 2: Profile Creation

**User Story:** As an API consumer, I want to create a name profile by submitting a name, so that the service enriches and stores demographic data for that name.

#### Acceptance Criteria

1. WHEN a valid `POST /api/profiles` request is received with a new name, THE Profile_Service SHALL call the Enrichment_Client to fetch gender, age, and nationality data for that name.
2. WHEN all three enrichment calls succeed, THE Profile_Service SHALL apply `ClassifyAge` to the returned age and `TopCountry` to the returned country list to derive `age_group` and `country_id`.
3. WHEN all enrichment and classification steps succeed, THE Profile_Service SHALL generate a UUID_v7 `id` and a UTC_ISO_8601 `created_at` timestamp, then persist the profile via the Profile_Repository.
4. WHEN a profile is successfully created, THE Profile_Handler SHALL return HTTP 201 with `{"status":"success","data":{<profile object>}}`.
5. WHEN a `POST /api/profiles` request is received for a name that already exists, THE Profile_Handler SHALL return HTTP 200 with `{"status":"success","message":"Profile already exists","data":{<existing profile object>}}` without creating a duplicate record.
6. THE Profile_Repository SHALL enforce uniqueness on the `name` column so that no two profiles share the same name.

---

### Requirement 3: Age Classification

**User Story:** As an API consumer, I want the service to classify a numeric age into a human-readable group, so that profiles carry a meaningful demographic label.

#### Acceptance Criteria

1. WHEN `ClassifyAge` is called with an age in the range `[0, 12]`, THE ClassifyAge function SHALL return `"child"`.
2. WHEN `ClassifyAge` is called with an age in the range `[13, 19]`, THE ClassifyAge function SHALL return `"teenager"`.
3. WHEN `ClassifyAge` is called with an age in the range `[20, 59]`, THE ClassifyAge function SHALL return `"adult"`.
4. WHEN `ClassifyAge` is called with an age of `60` or greater, THE ClassifyAge function SHALL return `"senior"`.
5. THE ClassifyAge function SHALL return exactly one of `"child"`, `"teenager"`, `"adult"`, or `"senior"` for any non-negative integer input.

---

### Requirement 4: Nationality Selection

**User Story:** As an API consumer, I want the service to select the most probable nationality from the Nationalize response, so that profiles carry a single representative country.

#### Acceptance Criteria

1. WHEN `TopCountry` is called with a non-empty list of country entries, THE TopCountry function SHALL return the entry with the highest `Probability` value.
2. WHEN `TopCountry` is called with multiple entries sharing the highest probability, THE TopCountry function SHALL return the first such entry in the list.
3. WHEN `TopCountry` is called with an empty list, THE TopCountry function SHALL return `ErrNoCountryData`.

---

### Requirement 5: External API Enrichment

**User Story:** As an API consumer, I want the service to fetch gender, age, and nationality data from external APIs, so that profiles are enriched with real demographic information.

#### Acceptance Criteria

1. WHEN `FetchGender` is called, THE Enrichment_Client SHALL make an HTTP GET request to `https://api.genderize.io?name={url-encoded-name}` and decode the JSON response into a typed `GenderResult`.
2. WHEN `FetchAge` is called, THE Enrichment_Client SHALL make an HTTP GET request to `https://api.agify.io?name={url-encoded-name}` and decode the JSON response into a typed `AgeResult`.
3. WHEN `FetchNationality` is called, THE Enrichment_Client SHALL make an HTTP GET request to `https://api.nationalize.io?name={url-encoded-name}` and decode the JSON response into a typed `NationalityResult`.
4. WHEN the Genderize API returns a response where `gender` is `null` or `count` is `0`, THE Enrichment_Client SHALL return an `UpstreamError` with `Source` set to `"Genderize"`.
5. WHEN the Agify API returns a response where `age` is `null`, THE Enrichment_Client SHALL return an `UpstreamError` with `Source` set to `"Agify"`.
6. WHEN the Nationalize API returns a response with an empty country array, THE Enrichment_Client SHALL return an `UpstreamError` with `Source` set to `"Nationalize"`.
7. WHEN any enrichment call returns an `UpstreamError`, THE Profile_Handler SHALL return HTTP 502 with `{"status":"error","message":"<Source> returned an invalid response"}` and THE Profile_Service SHALL NOT persist a profile.
8. WHEN an enrichment HTTP call fails due to a network error, THE Enrichment_Client SHALL return an error that causes THE Profile_Handler to return HTTP 502.

---

### Requirement 6: Retrieve Single Profile

**User Story:** As an API consumer, I want to retrieve a profile by its ID, so that I can inspect the stored demographic data for a specific name.

#### Acceptance Criteria

1. WHEN a `GET /api/profiles/{id}` request is received with a valid existing ID, THE Profile_Handler SHALL return HTTP 200 with `{"status":"success","data":{<profile object>}}`.
2. WHEN a `GET /api/profiles/{id}` request is received with an ID that does not exist, THE Profile_Handler SHALL return HTTP 404 with `{"status":"error","message":"profile not found"}`.

---

### Requirement 7: List Profiles with Filters

**User Story:** As an API consumer, I want to list all profiles with optional filters, so that I can query profiles by gender, country, or age group.

#### Acceptance Criteria

1. WHEN a `GET /api/profiles` request is received with no query parameters, THE Profile_Handler SHALL return HTTP 200 with `{"status":"success","data":[<all profiles>]}`.
2. WHEN a `GET /api/profiles` request is received with a `gender` query parameter, THE Profile_Repository SHALL return only profiles whose `gender` field matches the parameter value case-insensitively.
3. WHEN a `GET /api/profiles` request is received with a `country_id` query parameter, THE Profile_Repository SHALL return only profiles whose `country_id` field matches the parameter value case-insensitively.
4. WHEN a `GET /api/profiles` request is received with an `age_group` query parameter, THE Profile_Repository SHALL return only profiles whose `age_group` field matches the parameter value case-insensitively.
5. WHEN multiple filter parameters are provided, THE Profile_Repository SHALL apply all filters conjunctively (AND logic).
6. WHEN no profiles match the applied filters, THE Profile_Handler SHALL return HTTP 200 with `{"status":"success","data":[]}`.

---

### Requirement 8: Delete Profile

**User Story:** As an API consumer, I want to delete a profile by its ID, so that I can remove stored data for a name.

#### Acceptance Criteria

1. WHEN a `DELETE /api/profiles/{id}` request is received with a valid existing ID, THE Profile_Handler SHALL return HTTP 204 with no response body.
2. WHEN a `DELETE /api/profiles/{id}` request is received with an ID that does not exist, THE Profile_Handler SHALL return HTTP 404 with `{"status":"error","message":"profile not found"}`.
3. WHEN a profile is deleted, THE Profile_Repository SHALL remove the record so that subsequent `GET /api/profiles/{id}` requests for the same ID return 404.

---

### Requirement 9: Profile Data Model

**User Story:** As an API consumer, I want profile responses to follow a consistent structure, so that I can reliably parse and use the returned data.

#### Acceptance Criteria

1. THE API SHALL include the following fields in every profile object: `id`, `name`, `gender`, `gender_probability`, `sample_size`, `age`, `age_group`, `country_id`, `country_probability`, `created_at`.
2. THE API SHALL represent `id` as a UUID_v7 string.
3. THE API SHALL represent `created_at` as a UTC_ISO_8601 string.
4. THE API SHALL represent `age_group` as one of `"child"`, `"teenager"`, `"adult"`, or `"senior"`.
5. THE Profile_Repository SHALL persist profiles in a SQLite table with a `UNIQUE` constraint on the `name` column.

---

### Requirement 10: CORS and HTTP Headers

**User Story:** As a browser-based API consumer, I want the service to include CORS headers, so that I can call the API from any web origin.

#### Acceptance Criteria

1. THE Profile_Handler SHALL include the HTTP response header `Access-Control-Allow-Origin: *` on every response.

---

### Requirement 11: Persistence and Idempotency

**User Story:** As an API consumer, I want the service to be idempotent on name, so that repeated creation requests for the same name do not produce duplicate records.

#### Acceptance Criteria

1. WHEN `POST /api/profiles` is called twice with the same name, THE Profile_Service SHALL return the same profile `id` on both calls.
2. WHEN `POST /api/profiles` is called twice with the same name, THE Profile_Repository SHALL contain exactly one record for that name.
3. THE Profile_Service SHALL check for an existing profile by name before calling the Enrichment_Client, so that external API calls are not made for duplicate names.

---

### Requirement 12: Serialization and Deserialization

**User Story:** As a developer, I want profile data to be correctly serialized to and deserialized from JSON and SQLite, so that stored and returned data is always consistent.

#### Acceptance Criteria

1. WHEN a `Profile` struct is serialized to JSON, THE API SHALL produce a JSON object containing all required fields with correct types.
2. WHEN a profile row is read from SQLite and mapped to a `Profile` struct, THE Profile_Repository SHALL produce a struct whose fields match the stored values exactly.
3. THE Profile_Repository SHALL store and retrieve `created_at` as a UTC_ISO_8601 string without loss of precision.
4. FOR ALL valid `Profile` objects, serializing to JSON and then deserializing SHALL produce an equivalent `Profile` object (round-trip property).
