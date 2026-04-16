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
