// Package mcpserver provides the MCP tool server for school lunch queries.
package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"menu/internal/menu"
	"menu/internal/nutrislice"
)

// ── input/output types ────────────────────────────────────────────────────────

type getLunchInput struct {
	Date     string `json:"date,omitempty"`
	School   string `json:"school,omitempty"`
	MealType string `json:"meal_type,omitempty"`
}

type getLunchSummaryInput struct {
	// Date accepts: "today", "tomorrow", "next" (next school day with menu),
	// a weekday name, or YYYY-MM-DD. Defaults to "today".
	Date     string `json:"date,omitempty"`
	School   string `json:"school,omitempty"`
	MealType string `json:"meal_type,omitempty"`
}

type getLunchWeekInput struct {
	Date     string `json:"date,omitempty"`
	School   string `json:"school,omitempty"`
	MealType string `json:"meal_type,omitempty"`
}

type foodJSON struct {
	Name     string   `json:"name"`
	ImageURL string   `json:"image_url,omitempty"`
	Calories int      `json:"calories,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type sectionJSON struct {
	Name  string     `json:"name"`
	Foods []foodJSON `json:"foods"`
}

type dayJSON struct {
	Date     string        `json:"date"`
	School   string        `json:"school"`
	Sections []sectionJSON `json:"sections"`
}

type weekJSON struct {
	School string    `json:"school"`
	WeekOf string    `json:"week_of"`
	Days   []dayJSON `json:"days"`
}

type schoolJSON struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	District string `json:"district"`
}

type schoolsJSON struct {
	Schools []schoolJSON `json:"schools"`
}

// ── New ───────────────────────────────────────────────────────────────────────

// New creates and returns a configured MCP server with all food tools registered.
func New(client *nutrislice.Client) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "menu", Version: "1.0.0"}, nil)

	mcp.AddTool(srv,
		&mcp.Tool{
			Name: "get_lunch",
			Description: `Get the school lunch menu for a specific date.
date accepts: "today", "tomorrow", "yesterday", a weekday name (e.g. "monday"), or YYYY-MM-DD.
school accepts a school name or slug (e.g. "woodmen-roberts" or "eagleview"). Defaults to Woodmen Roberts Elementary.
meal_type accepts "lunch" or "breakfast". Defaults to "lunch".`,
		},
		func(_ context.Context, _ *mcp.CallToolRequest, in getLunchInput) (*mcp.CallToolResult, dayJSON, error) {
			school := resolveSchool(in.School)
			mealType := in.MealType
			if mealType == "" {
				mealType = "lunch"
			}
			d, err := parseDate(in.Date)
			if err != nil {
				return nil, dayJSON{}, fmt.Errorf("invalid date %q: %w", in.Date, err)
			}
			week, err := client.FetchWeek(*school, d, mealType)
			if err != nil {
				return nil, dayJSON{}, fmt.Errorf("fetching menu: %w", err)
			}
			dayMenus, err := menu.ParseWeek(*week)
			if err != nil {
				return nil, dayJSON{}, fmt.Errorf("parsing menu: %w", err)
			}
			key := d.Format("2006-01-02")
			dm, ok := dayMenus[key]
			if !ok {
				return nil, dayJSON{Date: key, School: school.Name}, nil
			}
			return nil, dayJSON{
				Date:     key,
				School:   school.Name,
				Sections: toSectionsJSON(dm.Sections),
			}, nil
		},
	)

	mcp.AddTool(srv,
		&mcp.Tool{
			Name: "get_lunch_week",
			Description: `Get the school lunch menu for an entire week (Mon–Fri).
date can be any day within the target week; accepts the same formats as get_lunch.
school accepts a school name or slug. Defaults to Woodmen Roberts Elementary.
meal_type accepts "lunch" or "breakfast". Defaults to "lunch".`,
		},
		func(_ context.Context, _ *mcp.CallToolRequest, in getLunchWeekInput) (*mcp.CallToolResult, weekJSON, error) {
			school := resolveSchool(in.School)
			mealType := in.MealType
			if mealType == "" {
				mealType = "lunch"
			}
			d, err := parseDate(in.Date)
			if err != nil {
				return nil, weekJSON{}, fmt.Errorf("invalid date %q: %w", in.Date, err)
			}
			week, err := client.FetchWeek(*school, d, mealType)
			if err != nil {
				return nil, weekJSON{}, fmt.Errorf("fetching menu: %w", err)
			}
			dayMenus, err := menu.ParseWeek(*week)
			if err != nil {
				return nil, weekJSON{}, fmt.Errorf("parsing menu: %w", err)
			}
			days := make([]dayJSON, 0, len(dayMenus))
			for dateStr, dm := range dayMenus {
				days = append(days, dayJSON{
					Date:     dateStr,
					School:   school.Name,
					Sections: toSectionsJSON(dm.Sections),
				})
			}
			sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })
			return nil, weekJSON{
				School: school.Name,
				WeekOf: d.Format("2006-01-02"),
				Days:   days,
			}, nil
		},
	)

	mcp.AddTool(srv,
		&mcp.Tool{
			Name: "get_lunch_summary",
			Description: `Get a concise summary of today's (or any day's) main lunch options — perfect for voice assistants or quick status checks.
Returns only the real rotating entrées (sun butter standing alternatives are excluded).
date accepts: "today", "tomorrow", "next" (next school day with menu data), a weekday name, or YYYY-MM-DD.
The "text" field is ready-to-speak: e.g. "Buffalo Cheese Pizza Sticks, Cheeseburger, or Turkey & Cheese Combo Sub".`,
		},
		func(_ context.Context, _ *mcp.CallToolRequest, in getLunchSummaryInput) (*mcp.CallToolResult, menu.Summary, error) {
			school := resolveSchool(in.School)
			mealType := in.MealType
			if mealType == "" {
				mealType = "lunch"
			}
			dateParam := strings.ToLower(strings.TrimSpace(in.Date))
			if dateParam == "" {
				dateParam = "today"
			}

			var dm menu.DayMenu
			var err error
			if dateParam == "next" {
				dm, err = findNextMenuDay(client, *school, mealType)
			} else {
				d, parseErr := parseDate(dateParam)
				if parseErr != nil {
					return nil, menu.Summary{}, parseErr
				}
				dm, err = fetchDay(client, *school, d, mealType)
			}
			if err != nil {
				return nil, menu.Summary{}, err
			}
			return nil, menu.BuildSummary(dm, school.Name, nil), nil
		},
	)

	mcp.AddTool(srv,
		&mcp.Tool{
			Name:        "list_schools",
			Description: "List all available schools with their names and slugs.",
		},
		func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, schoolsJSON, error) {
			schools := make([]schoolJSON, len(nutrislice.DefaultSchools))
			for i, s := range nutrislice.DefaultSchools {
				schools[i] = schoolJSON{Name: s.Name, Slug: s.Slug, District: s.District}
			}
			return nil, schoolsJSON{Schools: schools}, nil
		},
	)

	return srv
}

// ── helpers ───────────────────────────────────────────────────────────────────

func resolveSchool(query string) *nutrislice.School {
	if s := nutrislice.FindSchool(query); s != nil {
		return s
	}
	return &nutrislice.DefaultSchools[0]
}

func toSectionsJSON(sections []menu.Section) []sectionJSON {
	out := make([]sectionJSON, 0, len(sections))
	for _, s := range sections {
		foods := make([]foodJSON, 0, len(s.Foods))
		for _, f := range s.Foods {
			foods = append(foods, foodJSON{
				Name:     f.Name,
				ImageURL: f.ImageURL,
				Calories: f.Calories,
				Tags:     f.Tags,
			})
		}
		out = append(out, sectionJSON{Name: s.Name, Foods: foods})
	}
	return out
}

// parseDate converts human-friendly date strings to time.Time (local midnight).
func parseDate(s string) (time.Time, error) {
	now := time.Now()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch s {
	case "", "today":
		return today, nil
	case "tomorrow":
		return today.AddDate(0, 0, 1), nil
	case "yesterday":
		return today.AddDate(0, 0, -1), nil
	case "monday":
		return nextWeekday(today, time.Monday), nil
	case "tuesday":
		return nextWeekday(today, time.Tuesday), nil
	case "wednesday":
		return nextWeekday(today, time.Wednesday), nil
	case "thursday":
		return nextWeekday(today, time.Thursday), nil
	case "friday":
		return nextWeekday(today, time.Friday), nil
	default:
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or a weekday name, got %q", s)
		}
		return t, nil
	}
}

func nextWeekday(from time.Time, wd time.Weekday) time.Time {
	days := int(wd - from.Weekday())
	if days < 0 {
		days += 7
	}
	return from.AddDate(0, 0, days)
}

// fetchDay retrieves and parses a single day's menu.
func fetchDay(client *nutrislice.Client, school nutrislice.School, d time.Time, mealType string) (menu.DayMenu, error) {
	week, err := client.FetchWeek(school, d, mealType)
	if err != nil {
		return menu.DayMenu{}, err
	}
	dayMenus, err := menu.ParseWeek(*week)
	if err != nil {
		return menu.DayMenu{}, err
	}
	key := d.Format("2006-01-02")
	dm, ok := dayMenus[key]
	if !ok {
		return menu.DayMenu{}, fmt.Errorf("no menu for %s", key)
	}
	return dm, nil
}

// findNextMenuDay searches forward from tomorrow for the next school day with
// at least one real entrée option (up to 14 calendar days ahead).
func findNextMenuDay(client *nutrislice.Client, school nutrislice.School, mealType string) (menu.DayMenu, error) {
	now := time.Now()
	y, m, d := now.Date()
	start := time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
	for i := 0; i < 14; i++ {
		candidate := start.AddDate(0, 0, i)
		if wd := candidate.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		dm, err := fetchDay(client, school, candidate, mealType)
		if err != nil {
			continue
		}
		if len(dm.OptionSections()) > 0 {
			return dm, nil
		}
	}
	return menu.DayMenu{}, fmt.Errorf("no upcoming menu found in the next 14 days")
}
