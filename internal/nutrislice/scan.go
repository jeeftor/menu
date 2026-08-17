package nutrislice

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanMissingImages scans all cached menu JSON files and returns food names
// that appear in menus but have no API-provided image URL.
// customCovered is a set of normalized (lower-case) food names that already
// have a custom image override — those are excluded from the result.
func (c *Client) ScanMissingImages(customCovered map[string]bool) ([]string, error) {
	if c.CacheDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(c.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// name → true if any cached occurrence has an image
	seen := make(map[string]bool)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.CacheDir, e.Name()))
		if err != nil {
			continue
		}
		var week WeekResponse
		if err := json.Unmarshal(data, &week); err != nil {
			continue
		}
		for _, day := range week.Days {
			for _, mi := range day.MenuItems {
				if mi.IsSectionTitle || mi.IsStationHeader || mi.Food == nil {
					continue
				}
				name := strings.TrimSpace(mi.Food.Name)
				if name == "" {
					continue
				}
				hasImg := strings.TrimSpace(mi.Food.ImageURL) != ""
				if existing, ok := seen[name]; ok {
					if hasImg && !existing {
						seen[name] = true // upgrade: found an image on another day
					}
				} else {
					seen[name] = hasImg
				}
			}
		}
	}

	var missing []string
	for name, hasImg := range seen {
		if hasImg {
			continue
		}
		if customCovered[strings.ToLower(strings.TrimSpace(name))] {
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return missing, nil
}
