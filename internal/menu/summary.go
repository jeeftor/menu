package menu

import (
	"strings"
)

// Summary is a concise view of a day's lunch — just the real entrée options,
// formatted for Home Assistant sensors and voice assistants like Alexa.
type Summary struct {
	Date    string   `json:"date"`
	School  string   `json:"school"`
	Options []string `json:"options"`
	// Text is a natural-language string ready for HA sensor state or Alexa speech.
	// Example: "Buffalo Cheese Pizza Sticks, Cheeseburger, or Turkey & Cheese Combo Sub"
	Text string `json:"text"`
}

// BuildSummary extracts the main entrée choices from a DayMenu, filtering out
// standing alternatives (e.g. Sun Butter & Jelly Sandwich) that appear every day.
func BuildSummary(day DayMenu, schoolName string) Summary {
	opts := day.OptionSections()
	names := make([]string, 0, len(opts))
	for _, sec := range opts {
		if len(sec.Foods) == 0 {
			continue
		}
		first := sec.Foods[0].Name
		if isSunButter(first) {
			continue
		}
		names = append(names, first)
	}

	return Summary{
		Date:    day.Date.Format("2006-01-02"),
		School:  schoolName,
		Options: names,
		Text:    joinOptions(names),
	}
}

// isSunButter reports whether a food name is a sun butter item — a standing
// daily alternative at Woodmen Roberts that isn't a real rotating choice.
func isSunButter(name string) bool {
	return strings.Contains(strings.ToLower(name), "sun butter")
}

// joinOptions formats a slice of option names into natural English.
//   []string{"A"}           → "A"
//   []string{"A", "B"}      → "A or B"
//   []string{"A", "B", "C"} → "A, B, or C"
func joinOptions(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " or " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
	}
}
