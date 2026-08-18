// Package menu provides parsed, display-ready lunch menu types.
package menu

import (
	"fmt"
	"sort"
	"time"

	"menu/internal/nutrislice"
)

// DayMenu is a cleaned, display-ready version of a single day's lunch menu.
type DayMenu struct {
	Date     time.Time
	Sections []Section
}

// Section is a named group of food items (e.g. "Option 1", "Vegetable").
type Section struct {
	Name  string
	Foods []Item
}

// Item is a single food with display-relevant fields extracted.
type Item struct {
	Name     string
	ImageURL string
	Calories int
	Tags     []string
}

// OptionSections filters sections whose name starts with "Option".
func (d DayMenu) OptionSections() []Section {
	var out []Section
	for _, s := range d.Sections {
		if len(s.Name) >= 6 && s.Name[:6] == "Option" {
			out = append(out, s)
		}
	}
	return out
}

// HasMenu returns true if any section contains at least one food item.
// Works for both "Option N" style (Woodmen Roberts) and named-section
// style (Eagleview) menus.
func (d DayMenu) HasMenu() bool {
	for _, s := range d.Sections {
		if len(s.Foods) > 0 {
			return true
		}
	}
	return false
}

// SectionByName returns the first section with the given name, or nil.
func (d DayMenu) SectionByName(name string) *Section {
	for i, s := range d.Sections {
		if s.Name == name {
			return &d.Sections[i]
		}
	}
	return nil
}

// ParseDay converts raw Nutrislice day data into a clean DayMenu.
func ParseDay(raw nutrislice.RawDay) (DayMenu, error) {
	date, err := time.Parse("2006-01-02", raw.Date)
	if err != nil {
		return DayMenu{}, fmt.Errorf("parsing date %q: %w", raw.Date, err)
	}

	items := make([]nutrislice.MenuItem, len(raw.MenuItems))
	copy(items, raw.MenuItems)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Position < items[j].Position
	})

	var sections []Section
	var current *Section

	for _, mi := range items {
		if mi.IsSectionTitle || mi.IsStationHeader {
			sections = append(sections, Section{Name: mi.Text})
			current = &sections[len(sections)-1]
			continue
		}
		if mi.Food == nil || current == nil {
			continue
		}

		f := mi.Food
		cal := 0
		if f.RoundedNutrition != nil && f.RoundedNutrition.Calories > 0 {
			cal = int(f.RoundedNutrition.Calories)
		}
		tags := make([]string, 0, len(f.Tags))
		for _, t := range f.Tags {
			if t.Name != "" {
				tags = append(tags, t.Name)
			}
		}

		current.Foods = append(current.Foods, Item{
			Name:     f.Name,
			ImageURL: f.ImageURL,
			Calories: cal,
			Tags:     tags,
		})
	}

	return DayMenu{Date: date, Sections: sections}, nil
}

// ParseWeek converts a WeekResponse into a map of date string → DayMenu.
// Only weekdays (Mon-Fri) with non-empty menus are included.
func ParseWeek(week nutrislice.WeekResponse) (map[string]DayMenu, error) {
	result := make(map[string]DayMenu)
	for _, raw := range week.Days {
		if len(raw.MenuItems) == 0 {
			continue
		}
		day, err := ParseDay(raw)
		if err != nil {
			return nil, err
		}
		if day.Date.Weekday() == time.Saturday || day.Date.Weekday() == time.Sunday {
			continue
		}
		result[raw.Date] = day
	}
	return result, nil
}
