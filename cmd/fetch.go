package cmd

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"menu/internal/menu"
	"menu/internal/nutrislice"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Pre-fetch and cache a month of menus",
	Long: `Download and cache menu data for a given month so 'show' and 'serve' work offline.

Examples:
  food fetch                  # current month, default school
  food fetch --month 2026-09  # September 2026
  food fetch --school eagleview --month 2026-08`,
	RunE: runFetch,
}

func init() {
	fetchCmd.Flags().String("month", "", "month to fetch in YYYY-MM format (default: current month)")
	fetchCmd.Flags().String("school", "", "school name or slug (default: Woodmen Roberts)")
	rootCmd.AddCommand(fetchCmd)
}

func runFetch(cmd *cobra.Command, _ []string) error {
	monthStr, _ := cmd.Flags().GetString("month")
	schoolQuery, _ := cmd.Flags().GetString("school")
	cacheDir := viper.GetString("cache_dir")

	school := nutrislice.FindSchool(schoolQuery)
	if school == nil {
		school = &nutrislice.DefaultSchools[0]
	}

	var year, month int
	if monthStr == "" {
		now := time.Now()
		year, month = now.Year(), int(now.Month())
	} else {
		t, err := time.Parse("2006-01", monthStr)
		if err != nil {
			return fmt.Errorf("invalid month %q — use YYYY-MM format", monthStr)
		}
		year, month = t.Year(), int(t.Month())
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))

	fmt.Println(titleStyle.Render(fmt.Sprintf("🗓  Fetching %s %d — %s", time.Month(month), year, school.Name)))
	fmt.Println()

	client := nutrislice.NewClient(cacheDir)
	weeks, err := client.FetchMonth(*school, year, month)
	if err != nil {
		return fmt.Errorf("fetching month: %w", err)
	}

	totalDays := 0
	for _, week := range weeks {
		parsed, err := menu.ParseWeek(*week)
		if err != nil {
			fmt.Println(errStyle.Render("  ✗ parse error: " + err.Error()))
			continue
		}
		for dateStr, day := range parsed {
			totalDays++
			opts := day.OptionSections()
			optNames := make([]string, len(opts))
			for i, o := range opts {
				if len(o.Foods) > 0 {
					optNames[i] = o.Foods[0].Name
				}
			}
			line := fmt.Sprintf("  %s  %s", dateStr, day.Date.Format("Mon"))
			for i, n := range optNames {
				if n != "" {
					line += fmt.Sprintf("  [%d] %s", i+1, n)
				}
			}
			fmt.Println(okStyle.Render("  ✓ ") + dimStyle.Render(line))
		}
	}

	fmt.Println()
	fmt.Println(okStyle.Render(fmt.Sprintf("  Cached %d school days → %s", totalDays, cacheDir)))
	return nil
}
