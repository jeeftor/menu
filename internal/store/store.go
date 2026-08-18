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

// SectionInclude specifies which option sections are included for a given school+meal.
// When any include rules exist for a school+meal combo, only those sections count.
// Non-option sections (Fruit, Vegetable, Milk, etc.) are always shown in Option-N mode.
type SectionInclude struct {
	ID          int64
	SchoolSlug  string // empty = all schools
	MealType    string // empty = all meals
	SectionName string // e.g. "Option 1" or "Entree"
	Position    int    // display order within the school+meal group (0-based)
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

		CREATE TABLE IF NOT EXISTS section_includes (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			school_slug  TEXT NOT NULL DEFAULT '',
			meal_type    TEXT NOT NULL DEFAULT '',
			section_name TEXT NOT NULL,
			position     INTEGER NOT NULL DEFAULT 0,
			UNIQUE(school_slug, meal_type, section_name)
		);

		-- Seed default exclusions for Woodmen Roberts (sun butter is a standing alternative)
		INSERT OR IGNORE INTO summary_exclusions (school_slug, pattern)
		VALUES ('woodmen-roberts-elementary-school', 'sun butter');
	`)
	if err != nil {
		return err
	}
	// Add position column to existing section_includes tables (idempotent).
	_, _ = s.db.Exec(`ALTER TABLE section_includes ADD COLUMN position INTEGER NOT NULL DEFAULT 0`)
	return nil
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

// ── Section Includes ─────────────────────────────────────────────────────────

// GetSectionIncludes returns section names for a given school+meal, ordered by position.
// An empty result means "include all sections" (no filter).
func (s *Store) GetSectionIncludes(schoolSlug, mealType string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT section_name FROM section_includes WHERE school_slug = ? AND meal_type = ? ORDER BY position, id`,
		schoolSlug, mealType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddSectionInclude adds a section include rule, assigning the next available position.
func (s *Store) AddSectionInclude(schoolSlug, mealType, sectionName string) error {
	meal := strings.ToLower(strings.TrimSpace(mealType))
	name := strings.TrimSpace(sectionName)
	var maxPos int
	s.db.QueryRow(
		`SELECT COALESCE(MAX(position)+1, 0) FROM section_includes WHERE school_slug = ? AND meal_type = ?`,
		schoolSlug, meal,
	).Scan(&maxPos)
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO section_includes(school_slug, meal_type, section_name, position) VALUES(?, ?, ?, ?)`,
		schoolSlug, meal, name, maxPos,
	)
	return err
}

// ReorderSectionIncludes sets the position of each section in orderedNames (0-based index).
// Sections not in the list are unaffected.
func (s *Store) ReorderSectionIncludes(schoolSlug, mealType string, orderedNames []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(
		`UPDATE section_includes SET position = ? WHERE school_slug = ? AND meal_type = ? AND section_name = ?`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, name := range orderedNames {
		if _, err := stmt.Exec(i, schoolSlug, mealType, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteSectionInclude removes a section include rule by ID.
func (s *Store) DeleteSectionInclude(id int64) error {
	_, err := s.db.Exec(`DELETE FROM section_includes WHERE id = ?`, id)
	return err
}

// ListSectionIncludes returns all section include rules ordered by school, meal, position.
func (s *Store) ListSectionIncludes() ([]SectionInclude, error) {
	rows, err := s.db.Query(
		`SELECT id, school_slug, meal_type, section_name, position FROM section_includes ORDER BY school_slug, meal_type, position, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SectionInclude
	for rows.Next() {
		var si SectionInclude
		if err := rows.Scan(&si.ID, &si.SchoolSlug, &si.MealType, &si.SectionName, &si.Position); err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}

// ── helpers ───────────────────────────────────────────────────────────────────

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
