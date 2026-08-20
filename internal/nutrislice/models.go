// Package nutrislice provides types and a client for the Nutrislice lunch menu API.
package nutrislice

// WeekResponse is the top-level response from the Nutrislice weekly menu API.
type WeekResponse struct {
	StartDate  string   `json:"start_date"`
	MenuTypeID int      `json:"menu_type_id"`
	Days       []RawDay `json:"days"`
	ID         int      `json:"id"`
}

// RawDay holds one calendar day's flat list of menu items.
type RawDay struct {
	Date      string     `json:"date"`
	MenuItems []MenuItem `json:"menu_items"`
}

// MenuItem is a single entry in a day's menu — either a section header or a food.
type MenuItem struct {
	ID              int    `json:"id"`
	Position        int    `json:"position"`
	IsSectionTitle  bool   `json:"is_section_title"`
	IsStationHeader bool   `json:"is_station_header"`
	Text            string `json:"text"`
	BlankLine       bool   `json:"blank_line"`
	Food            *Food  `json:"food"`
}

// Food holds detailed information about a single food item.
type Food struct {
	ID               int            `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	ImageURL         string         `json:"image_url"`
	HoverpicURL      string         `json:"hoverpic_url"`
	Ingredients      string         `json:"ingredients"`
	RoundedNutrition *NutritionInfo `json:"rounded_nutrition_info"`
	Tags             []FoodTag      `json:"tags"`
}

// NutritionInfo contains per-serving nutritional values.
type NutritionInfo struct {
	Calories float64 `json:"calories"`
	GFat     float64 `json:"g_fat"`
	GProtein float64 `json:"g_protein"`
	GCarbs   float64 `json:"g_carbs"`
	MGSodium float64 `json:"mg_sodium"`
	GFiber   float64 `json:"g_fiber"`
	GSugar   float64 `json:"g_sugar"`
}

// FoodTag is a dietary label (e.g. "Vegan", "Vegetarian").
type FoodTag struct {
	Name string `json:"name"`
}

// School represents a school with its Nutrislice API credentials.
type School struct {
	Name      string // full display name
	ShortName string // abbreviated label used in UI (e.g. "WRES", "EMS")
	Slug      string
	District  string
}

// DefaultSchools contains the configured ASD20 schools.
var DefaultSchools = []School{
	{
		Name:      "Woodman Roberts Elementary",
		ShortName: "WRES",
		Slug:      "woodmen-roberts-elementary-school",
		District:  "asd20",
	},
	{
		Name:      "Eagleview Middle School",
		ShortName: "EMS",
		Slug:      "eagleview-middle-school",
		District:  "asd20",
	},
}

// FindSchool returns the first school matching the given slug, name, or substring.
// Returns nil if no match is found.
func FindSchool(query string) *School {
	if query == "" {
		return &DefaultSchools[0]
	}
	q := toLower(query)
	for i, s := range DefaultSchools {
		if toLower(s.Slug) == q || toLower(s.Name) == q {
			return &DefaultSchools[i]
		}
	}
	for i, s := range DefaultSchools {
		if contains(toLower(s.Slug), q) || contains(toLower(s.Name), q) {
			return &DefaultSchools[i]
		}
	}
	return nil
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
