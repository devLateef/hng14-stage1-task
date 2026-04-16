# Implementation Plan: Name Profile API

## Overview

Implement a Go REST API that accepts a name, enriches it via three external APIs (Genderize, Agify, Nationalize), persists the result in SQLite, and exposes four CRUD endpoints. The project is built in the workspace root using a standard Go layout. The existing `demo.go` and `demo.exe` files are removed as the first step.

## Tasks

- [x] 1. Bootstrap project structure
  - Delete `demo.go` and `demo.exe` from the workspace root
  - Create `go.mod` with module name `name-profile-api` and Go 1.22+
  - Run `go get` to add dependencies: `github.com/go-chi/chi/v5`, `github.com/mattn/go-sqlite3`, `github.com/google/uuid`, `pgregory.net/rapid`
  - Create the directory tree: `internal/model`, `internal/handler`, `internal/service`, `internal/enrichment`, `internal/repository`, `db/`
  - _Requirements: 9.1, 9.2, 9.3_

- [x] 2. Define shared data models and sentinel errors
  - [x] 2.1 Create `internal/model/profile.go`
    - Define `Profile` struct with all JSON tags matching the spec (`id`, `name`, `gender`, `gender_probability`, `sample_size`, `age`, `age_group`, `country_id`, `country_probability`, `created_at`)
    - Define `CreateProfileRequest`, `APIResponse`, `ErrorResponse` structs
    - Define `ProfileFilters` struct (`Gender`, `CountryID`, `AgeGroup`)
    - Define sentinel errors: `ErrNotFound`, `ErrAlreadyExists`, `ErrMissingName`
    - Define `UpstreamError` struct with `Source` field and `Error()` method
    - Define enrichment result types: `GenderResult`, `AgeResult`, `NationalityResult`, `CountryEntry`
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 1.5, 5.4, 5.5, 5.6_

- [x] 3. Implement pure business-logic functions and property tests
  - [x] 3.1 Create `internal/service/classify.go`
    - Implement `ClassifyAge(age int) string` with the four range branches: `[0,12]`→`"child"`, `[13,19]`→`"teenager"`, `[20,59]`→`"adult"`, `≥60`→`"senior"`
    - Implement `TopCountry(countries []model.CountryEntry) (string, float64, error)` — return first max-probability entry or `ErrNoCountryData` on empty slice
    - Implement `ValidateName(name string) error` — return `ErrMissingName` when `strings.TrimSpace(name) == ""`
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 4.1, 4.2, 4.3, 1.5_

  - [x] 3.2 Write property test for `ClassifyAge` — Property 1
    - Create `internal/service/classify_test.go`
    - **Property 1: ClassifyAge covers all non-negative ages with correct labels**
    - Use `rapid.Int().Filter(...)` to generate ages in each sub-range and assert the correct label
    - Also assert the result is always one of the four valid labels for any non-negative age
    - **Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5**

  - [x] 3.3 Write property test for `TopCountry` — Property 2
    - **Property 2: TopCountry returns the maximum-probability entry**
    - Use `rapid.SliceOf(...)` to generate non-empty slices of `CountryEntry` and assert the returned probability equals the slice maximum
    - **Validates: Requirements 4.1, 4.2**

  - [x] 3.4 Write property test for `ValidateName` — Property 3
    - **Property 3: ValidateName rejects exactly the whitespace-only inputs**
    - Generate arbitrary strings; assert `ValidateName` returns `ErrMissingName` iff `strings.TrimSpace(s) == ""`
    - **Validates: Requirements 1.2, 1.5**

  - [x] 3.5 Write unit tests for `ClassifyAge` boundary values
    - Table-driven tests covering: 0, 12, 13, 19, 20, 59, 60, 100
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

  - [x] 3.6 Write unit tests for `TopCountry` edge cases
    - Empty slice → error; single entry; multiple entries with tie (first wins)
    - _Requirements: 4.1, 4.2, 4.3_

- [x] 4. Checkpoint — Ensure all tests pass
  - Run `go test ./internal/service/...` and confirm all pass. Ask the user if questions arise.

- [x] 5. Implement the enrichment client
  - [x] 5.1 Create `internal/enrichment/client.go`
    - Define `EnrichmentClient` interface with `FetchGender`, `FetchAge`, `FetchNationality`
    - Implement `httpEnrichmentClient` struct holding an `*http.Client`
    - Implement `NewEnrichmentClient(httpClient *http.Client) EnrichmentClient`
    - Implement `FetchGender`: GET `https://api.genderize.io?name={url.QueryEscape(name)}`, decode into anonymous struct with `*string gender`, `float64 probability`, `int count`; return `UpstreamError{Source:"Genderize"}` when `gender == nil || count == 0`
    - Implement `FetchAge`: GET `https://api.agify.io?name={url.QueryEscape(name)}`, decode into anonymous struct with `*int age`, `int count`; return `UpstreamError{Source:"Agify"}` when `age == nil`
    - Implement `FetchNationality`: GET `https://api.nationalize.io?name={url.QueryEscape(name)}`, decode into anonymous struct with `[]CountryEntry country`; call `TopCountry`; return `UpstreamError{Source:"Nationalize"}` when country array is empty
    - Propagate network errors as-is (handler maps them to 502)
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8_

  - [x] 5.2 Write unit tests for enrichment client
    - Use `httptest.NewServer` to mock each external API
    - Test null gender, null age, empty country array → `UpstreamError`
    - Test network failure → error propagated
    - _Requirements: 5.4, 5.5, 5.6, 5.7, 5.8_

- [x] 6. Implement the SQLite repository
  - [x] 6.1 Create `db/schema.sql`
    - Write `CREATE TABLE IF NOT EXISTS profiles` with all columns and `UNIQUE` constraint on `name`
    - Add indexes on `name`, `LOWER(gender)`, `LOWER(country_id)`, `LOWER(age_group)`
    - _Requirements: 9.5, 11.2_

  - [x] 6.2 Create `internal/repository/sqlite.go`
    - Define `ProfileRepository` interface with `Insert`, `FindByID`, `FindByName`, `List`, `Delete`
    - Implement `sqliteRepo` struct holding `*sql.DB`
    - Implement `NewSQLiteRepository(db *sql.DB) ProfileRepository` — runs schema migration from embedded SQL or inline string
    - Implement `Insert`: parameterized INSERT; return `ErrAlreadyExists` on UNIQUE constraint violation
    - Implement `FindByID`: SELECT by `id`; return `ErrNotFound` on `sql.ErrNoRows`
    - Implement `FindByName`: SELECT by `name`; return `ErrNotFound` on `sql.ErrNoRows`
    - Implement `List`: dynamic WHERE clause with `LOWER(col) = LOWER(?)` for each non-empty filter; return empty slice (not nil) when no rows
    - Implement `Delete`: DELETE by `id`; check `RowsAffected() == 0` → return `ErrNotFound`
    - Store and retrieve `created_at` as UTC ISO 8601 string (`time.RFC3339`)
    - _Requirements: 6.1, 6.2, 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 8.1, 8.2, 8.3, 9.5, 11.2, 12.2, 12.3_

  - [x] 6.3 Write property test for SQLite round-trip — Property 9
    - Create `internal/repository/sqlite_test.go`
    - **Property 9: Profile SQLite round-trip**
    - Use `rapid` to generate arbitrary `Profile` values; insert then `FindByID`; assert all fields equal including `created_at` at UTC ISO 8601 precision
    - **Validates: Requirements 12.2, 12.3**

  - [x] 6.4 Write unit tests for repository filter logic
    - Insert several profiles with varying gender/country/age_group; assert `List` with each filter combination returns only matching profiles
    - _Requirements: 7.2, 7.3, 7.4, 7.5_

- [x] 7. Checkpoint — Ensure all tests pass
  - Run `go test ./internal/repository/...` and confirm all pass. Ask the user if questions arise.

- [x] 8. Implement the profile service
  - [x] 8.1 Create `internal/service/profile.go`
    - Define `ProfileService` interface with `CreateProfile`, `GetProfile`, `ListProfiles`, `DeleteProfile`
    - Implement `profileService` struct holding `repo ProfileRepository` and `enrichment EnrichmentClient`
    - Implement `NewProfileService(repo, enrichment) ProfileService`
    - Implement `CreateProfile`: call `ValidateName`; call `repo.FindByName` (return existing + `ErrAlreadyExists` if found); call `FetchGender`, `FetchAge`, `FetchNationality` sequentially; call `ClassifyAge` and `TopCountry`; generate UUID v7 via `uuid.New()` (v7); set `CreatedAt = time.Now().UTC()`; call `repo.Insert`; return profile
    - Implement `GetProfile`: delegate to `repo.FindByID`
    - Implement `ListProfiles`: delegate to `repo.List`
    - Implement `DeleteProfile`: delegate to `repo.Delete`
    - _Requirements: 2.1, 2.2, 2.3, 2.5, 6.1, 6.2, 7.1, 8.1, 8.2, 11.1, 11.2, 11.3_

  - [x] 8.2 Write property test for idempotency — Property 7
    - Create `internal/service/profile_test.go`
    - **Property 7: Profile creation idempotency**
    - Use an in-memory mock repository; call `CreateProfile` twice with the same name; assert same `id` returned and repository contains exactly one record
    - **Validates: Requirements 11.1, 11.2**

  - [x] 8.3 Write property test for upstream errors — Property 4
    - **Property 4: Upstream errors never result in a stored profile**
    - Use `rapid` to pick which enrichment call fails; assert `repo.Insert` is never called and service returns an `UpstreamError`
    - **Validates: Requirements 5.4, 5.5, 5.6, 5.7**

  - [x] 8.4 Write unit tests for service layer
    - Test `CreateProfile` with mock enrichment returning valid data → 201 path
    - Test `CreateProfile` with each enrichment returning `UpstreamError` → error propagated, no insert
    - Test `GetProfile` with unknown ID → `ErrNotFound`
    - Test `DeleteProfile` with unknown ID → `ErrNotFound`
    - _Requirements: 2.1, 2.2, 2.3, 5.7, 6.2, 8.2_

- [x] 9. Checkpoint — Ensure all tests pass
  - Run `go test ./internal/service/...` and confirm all pass. Ask the user if questions arise.

- [x] 10. Implement the HTTP handler
  - [x] 10.1 Create `internal/handler/profile.go`
    - Define `ProfileHandler` struct holding a `ProfileService`
    - Implement `NewProfileHandler(service ProfileService) *ProfileHandler`
    - Implement `writeJSON` helper that sets `Content-Type: application/json`, `Access-Control-Allow-Origin: *`, and writes the status code + JSON body
    - Implement `CreateProfile`: decode JSON body; on decode error return 422 `{"status":"error","message":"invalid request body"}`; call `service.CreateProfile`; map `ErrMissingName`→400, `ErrAlreadyExists`→200 with message, `*UpstreamError`→502, success→201
    - Implement `GetProfile`: extract `{id}` via `chi.URLParam`; call `service.GetProfile`; map `ErrNotFound`→404, success→200
    - Implement `ListProfiles`: read `gender`, `country_id`, `age_group` query params; call `service.ListProfiles`; return 200 with array (empty array when no results)
    - Implement `DeleteProfile`: extract `{id}`; call `service.DeleteProfile`; map `ErrNotFound`→404, success→204 no body
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.4, 2.5, 5.7, 6.1, 6.2, 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 8.1, 8.2, 10.1_

  - [x] 10.2 Write property test for CORS header — Property 10
    - Create `internal/handler/profile_test.go`
    - **Property 10: Every response carries the CORS header**
    - Use `rapid` to pick any endpoint and any outcome (success/error); assert `Access-Control-Allow-Origin: *` is present on every response
    - **Validates: Requirements 10.1**

  - [x] 10.3 Write property test for JSON round-trip — Property 8
    - **Property 8: Profile JSON round-trip**
    - Use `rapid` to generate arbitrary `Profile` structs; marshal to JSON then unmarshal; assert all fields equal
    - **Validates: Requirements 12.1, 12.4**

  - [x] 10.4 Write unit tests for handler error mapping
    - Use `httptest.NewRecorder` and a mock service
    - Test missing name → 400; malformed JSON → 422; duplicate name → 200 with message; upstream error → 502; not found → 404; delete success → 204
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.4, 2.5, 5.7, 6.2, 8.2_

- [x] 11. Checkpoint — Ensure all tests pass
  - Run `go test ./internal/handler/...` and confirm all pass. Ask the user if questions arise.

- [x] 12. Wire everything together in `main.go`
  - [x] 12.1 Create `main.go`
    - Open SQLite database file (`profiles.db`) with `sql.Open("sqlite3", "profiles.db?_journal_mode=WAL")`
    - Instantiate `sqliteRepo`, `httpEnrichmentClient` (with 5 s timeout `http.Client`), `profileService`, `profileHandler`
    - Create a `chi.Router`; register routes: `POST /api/profiles`, `GET /api/profiles`, `GET /api/profiles/{id}`, `DELETE /api/profiles/{id}`
    - Start `http.ListenAndServe(":8080", r)`; log fatal on error
    - _Requirements: 2.1, 6.1, 7.1, 8.1_

  - [x] 12.2 Write integration tests for full request lifecycle
    - Create `integration_test.go` (or `internal/handler/integration_test.go`)
    - Use `httptest.NewServer` with the real router, an in-memory SQLite (`:memory:`), and a mock HTTP transport for external APIs
    - Test: create → get → list → delete lifecycle
    - Test idempotency: two POSTs with same name return same `id`
    - Test filter: create profiles with different genders; list with `?gender=female` returns only female profiles
    - _Requirements: 2.4, 2.5, 6.1, 7.2, 8.1, 8.3, 11.1, 11.2_

- [x] 13. Final checkpoint — Ensure all tests pass
  - Run `go test ./...` and confirm all tests pass. Verify `go build ./...` succeeds with no errors. Ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for a faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation at each layer boundary
- Property tests use `pgregory.net/rapid` and validate universal correctness properties
- Unit tests validate specific examples and boundary conditions
- The `demo.go` / `demo.exe` files are removed in Task 1 before any new code is written
- SQLite is opened with WAL mode for concurrent read performance
- All SQL queries use parameterized statements — no string interpolation of user input
