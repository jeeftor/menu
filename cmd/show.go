package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"menu/internal/menu"
	"menu/internal/nutrislice"
	"menu/internal/ui"
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the lunch menu in your terminal",
	Long: `Print the lunch menu for a given day (or week) with colorful terminal output.

Date formats:
  today, tomorrow, monday … friday
  YYYY-MM-DD`,
	RunE: runShow,
}

func init() {
	showCmd.Flags().String("date", "today", "date to show (today/tomorrow/YYYY-MM-DD/weekday)")
	showCmd.Flags().String("school", "", "school name or slug (default: Woodmen Roberts)")
	showCmd.Flags().Bool("week", false, "show the entire week instead of one day")
	showCmd.Flags().BoolP("json", "j", false, "output as JSON instead of terminal display")
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, _ []string) error {
	dateStr, _ := cmd.Flags().GetString("date")
	schoolQuery, _ := cmd.Flags().GetString("school")
	showWeek, _ := cmd.Flags().GetBool("week")
	asJSON, _ := cmd.Flags().GetBool("json")
	cacheDir := viper.GetString("cache_dir")

	school := nutrislice.FindSchool(schoolQuery)
	if school == nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
		fmt.Fprintln(os.Stderr, errStyle.Render("✗ Unknown school: "+schoolQuery))
		fmt.Fprintln(os.Stderr, "  Available schools:")
		for _, s := range nutrislice.DefaultSchools {
			fmt.Fprintln(os.Stderr, "    • "+s.Name+" ("+s.Slug+")")
		}
		return fmt.Errorf("school not found")
	}

	d, err := parseDate(dateStr)
	if err != nil {
		return fmt.Errorf("invalid date %q: %w", dateStr, err)
	}

	client := nutrislice.NewClient(cacheDir)
	week, err := client.FetchWeek(*school, d)
	if err != nil {
		return fmt.Errorf("fetching menu: %w", err)
	}

	dayMenus, err := menu.ParseWeek(*week)
	if err != nil {
		return fmt.Errorf("parsing menu: %w", err)
	}

	if showWeek {
		var days []menu.DayMenu
		for _, dm := range dayMenus {
			days = append(days, dm)
		}
		sort.Slice(days, func(i, j int) bool { return days[i].Date.Before(days[j].Date) })

		if asJSON {
			return printJSON(map[string]any{
				"school": school.Name,
				"week_of": d.Format("2006-01-02"),
				"days":    menuDaysToJSON(days),
			})
		}
		if len(days) == 0 {
			ui.RenderNoSchool()
		} else {
			ui.RenderWeek(days, school.Name)
		}
		return nil
	}

	key := d.Format("2006-01-02")
	day, ok := dayMenus[key]
	if !ok {
		if asJSON {
			return printJSON(map[string]any{
				"date": key, "school": school.Name,
				"note": "no menu found — holiday or non-school day",
			})
		}
		ui.RenderNoSchool()
		return nil
	}

	if asJSON {
		return printJSON(map[string]any{
			"date":     key,
			"school":   school.Name,
			"sections": sectionsToJSON(day.Sections),
		})
	}
	ui.RenderDay(day, school.Name)
	return nil
}

// ── JSON helpers ─────────────────────────────────────────────────────────────

type jsonFood struct {
	Name     string   `json:"name"`
	ImageURL string   `json:"image_url,omitempty"`
	Calories int      `json:"calories,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}
type jsonSection struct {
	Name  string     `json:"name"`
	Foods []jsonFood `json:"foods"`
}
type jsonDay struct {
	Date     string        `json:"date"`
	Sections []jsonSection `json:"sections"`
}

func sectionsToJSON(sections []menu.Section) []jsonSection {
	out := make([]jsonSection, 0, len(sections))
	for _, s := range sections {
		js := jsonSection{Name: s.Name, Foods: make([]jsonFood, 0, len(s.Foods))}
		for _, f := range s.Foods {
			js.Foods = append(js.Foods, jsonFood{
				Name: f.Name, ImageURL: f.ImageURL,
				Calories: f.Calories, Tags: f.Tags,
			})
		}
		out = append(out, js)
	}
	return out
}

func menuDaysToJSON(days []menu.DayMenu) []jsonDay {
	out := make([]jsonDay, 0, len(days))
	for _, d := range days {
		out = append(out, jsonDay{
			Date:     d.Date.Format("2006-01-02"),
			Sections: sectionsToJSON(d.Sections),
		})
	}
	return out
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// parseDate converts a human-friendly string to a time.Time.
func parseDate(s string) (time.Time, error) {
	// Use local midnight, not UTC midnight (Truncate snaps to UTC zero which is wrong in US timezones)
	now := time.Now()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch strings.ToLower(strings.TrimSpace(s)) {
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

// nextWeekday returns today if it matches the weekday, otherwise the next occurrence.
func nextWeekday(from time.Time, wd time.Weekday) time.Time {
	days := int(wd - from.Weekday())
	if days < 0 {
		days += 7
	}
	return from.AddDate(0, 0, days)
}
