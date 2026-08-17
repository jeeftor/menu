// Package ui provides terminal and web rendering for lunch menus.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"menu/internal/menu"
)

// Palette defines the ANSI colors used for each menu section in terminal output.
var Palette = map[string]lipgloss.AdaptiveColor{
	"Option 1":   {Light: "#1D4ED8", Dark: "#60A5FA"},
	"Option 2":   {Light: "#065F46", Dark: "#34D399"},
	"Option 3":   {Light: "#92400E", Dark: "#FBBF24"},
	"Option 4":   {Light: "#5B21B6", Dark: "#A78BFA"},
	"Vegetable":  {Light: "#166534", Dark: "#86EFAC"},
	"Fruit":      {Light: "#9A3412", Dark: "#FCA5A5"},
	"Milk":       {Light: "#1E40AF", Dark: "#93C5FD"},
	"Condiments": {Light: "#374151", Dark: "#D1D5DB"},
}

var sectionEmoji = map[string]string{
	"Option 1": "①", "Option 2": "②", "Option 3": "③", "Option 4": "④",
	"Vegetable": "🥦", "Fruit": "🍎", "Milk": "🥛", "Condiments": "🧂",
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	subStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	divStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#334155"))
	nameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0"))
	calStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Faint(true)
)

const termWidth = 62

// RenderDay prints a day's lunch menu to stdout using colorful terminal styles.
func RenderDay(day menu.DayMenu, schoolName string) {
	dateStr := day.Date.Format("Monday, January 2, 2006")

	header := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("🍽️  "+dateStr),
		subStyle.Render(schoolName+" · Lunch"),
	)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Padding(0, 2).
		MarginBottom(1).
		Render(header)

	fmt.Println(box)
	fmt.Println()

	for _, sec := range day.Sections {
		if len(sec.Foods) == 0 {
			continue
		}
		color := Palette[sec.Name]
		emoji := sectionEmoji[sec.Name]
		if emoji == "" {
			emoji = "•"
		}

		if strings.HasPrefix(sec.Name, "Option") {
			renderOptionSection(sec, color, emoji)
		} else {
			renderSideSection(sec, color, emoji)
		}
	}
}

func renderOptionSection(sec menu.Section, color lipgloss.AdaptiveColor, emoji string) {
	hdrStyle := lipgloss.NewStyle().Bold(true).Foreground(color)
	label := hdrStyle.Render(emoji + "  " + strings.ToUpper(sec.Name))

	ruleLen := termWidth - lipgloss.Width(label) - 1
	if ruleLen < 1 {
		ruleLen = 1
	}
	rule := divStyle.Render(strings.Repeat("─", ruleLen))

	fmt.Println(" " + label + " " + rule)

	for _, food := range sec.Foods {
		cal := ""
		if food.Calories > 0 {
			cal = calStyle.Render(fmt.Sprintf("%d cal", food.Calories))
		}
		namePart := "   " + nameStyle.Render(food.Name)
		if cal != "" {
			pad := termWidth - lipgloss.Width(namePart) - lipgloss.Width(cal)
			if pad < 1 {
				pad = 1
			}
			namePart += strings.Repeat(" ", pad) + cal
		}
		fmt.Println(namePart)
	}
	fmt.Println()
}

func renderSideSection(sec menu.Section, color lipgloss.AdaptiveColor, emoji string) {
	label := lipgloss.NewStyle().Bold(true).Foreground(color).Width(13).
		Render(emoji + " " + sec.Name)

	names := make([]string, len(sec.Foods))
	for i, f := range sec.Foods {
		names[i] = f.Name
	}
	items := subStyle.Render(strings.Join(names, "  ·  "))

	fmt.Println(" " + label + " " + items)
}

// RenderWeek prints Mon–Fri menu entries with separators.
func RenderWeek(days []menu.DayMenu, schoolName string) {
	sep := divStyle.Render(strings.Repeat("━", termWidth))
	for i, day := range days {
		if i > 0 {
			fmt.Println()
			fmt.Println(sep)
			fmt.Println()
		}
		RenderDay(day, schoolName)
	}
}

// RenderNoSchool prints a message for days without menu data.
func RenderNoSchool() {
	msg := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#64748B")).
		Italic(true).
		Render("No lunch menu found — possibly a weekend, holiday, or non-school day.")
	fmt.Println(msg)
}
