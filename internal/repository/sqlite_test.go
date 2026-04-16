package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"name-profile-api/internal/model"

	_ "modernc.org/sqlite"
	"pgregory.net/rapid"
)

// newTestDB opens an in-memory SQLite database using the pure-Go modernc driver
// (no CGO required) and returns a ready-to-use ProfileRepository.
func newTestDB(t *testing.T) (ProfileRepository, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	// Run schema migration inline (same SQL as the production schema const).
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS profiles (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    gender              TEXT NOT NULL,
    gender_probability  REAL NOT NULL,
    sample_size         INTEGER NOT NULL,
    age                 INTEGER NOT NULL,
    age_group           TEXT NOT NULL,
    country_id          TEXT NOT NULL,
    country_probability REAL NOT NULL,
    created_at          TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_profiles_name ON profiles(name);
CREATE INDEX IF NOT EXISTS idx_profiles_gender ON profiles(LOWER(gender));
CREATE INDEX IF NOT EXISTS idx_profiles_country_id ON profiles(LOWER(country_id));
CREATE INDEX IF NOT EXISTS idx_profiles_age_group ON profiles(LOWER(age_group));
`)
	if err != nil {
		t.Fatalf("schema migration: %v", err)
	}
	repo := &sqliteRepo{db: db}
	return repo, db
}

// ---------------------------------------------------------------------------
// Property 9: Profile SQLite round-trip
// Validates: Requirements 12.2, 12.3
// ---------------------------------------------------------------------------

func TestProperty9_SQLiteRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		repo, db := newTestDB(t)
		defer db.Close()

		ctx := context.Background()
		p := generateProfile(rt)

		if err := repo.Insert(ctx, p); err != nil {
			rt.Fatalf("Insert failed: %v", err)
		}

		got, err := repo.FindByID(ctx, p.ID)
		if err != nil {
			rt.Fatalf("FindByID failed: %v", err)
		}

		assertProfilesEqual(rt, p, got)
	})
}

// generateProfile produces a random model.Profile using rapid generators.
// created_at is truncated to second precision to match RFC3339 round-trip.
func generateProfile(t *rapid.T) *model.Profile {
	id := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rapid.IntRange(0, 0xFFFFFFFF).Draw(t, "id_p1"),
		rapid.IntRange(0, 0xFFFF).Draw(t, "id_p2"),
		rapid.IntRange(0, 0xFFFF).Draw(t, "id_p3"),
		rapid.IntRange(0, 0xFFFF).Draw(t, "id_p4"),
		rapid.IntRange(0, 0xFFFFFFFFFFFF).Draw(t, "id_p5"),
	)

	name := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9_-]{0,30}`).Draw(t, "name")
	gender := rapid.SampledFrom([]string{"male", "female", "nonbinary"}).Draw(t, "gender")
	genderProb := rapid.Float64Range(0.0, 1.0).Draw(t, "gender_probability")
	sampleSize := rapid.IntRange(1, 100000).Draw(t, "sample_size")
	age := rapid.IntRange(0, 120).Draw(t, "age")
	ageGroup := rapid.SampledFrom([]string{"child", "teenager", "adult", "senior"}).Draw(t, "age_group")
	countryID := rapid.StringMatching(`[A-Z]{2}`).Draw(t, "country_id")
	countryProb := rapid.Float64Range(0.0, 1.0).Draw(t, "country_probability")

	baseTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	offsetSecs := rapid.IntRange(0, 10*365*24*3600).Draw(t, "created_at_offset")
	createdAt := baseTime.Add(time.Duration(offsetSecs) * time.Second).UTC()

	return &model.Profile{
		ID:                 id,
		Name:               name,
		Gender:             gender,
		GenderProbability:  genderProb,
		SampleSize:         sampleSize,
		Age:                age,
		AgeGroup:           ageGroup,
		CountryID:          countryID,
		CountryProbability: countryProb,
		CreatedAt:          createdAt,
	}
}

func assertProfilesEqual(t *rapid.T, want, got *model.Profile) {
	t.Helper()
	if got.ID != want.ID {
		t.Fatalf("ID: want %q, got %q", want.ID, got.ID)
	}
	if got.Name != want.Name {
		t.Fatalf("Name: want %q, got %q", want.Name, got.Name)
	}
	if got.Gender != want.Gender {
		t.Fatalf("Gender: want %q, got %q", want.Gender, got.Gender)
	}
	if got.GenderProbability != want.GenderProbability {
		t.Fatalf("GenderProbability: want %v, got %v", want.GenderProbability, got.GenderProbability)
	}
	if got.SampleSize != want.SampleSize {
		t.Fatalf("SampleSize: want %d, got %d", want.SampleSize, got.SampleSize)
	}
	if got.Age != want.Age {
		t.Fatalf("Age: want %d, got %d", want.Age, got.Age)
	}
	if got.AgeGroup != want.AgeGroup {
		t.Fatalf("AgeGroup: want %q, got %q", want.AgeGroup, got.AgeGroup)
	}
	if got.CountryID != want.CountryID {
		t.Fatalf("CountryID: want %q, got %q", want.CountryID, got.CountryID)
	}
	if got.CountryProbability != want.CountryProbability {
		t.Fatalf("CountryProbability: want %v, got %v", want.CountryProbability, got.CountryProbability)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("CreatedAt: want %v, got %v", want.CreatedAt, got.CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for repository filter logic (Task 6.4)
// ---------------------------------------------------------------------------

func makeProfile(id, name, gender, countryID, ageGroup string) *model.Profile {
	return &model.Profile{
		ID:                 id,
		Name:               name,
		Gender:             gender,
		GenderProbability:  0.9,
		SampleSize:         100,
		Age:                25,
		AgeGroup:           ageGroup,
		CountryID:          countryID,
		CountryProbability: 0.5,
		CreatedAt:          time.Now().UTC().Truncate(time.Second),
	}
}

func TestList_GenderFilter(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, p := range []*model.Profile{
		makeProfile("id-1", "Alice", "female", "US", "adult"),
		makeProfile("id-2", "Bob", "male", "US", "adult"),
		makeProfile("id-3", "Charlie", "male", "GB", "adult"),
	} {
		if err := repo.Insert(ctx, p); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	results, err := repo.List(ctx, model.ProfileFilters{Gender: "female"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 || results[0].ID != "id-1" {
		t.Errorf("expected [id-1], got %v", results)
	}
}

func TestList_CountryIDFilter(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, p := range []*model.Profile{
		makeProfile("id-1", "Alice", "female", "US", "adult"),
		makeProfile("id-2", "Bob", "male", "US", "adult"),
		makeProfile("id-3", "Charlie", "male", "GB", "adult"),
	} {
		if err := repo.Insert(ctx, p); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	results, err := repo.List(ctx, model.ProfileFilters{CountryID: "GB"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 || results[0].ID != "id-3" {
		t.Errorf("expected [id-3], got %v", results)
	}
}

func TestList_AgeGroupFilter(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, p := range []*model.Profile{
		makeProfile("id-1", "Alice", "female", "US", "adult"),
		makeProfile("id-2", "Bob", "male", "US", "teenager"),
		makeProfile("id-3", "Charlie", "male", "GB", "senior"),
	} {
		if err := repo.Insert(ctx, p); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	results, err := repo.List(ctx, model.ProfileFilters{AgeGroup: "teenager"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 || results[0].ID != "id-2" {
		t.Errorf("expected [id-2], got %v", results)
	}
}

func TestList_MultipleFilters_ANDLogic(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, p := range []*model.Profile{
		makeProfile("id-1", "Alice", "female", "US", "adult"),
		makeProfile("id-2", "Bob", "male", "US", "adult"),
		makeProfile("id-3", "Carol", "female", "GB", "adult"),
		makeProfile("id-4", "Diana", "female", "US", "senior"),
	} {
		if err := repo.Insert(ctx, p); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	results, err := repo.List(ctx, model.ProfileFilters{Gender: "female", CountryID: "US", AgeGroup: "adult"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 || results[0].ID != "id-1" {
		t.Errorf("expected [id-1], got %v", results)
	}
}

func TestList_CaseInsensitiveGenderFilter(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	p := makeProfile("id-1", "Bob", "male", "US", "adult")
	if err := repo.Insert(ctx, p); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	results, err := repo.List(ctx, model.ProfileFilters{Gender: "MALE"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 || results[0].ID != "id-1" {
		t.Errorf("expected [id-1] for case-insensitive match, got %v", results)
	}
}

func TestList_NoFilters_ReturnsAll(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, p := range []*model.Profile{
		makeProfile("id-1", "Alice", "female", "US", "adult"),
		makeProfile("id-2", "Bob", "male", "GB", "teenager"),
	} {
		if err := repo.Insert(ctx, p); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	results, err := repo.List(ctx, model.ProfileFilters{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestList_NoMatch_ReturnsEmptySlice(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	p := makeProfile("id-1", "Alice", "female", "US", "adult")
	if err := repo.Insert(ctx, p); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	results, err := repo.List(ctx, model.ProfileFilters{Gender: "nonbinary"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestInsert_DuplicateName_ReturnsErrAlreadyExists(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	p1 := makeProfile("id-1", "Alice", "female", "US", "adult")
	p2 := makeProfile("id-2", "Alice", "female", "US", "adult")

	if err := repo.Insert(ctx, p1); err != nil {
		t.Fatalf("first Insert: %v", err)
	}
	if err := repo.Insert(ctx, p2); err != model.ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestFindByID_NotFound(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()

	_, err := repo.FindByID(context.Background(), "nonexistent-id")
	if err != model.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFindByName_NotFound(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()

	_, err := repo.FindByName(context.Background(), "nobody")
	if err != model.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete_ExistingProfile(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	p := makeProfile("id-1", "Alice", "female", "US", "adult")
	if err := repo.Insert(ctx, p); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := repo.Delete(ctx, "id-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, "id-1"); err != model.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo, db := newTestDB(t)
	defer db.Close()

	if err := repo.Delete(context.Background(), "nonexistent-id"); err != model.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
