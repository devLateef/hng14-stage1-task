package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"name-profile-api/internal/model"
)

const schema = `
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
`

// ProfileRepository defines the persistence operations for Profile entities.
type ProfileRepository interface {
	Insert(ctx context.Context, p *model.Profile) error
	FindByID(ctx context.Context, id string) (*model.Profile, error)
	FindByName(ctx context.Context, name string) (*model.Profile, error)
	List(ctx context.Context, filters model.ProfileFilters) ([]model.Profile, error)
	Delete(ctx context.Context, id string) error
}

type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed ProfileRepository and runs
// the schema migration to ensure the profiles table and indexes exist.
func NewSQLiteRepository(db *sql.DB) ProfileRepository {
	db.Exec(schema) //nolint:errcheck // schema is a trusted constant
	return &sqliteRepo{db: db}
}

// Insert persists a new Profile. Returns model.ErrAlreadyExists if a profile
// with the same name already exists.
func (r *sqliteRepo) Insert(ctx context.Context, p *model.Profile) error {
	const q = `INSERT INTO profiles
		(id, name, gender, gender_probability, sample_size, age, age_group,
		 country_id, country_probability, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	createdAt := p.CreatedAt.UTC().Format(time.RFC3339)

	_, err := r.db.ExecContext(ctx, q,
		p.ID,
		p.Name,
		p.Gender,
		p.GenderProbability,
		p.SampleSize,
		p.Age,
		p.AgeGroup,
		p.CountryID,
		p.CountryProbability,
		createdAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return model.ErrAlreadyExists
		}
		return err
	}
	return nil
}

// FindByID retrieves a Profile by its UUID. Returns model.ErrNotFound when no
// row matches.
func (r *sqliteRepo) FindByID(ctx context.Context, id string) (*model.Profile, error) {
	const q = `SELECT id, name, gender, gender_probability, sample_size, age,
		age_group, country_id, country_probability, created_at
		FROM profiles WHERE id = ?`

	row := r.db.QueryRowContext(ctx, q, id)
	return scanProfile(row)
}

// FindByName retrieves a Profile by its name. Returns model.ErrNotFound when
// no row matches.
func (r *sqliteRepo) FindByName(ctx context.Context, name string) (*model.Profile, error) {
	const q = `SELECT id, name, gender, gender_probability, sample_size, age,
		age_group, country_id, country_probability, created_at
		FROM profiles WHERE name = ?`

	row := r.db.QueryRowContext(ctx, q, name)
	return scanProfile(row)
}

// List returns all profiles that match the supplied filters. Each non-empty
// filter is applied as LOWER(col) = LOWER(?). Returns an empty (non-nil) slice
// when no rows match.
func (r *sqliteRepo) List(ctx context.Context, filters model.ProfileFilters) ([]model.Profile, error) {
	query := `SELECT id, name, gender, gender_probability, sample_size, age,
		age_group, country_id, country_probability, created_at
		FROM profiles WHERE 1=1`
	args := []interface{}{}

	if filters.Gender != "" {
		query += " AND LOWER(gender) = LOWER(?)"
		args = append(args, filters.Gender)
	}
	if filters.CountryID != "" {
		query += " AND LOWER(country_id) = LOWER(?)"
		args = append(args, filters.CountryID)
	}
	if filters.AgeGroup != "" {
		query += " AND LOWER(age_group) = LOWER(?)"
		args = append(args, filters.AgeGroup)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	profiles := []model.Profile{}
	for rows.Next() {
		var createdAtStr string
		var p model.Profile
		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.Gender,
			&p.GenderProbability,
			&p.SampleSize,
			&p.Age,
			&p.AgeGroup,
			&p.CountryID,
			&p.CountryProbability,
			&createdAtStr,
		); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, err
		}
		p.CreatedAt = t.UTC()
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return profiles, nil
}

// Delete removes a Profile by its UUID. Returns model.ErrNotFound when no row
// was deleted.
func (r *sqliteRepo) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM profiles WHERE id = ?`
	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// scanProfile scans a single row into a Profile struct.
func scanProfile(row *sql.Row) (*model.Profile, error) {
	var createdAtStr string
	var p model.Profile
	err := row.Scan(
		&p.ID,
		&p.Name,
		&p.Gender,
		&p.GenderProbability,
		&p.SampleSize,
		&p.Age,
		&p.AgeGroup,
		&p.CountryID,
		&p.CountryProbability,
		&createdAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, err
	}
	p.CreatedAt = t.UTC()
	return &p, nil
}
