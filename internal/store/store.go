// Package store provides SQLite persistence for settings, favorites, and custom food images.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database.
type Store struct {
	db *sql.DB
}

// FoodImage is a user-supplied image URL for a food item.
type FoodImage struct {
	FoodName  string
	ImageURL  string
	UpdatedAt time.Time
}

// Favorite is a starred food item.
type Favorite struct {
	ID         int64
	FoodName   string
	SchoolSlug string // empty = any school
	CreatedAt  time.Time
}

// Exclusion is a summary-exclusion pattern (case-insensitive substring match).
type Exclusion struct {
	ID         int64
	SchoolSlug string // empty = all schools
	Pattern    string
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS food_images (
			food_name  TEXT PRIMARY KEY,
			image_url  TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS favorites (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			food_name   TEXT NOT NULL,
			school_slug TEXT NOT NULL DEFAULT '',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(food_name, school_slug)
		);

		CREATE TABLE IF NOT EXISTS summary_exclusions (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			school_slug TEXT NOT NULL DEFAULT '',
			pattern     TEXT NOT NULL,
			UNIQUE(school_slug, pattern)
		);

		-- Seed default exclusions for Woodmen Roberts (sun butter is a standing alternative)
		INSERT OR IGNORE INTO summary_exclusions (school_slug, pattern)
		VALUES ('woodmen-roberts-elementary-school', 'sun butter');
	`)
	return err
}

// ── Food Images ───────────────────────────────────────────────────────────────

// ResolveImage returns the best image URL for a food item.
// API-provided images take priority; custom images fill the gap.
func (s *Store) ResolveImage(foodName, apiImageURL string) string {
	if apiImageURL != "" {
		return apiImageURL
	}
	url, _ := s.GetFoodImage(foodName)
	return url
}

// GetFoodImage returns the custom image URL for a food name (normalized).
func (s *Store) GetFoodImage(foodName string) (string, bool) {
	var url string
	err := s.db.QueryRow(
		`SELECT image_url FROM food_images WHERE food_name = ?`, normalize(foodName),
	).Scan(&url)
	if err != nil {
		return "", false
	}
	return url, true
}

// UpsertFoodImage sets a custom image URL for a food name.
func (s *Store) UpsertFoodImage(foodName, imageURL string) error {
	_, err := s.db.Exec(
		`INSERT INTO food_images(food_name, image_url, updated_at)
		 VALUES(?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(food_name) DO UPDATE SET image_url=excluded.image_url, updated_at=CURRENT_TIMESTAMP`,
		normalize(foodName), imageURL,
	)
	return err
}

// DeleteFoodImage removes a custom image entry.
func (s *Store) DeleteFoodImage(foodName string) error {
	_, err := s.db.Exec(`DELETE FROM food_images WHERE food_name = ?`, normalize(foodName))
	return err
}

// ListFoodImages returns all custom food image entries.
func (s *Store) ListFoodImages() ([]FoodImage, error) {
	rows, err := s.db.Query(`SELECT food_name, image_url, updated_at FROM food_images ORDER BY food_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FoodImage
	for rows.Next() {
		var fi FoodImage
		if err := rows.Scan(&fi.FoodName, &fi.ImageURL, &fi.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, fi)
	}
	return out, rows.Err()
}

// ── Favorites ────────────────────────────────────────────────────────────────

// ListFavorites returns all favorites, optionally filtered by school.
func (s *Store) ListFavorites(schoolSlug string) ([]Favorite, error) {
	q := `SELECT id, food_name, school_slug, created_at FROM favorites`
	args := []any{}
	if schoolSlug != "" {
		q += ` WHERE school_slug = '' OR school_slug = ?`
		args = append(args, schoolSlug)
	}
	q += ` ORDER BY food_name`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Favorite
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.FoodName, &f.SchoolSlug, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// IsFavorite reports whether the food is favorited for the given school (or globally).
func (s *Store) IsFavorite(foodName, schoolSlug string) bool {
	var n int
	s.db.QueryRow(
		`SELECT COUNT(*) FROM favorites WHERE food_name = ? AND (school_slug = '' OR school_slug = ?)`,
		normalize(foodName), schoolSlug,
	).Scan(&n)
	return n > 0
}

// AddFavorite stars a food item. schoolSlug="" means "any school".
func (s *Store) AddFavorite(foodName, schoolSlug string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO favorites(food_name, school_slug) VALUES(?, ?)`,
		normalize(foodName), schoolSlug,
	)
	return err
}

// RemoveFavorite removes a favorite by ID.
func (s *Store) RemoveFavorite(id int64) error {
	_, err := s.db.Exec(`DELETE FROM favorites WHERE id = ?`, id)
	return err
}

// ── Summary Exclusions ────────────────────────────────────────────────────────

// GetExclusions returns exclusion patterns for a school (including global ones).
func (s *Store) GetExclusions(schoolSlug string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT pattern FROM summary_exclusions WHERE school_slug = '' OR school_slug = ?`,
		schoolSlug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddExclusion adds a new summary exclusion pattern.
func (s *Store) AddExclusion(schoolSlug, pattern string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO summary_exclusions(school_slug, pattern) VALUES(?, ?)`,
		schoolSlug, strings.ToLower(strings.TrimSpace(pattern)),
	)
	return err
}

// DeleteExclusion removes a summary exclusion by ID.
func (s *Store) DeleteExclusion(id int64) error {
	_, err := s.db.Exec(`DELETE FROM summary_exclusions WHERE id = ?`, id)
	return err
}

// ListExclusions returns all exclusion rules.
func (s *Store) ListExclusions() ([]Exclusion, error) {
	rows, err := s.db.Query(`SELECT id, school_slug, pattern FROM summary_exclusions ORDER BY school_slug, pattern`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exclusion
	for rows.Next() {
		var e Exclusion
		if err := rows.Scan(&e.ID, &e.SchoolSlug, &e.Pattern); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── helpers ───────────────────────────────────────────────────────────────────

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
