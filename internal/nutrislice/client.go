package nutrislice

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Client fetches Nutrislice menu data with optional disk caching.
type Client struct {
	CacheDir   string
	HTTPClient *http.Client
}

// NewClient returns a Client with sensible defaults.
func NewClient(cacheDir string) *Client {
	return &Client{
		CacheDir:   cacheDir,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// FetchWeek returns the week of menu data containing the given date for a school.
// mealType is "lunch" or "breakfast"; defaults to "lunch" if empty.
// Results are cached on disk by school + meal type + date.
func (c *Client) FetchWeek(school School, d time.Time, mealType string) (*WeekResponse, error) {
	if mealType == "" {
		mealType = "lunch"
	}
	// Cache key: district_slug_mealtype_YYYY-MM-DD.json (use Monday of the week)
	monday := d.AddDate(0, 0, -int(d.Weekday()-time.Monday))
	if d.Weekday() == time.Sunday {
		monday = d.AddDate(0, 0, 1)
	}
	cacheKey := fmt.Sprintf("%s_%s_%s_%s.json", school.District, school.Slug, mealType, monday.Format("2006-01-02"))
	cachePath := filepath.Join(c.CacheDir, cacheKey)

	if data, err := os.ReadFile(cachePath); err == nil { // #nosec G304 -- cachePath is constructed internally from a fixed cache directory and a sanitized date hash
		var resp WeekResponse
		if err := json.Unmarshal(data, &resp); err == nil {
			slog.Debug("cache hit", "key", cacheKey)
			return &resp, nil
		}
	}

	url := fmt.Sprintf(
		"https://%s.api.nutrislice.com/menu/api/weeks/school/%s/menu-type/%s/%d/%02d/%02d/",
		school.District, school.Slug, mealType, d.Year(), d.Month(), d.Day(),
	)
	slog.Info("fetching menu", "url", url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var result WeekResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON response: %w", err)
	}

	// Cache to disk (best-effort)
	if err := os.MkdirAll(c.CacheDir, 0o750); err == nil {
		if err := os.WriteFile(cachePath, body, 0o600); err != nil {
			slog.Warn("could not cache response", "path", cachePath, "err", err)
		}
	}

	return &result, nil
}

// FetchMonth fetches all weeks covering the given month and returns a merged week map.
// mealType is "lunch" or "breakfast"; defaults to "lunch" if empty.
func (c *Client) FetchMonth(school School, year, month int, mealType string) (map[string]*WeekResponse, error) {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	last := first.AddDate(0, 1, -1)

	// Iterate directly over Mondays: from the Monday on/before the first day
	// through the Monday of the last day, so partial end-of-month weeks aren't missed.
	// (e.g. August 31 is a Monday — without this fix its week would be skipped.)
	startMonday := mondayOf(first)
	endMonday := mondayOf(last)

	result := make(map[string]*WeekResponse)
	for mon := startMonday; !mon.After(endMonday); mon = mon.AddDate(0, 0, 7) {
		key := mon.Format("2006-01-02")
		week, err := c.FetchWeek(school, mon, mealType)
		if err != nil {
			slog.Warn("skipping week", "monday", key, "err", err)
			continue
		}
		result[key] = week
	}

	return result, nil
}

// mondayOf returns the Monday on or before d.
func mondayOf(d time.Time) time.Time {
	offset := int(d.Weekday()) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	return d.AddDate(0, 0, -offset)
}
