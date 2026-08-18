// Package server provides the HTTP server and calendar web UI.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"menu/internal/auth"
	"menu/internal/menu"
	"menu/internal/nutrislice"
	"menu/internal/store"
)

// optionPalette cycles text/bg colors for Option 1, 2, 3, … wrapping for any count.
var optionPalette = [][2]string{
	{"#1D4ED8", "#EFF6FF"},
	{"#065F46", "#ECFDF5"},
	{"#92400E", "#FFFBEB"},
	{"#5B21B6", "#F5F3FF"},
	{"#0E7490", "#ECFEFF"},
	{"#B91C1C", "#FEF2F2"},
}

var sectionSideStyle = map[string][2]string{
	"Vegetable":  {"#166534", "#F0FDF4"},
	"Fruit":      {"#9A3412", "#FFF7ED"},
	"Milk":       {"#1E40AF", "#EFF6FF"},
	"Condiments": {"#374151", "#F9FAFB"},
}

var sectionSideEmoji = map[string]string{
	"Vegetable": "&#x1F966;", "Fruit": "&#x1F34E;",
	"Milk": "&#x1F95B;", "Condiments": "&#x1F9C2;",
}

var circledNums = []string{
	"&#x2460;", "&#x2461;", "&#x2462;", "&#x2463;",
	"&#x2464;", "&#x2465;", "&#x2466;", "&#x2467;",
}

// optionColor returns text/bg colors and whether the section is a rotating Option.
func optionColor(name string) (text, bg string, isOption bool) {
	if strings.HasPrefix(name, "Option ") {
		if n, err := strconv.Atoi(strings.TrimPrefix(name, "Option ")); err == nil && n > 0 {
			c := optionPalette[(n-1)%len(optionPalette)]
			return c[0], c[1], true
		}
	}
	if s, ok := sectionSideStyle[name]; ok {
		return s[0], s[1], false
	}
	return "#64748B", "#F8FAFC", false
}

// sectionEmoji returns a display emoji string for a section name.
func sectionEmoji(name string) string {
	if em, ok := sectionSideEmoji[name]; ok {
		return em
	}
	if strings.HasPrefix(name, "Option ") {
		if n, err := strconv.Atoi(strings.TrimPrefix(name, "Option ")); err == nil && n >= 1 && n <= len(circledNums) {
			return circledNums[n-1]
		}
	}
	return "&#x1F374;"
}

// modalSectionOrder defines the display order in the day-detail modal.
var modalSectionOrder = []string{
	"Option 1", "Option 2", "Option 3", "Option 4", "Option 5", "Option 6",
	"Vegetable", "Fruit", "Condiments", "Milk",
}

// Server is the school lunch HTTP server.
type Server struct {
	client       *nutrislice.Client
	port         int
	version      string
	mux          *http.ServeMux
	mcpServer    *mcp.Server
	store        *store.Store
	alexaHandler http.Handler
	oidc         *auth.OIDCProvider
	sessions     *auth.SessionManager
}

// AlexaConfig holds settings for the /alexa endpoint. Nil disables it.
type AlexaConfig struct {
	ApplicationID  string
	VerifyRequests bool
	DefaultSchool  string
	DefaultMeal    string
}

// AuthConfig holds optional OIDC / session settings. Nil disables web login.
type AuthConfig struct {
	OIDC     auth.OIDCConfig
	Sessions *auth.SessionManager
}

// New creates a Server bound to the given port. st may be nil if no persistence is needed.
func New(client *nutrislice.Client, port int, mcpSrv *mcp.Server, st *store.Store, version string, alexaCfg *AlexaConfig, authCfg *AuthConfig) *Server {
	s := &Server{client: client, port: port, version: version, mux: http.NewServeMux(), mcpServer: mcpSrv, store: st}
	if authCfg != nil {
		s.oidc = mustInitOIDC(authCfg.OIDC, authCfg.Sessions)
		s.sessions = authCfg.Sessions
	}
	s.mux.HandleFunc("/", s.handleRoot)
	s.mux.HandleFunc("/calendar", s.handleCalendar)
	// REST API v1 — public read endpoints
	s.mux.HandleFunc("/api/v1/schools", s.handleAPISchools)
	s.mux.HandleFunc("/api/v1/lunch", s.handleAPILunch)
	s.mux.HandleFunc("/api/v1/lunch/summary", s.handleAPISummary)
	s.mux.HandleFunc("/api/v1/lunch/week", s.handleAPILunchWeek)
	s.mux.HandleFunc("/api/v1/lunch/month", s.handleAPILunchMonth)
	s.mux.HandleFunc("/api/v1/sections", s.handleAPISections)
	// Settings/config require LAN or OIDC login
	s.mux.HandleFunc("/settings", s.requireLANOrAuth(s.handleSettings))
	s.mux.HandleFunc("/api/v1/food-images", s.requireLANOrAuthForWrites(s.handleAPIFoodImages))
	s.mux.HandleFunc("/api/v1/favorites", s.requireLANOrAuthForWrites(s.handleAPIFavorites))
	s.mux.HandleFunc("/api/v1/exclusions", s.requireLANOrAuthForWrites(s.handleAPIExclusions))
	s.mux.HandleFunc("/api/v1/section-includes", s.requireLANOrAuthForWrites(s.handleAPISectionIncludes))
	s.mux.HandleFunc("/api/v1/section-includes/order", s.requireLANOrAuthForWrites(s.handleAPISectionIncludesOrder))
	s.mux.HandleFunc("/api/v1/missing-images", s.requireLANOrAuthForWrites(s.handleAPIMissingImages))
	// OIDC login flow
	if s.oidc != nil && s.oidc.Enabled() {
		s.mux.HandleFunc("/login", s.oidc.LoginHandler)
		s.mux.HandleFunc("/callback", s.oidc.CallbackHandler)
		s.mux.HandleFunc("/logout", s.oidc.LogoutHandler)
	}
	// Alexa skill endpoint
	if alexaCfg != nil {
		s.alexaHandler = newAlexaHandler(s, alexaCfg)
		s.mux.HandleFunc("/alexa", s.handleAlexa)
	}
	// API Explorer
	s.mux.HandleFunc("/api", s.handleAPIExplorer)
	// MCP — Streamable HTTP transport
	s.mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpSrv }, nil))
	return s
}

func mustInitOIDC(cfg auth.OIDCConfig, sm *auth.SessionManager) *auth.OIDCProvider {
	if cfg.Issuer == "" || cfg.ClientID == "" {
		return nil
	}
	provider, err := auth.NewOIDCProvider(context.Background(), cfg, sm)
	if err != nil {
		slog.Warn("failed to initialize OIDC provider; login disabled", "err", err)
		return nil
	}
	return provider
}

// Start begins listening on the configured port.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	slog.Info("server listening", "addr", "http://localhost"+addr)
	return http.ListenAndServe(addr, s.mux)
}

// isWAN returns true when the request arrived via the Cloudflare tunnel.
// Caddy's wan_auth snippet gates on the presence of Cf-Connecting-Ip (injected
// by Cloudflare's edge); LAN and Tailscale requests never carry this header.
// We reuse the same signal so both layers agree on what "WAN" means.
func isWAN(r *http.Request) bool {
	return r.Header.Get("Cf-Connecting-Ip") != ""
}

// isAuthenticated returns true if the request has a valid session cookie.
func (s *Server) isAuthenticated(r *http.Request) bool {
	if s.sessions == nil {
		return false
	}
	_, err := s.sessions.SessionFromRequest(r)
	return err == nil
}

// requireLANOrAuth blocks WAN requests unless the user is authenticated via OIDC.
func (s *Server) requireLANOrAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isWAN(r) && !s.isAuthenticated(r) {
			http.Error(w, "403 Forbidden — login required from the internet", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// requireLANOrAuthForWrites allows GET/HEAD from anywhere but blocks mutating
// methods from WAN unless the user is authenticated via OIDC.
func (s *Server) requireLANOrAuthForWrites(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && isWAN(r) && !s.isAuthenticated(r) {
			http.Error(w, "403 Forbidden — login required from the internet", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// pageAuth returns the settings visibility and an auth link for the request.
func (s *Server) pageAuth(r *http.Request) (showSettings bool, authLink string) {
	showSettings = !isWAN(r) || s.isAuthenticated(r)
	if s.sessions == nil || s.oidc == nil || !s.oidc.Enabled() {
		return
	}
	sess, err := s.sessions.SessionFromRequest(r)
	if err == nil && sess != nil {
		name := sess.Name
		if name == "" {
			name = sess.Email
		}
		if name == "" {
			name = "User"
		}
		authLink = fmt.Sprintf(`<span class="nav-btn" style="cursor:default">%s</span><a class="nav-btn" href="/logout">Logout</a>`, html.EscapeString(name))
		return
	}
	authLink = `<a class="nav-btn" href="/login">Login</a>`
	return
}

// exclusions returns per-school summary exclusion patterns from the store, or nil if no store.
func (s *Server) exclusions(schoolSlug string) []string {
	if s.store == nil {
		return nil
	}
	ex, _ := s.store.GetExclusions(schoolSlug)
	return ex
}

// sectionIncludes returns the section include list for a school+meal from the store.
// An empty result means "include all sections".
func (s *Server) sectionIncludes(schoolSlug, mealType string) []string {
	if s.store == nil {
		return nil
	}
	inc, _ := s.store.GetSectionIncludes(schoolSlug, mealType)
	return inc
}

// filterSections returns a copy of days with sections filtered and ordered by the include list.
// For "Option N" style includes (Woodmen Roberts): matching options are kept in includes order,
// then non-option side sections (Fruit, Vegetable, Milk, etc.) are appended after.
// For named-section style includes (EMS): ONLY the listed sections are kept, in includes order.
// If includes is empty, days is returned unchanged.
func filterSections(days map[string]menu.DayMenu, includes []string) map[string]menu.DayMenu {
	if len(includes) == 0 {
		return days
	}
	// Detect style: if any include starts with "Option ", treat as Option-N mode.
	optionMode := false
	for _, inc := range includes {
		if strings.HasPrefix(inc, "Option ") {
			optionMode = true
			break
		}
	}
	result := make(map[string]menu.DayMenu, len(days))
	for k, dm := range days {
		// Collect sections in includes order first.
		var secs []menu.Section
		for _, inc := range includes {
			for _, sec := range dm.Sections {
				if strings.EqualFold(sec.Name, inc) {
					secs = append(secs, sec)
					break
				}
			}
		}
		// In Option-N mode, also append non-option side sections not already included.
		if optionMode {
			incSet := make(map[string]bool, len(includes))
			for _, inc := range includes {
				incSet[strings.ToLower(inc)] = true
			}
			for _, sec := range dm.Sections {
				if !strings.HasPrefix(sec.Name, "Option") && !incSet[strings.ToLower(sec.Name)] {
					secs = append(secs, sec)
				}
			}
		}
		result[k] = menu.DayMenu{Date: dm.Date, Sections: secs}
	}
	return result
}

// resolveMenuImages overlays custom images from the store onto API-provided menus.
// Items that already have an API image keep it; missing images are filled from the store.
func (s *Server) resolveMenuImages(days map[string]menu.DayMenu) map[string]menu.DayMenu {
	if s.store == nil {
		return days
	}
	resolved := make(map[string]menu.DayMenu, len(days))
	for k, day := range days {
		secs := make([]menu.Section, len(day.Sections))
		for i, sec := range day.Sections {
			foods := make([]menu.Item, len(sec.Foods))
			for j, f := range sec.Foods {
				f.ImageURL = s.store.ResolveImage(f.Name, f.ImageURL)
				foods[j] = f
			}
			sec.Foods = foods
			secs[i] = sec
		}
		day.Sections = secs
		resolved[k] = day
	}
	return resolved
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	now := time.Now()
	http.Redirect(w, r,
		fmt.Sprintf("/calendar?view=month&year=%d&month=%d", now.Year(), int(now.Month())),
		http.StatusFound)
}

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	school := nutrislice.FindSchool(q.Get("school"))
	if school == nil {
		school = &nutrislice.DefaultSchools[0]
	}
	view := q.Get("view")
	if view == "" {
		view = "month"
	}

	now := time.Now()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	mealType := q.Get("meal")
	if mealType == "" {
		mealType = "lunch"
	}

	if view == "week" {
		d, err := parseQueryDate(q.Get("date"))
		if err != nil {
			d = now
		}
		week, err := s.client.FetchWeek(*school, d, mealType)
		if err != nil {
			http.Error(w, "failed to fetch menu: "+err.Error(), http.StatusInternalServerError)
			return
		}
		dayMenus, err := menu.ParseWeek(*week)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dayMenus = s.resolveMenuImages(dayMenus)
		weekIncludes := s.sectionIncludes(school.Slug, mealType)
		dayMenus = filterSections(dayMenus, weekIncludes)
		showSettings, authLink := s.pageAuth(r)
		writeWeekPage(w, dayMenus, *school, d, mealType, s.version, s.exclusions(school.Slug), weekIncludes, showSettings, authLink)
		return
	}

	// Month view
	year, _ := strconv.Atoi(q.Get("year"))
	month, _ := strconv.Atoi(q.Get("month"))
	if year == 0 {
		year = now.Year()
	}
	if month == 0 {
		month = int(now.Month())
	}

	// Fetch the month plus adjacent weeks (for roll-over days at month edges)
	weeks, err := s.client.FetchMonth(*school, year, month, mealType)
	if err != nil {
		http.Error(w, "failed to fetch menu: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// ParseWeek includes all weekdays regardless of month, so roll-over days are free
	dayMenus := make(map[string]menu.DayMenu)
	for _, week := range weeks {
		parsed, err := menu.ParseWeek(*week)
		if err != nil {
			slog.Warn("parse error", "err", err)
			continue
		}
		for k, v := range parsed {
			dayMenus[k] = v
		}
	}
	dayMenus = s.resolveMenuImages(dayMenus)
	monthIncludes := s.sectionIncludes(school.Slug, mealType)
	dayMenus = filterSections(dayMenus, monthIncludes)
	showSettings, authLink := s.pageAuth(r)
	writeCalendarPage(w, dayMenus, *school, year, month, mealType, s.version, monthIncludes, showSettings, authLink)
}

func (s *Server) handleAPILunch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	school := nutrislice.FindSchool(q.Get("school"))
	if school == nil {
		school = &nutrislice.DefaultSchools[0]
	}
	mealType := q.Get("meal")
	if mealType == "" {
		mealType = "lunch"
	}

	var d time.Time
	var err error
	switch strings.ToLower(q.Get("date")) {
	case "", "today":
		d = time.Now()
	case "tomorrow":
		d = time.Now().AddDate(0, 0, 1)
	default:
		d, err = time.Parse("2006-01-02", q.Get("date"))
		if err != nil {
			http.Error(w, "invalid date; use YYYY-MM-DD, 'today', or 'tomorrow'", http.StatusBadRequest)
			return
		}
	}

	week, err := s.client.FetchWeek(*school, d, mealType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dayMenus, err := menu.ParseWeek(*week)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	key := d.Format("2006-01-02")
	day, ok := dayMenus[key]

	w.Header().Set("Content-Type", "application/json")
	if !ok {
		json.NewEncoder(w).Encode(map[string]string{
			"date": key, "school": school.Name,
			"note": "no menu found — holiday or non-school day",
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"date": key, "school": school.Name, "sections": day.Sections,
	})
}

// ── REST API handlers ────────────────────────────────────────────────────────

func (s *Server) handleAPISchools(w http.ResponseWriter, _ *http.Request) {
	type schoolJSON struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		District string `json:"district"`
	}
	schools := make([]schoolJSON, len(nutrislice.DefaultSchools))
	for i, sc := range nutrislice.DefaultSchools {
		schools[i] = schoolJSON{Name: sc.Name, Slug: sc.Slug, District: sc.District}
	}
	writeJSON(w, map[string]any{"schools": schools})
}

func (s *Server) handleAPILunchWeek(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	school := nutrislice.FindSchool(q.Get("school"))
	if school == nil {
		school = &nutrislice.DefaultSchools[0]
	}
	mealType := q.Get("meal")
	if mealType == "" {
		mealType = "lunch"
	}
	d, err := parseQueryDate(q.Get("date"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	week, err := s.client.FetchWeek(*school, d, mealType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dayMenus, err := menu.ParseWeek(*week)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	days := make([]map[string]any, 0, len(dayMenus))
	for dateStr, dm := range dayMenus {
		days = append(days, map[string]any{
			"date":     dateStr,
			"sections": sectionsJSON(dm.Sections),
		})
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i]["date"].(string) < days[j]["date"].(string)
	})
	writeJSON(w, map[string]any{
		"school":  school.Name,
		"week_of": d.Format("2006-01-02"),
		"days":    days,
	})
}

func (s *Server) handleAPILunchMonth(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	school := nutrislice.FindSchool(q.Get("school"))
	if school == nil {
		school = &nutrislice.DefaultSchools[0]
	}
	mealType := q.Get("meal")
	if mealType == "" {
		mealType = "lunch"
	}
	now := time.Now()
	year, _ := strconv.Atoi(q.Get("year"))
	month, _ := strconv.Atoi(q.Get("month"))
	if year == 0 {
		year = now.Year()
	}
	if month == 0 {
		month = int(now.Month())
	}
	weeks, err := s.client.FetchMonth(*school, year, month, mealType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	days := make([]map[string]any, 0)
	for _, week := range weeks {
		parsed, err := menu.ParseWeek(*week)
		if err != nil {
			continue
		}
		for dateStr, dm := range parsed {
			days = append(days, map[string]any{
				"date":     dateStr,
				"sections": sectionsJSON(dm.Sections),
			})
		}
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i]["date"].(string) < days[j]["date"].(string)
	})
	writeJSON(w, map[string]any{
		"school": school.Name, "year": year, "month": month, "days": days,
	})
}

// ResolveSummary returns a voice-friendly menu summary for the given school,
// meal type, and date parameter. Supported dateParams: "today", "tomorrow",
// "next", weekdays, and ISO dates (YYYY-MM-DD).
func (s *Server) ResolveSummary(schoolSlug, mealType, dateParam string) (menu.Summary, error) {
	school := nutrislice.FindSchool(schoolSlug)
	if school == nil {
		school = &nutrislice.DefaultSchools[0]
	}
	if mealType == "" {
		mealType = "lunch"
	}

	dateParam = strings.ToLower(strings.TrimSpace(dateParam))
	if dateParam == "" {
		dateParam = "today"
	}

	var (
		day menu.DayMenu
		err error
	)
	if dateParam == "next" {
		day, err = s.findNextMenuDay(*school, mealType)
	} else {
		d, parseErr := parseQueryDate(dateParam)
		if parseErr != nil {
			return menu.Summary{}, parseErr
		}
		day, err = s.fetchDay(*school, d, mealType)
	}
	if err != nil {
		return menu.Summary{}, err
	}

	return menu.BuildSummary(day, school.Name, s.exclusions(school.Slug), s.sectionIncludes(school.Slug, mealType)), nil
}

func (s *Server) handleAPISummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	summary, err := s.ResolveSummary(q.Get("school"), q.Get("meal"), q.Get("date"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, summary)
}

// findNextMenuDay searches forward from tomorrow for the next school day that
// has at least one real entrée option (up to 14 calendar days ahead).
func (s *Server) findNextMenuDay(school nutrislice.School, mealType string) (menu.DayMenu, error) {
	now := time.Now()
	y, m, d := now.Date()
	start := time.Date(y, m, d+1, 0, 0, 0, 0, now.Location()) // start from tomorrow
	for i := 0; i < 14; i++ {
		candidate := start.AddDate(0, 0, i)
		if wd := candidate.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		day, err := s.fetchDay(school, candidate, mealType)
		if err != nil {
			continue
		}
		if day.HasMenu() {
			return day, nil
		}
	}
	return menu.DayMenu{}, fmt.Errorf("no upcoming menu found in the next 14 days")
}

// fetchDay retrieves and parses a single day's menu.
func (s *Server) fetchDay(school nutrislice.School, d time.Time, mealType string) (menu.DayMenu, error) {
	week, err := s.client.FetchWeek(school, d, mealType)
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

// sectionsJSON converts menu sections to a JSON-serialisable slice.
func sectionsJSON(sections []menu.Section) []map[string]any {
	out := make([]map[string]any, 0, len(sections))
	for _, sec := range sections {
		foods := make([]map[string]any, 0, len(sec.Foods))
		for _, f := range sec.Foods {
			food := map[string]any{"name": f.Name}
			if f.ImageURL != "" {
				food["image_url"] = f.ImageURL
			}
			if f.Calories > 0 {
				food["calories"] = f.Calories
			}
			if len(f.Tags) > 0 {
				food["tags"] = f.Tags
			}
			foods = append(foods, food)
		}
		out = append(out, map[string]any{"name": sec.Name, "foods": foods})
	}
	return out
}

// parseQueryDate parses date query params: today, tomorrow, or YYYY-MM-DD.
func parseQueryDate(s string) (time.Time, error) {
	switch strings.ToLower(s) {
	case "", "today":
		return time.Now(), nil
	case "tomorrow":
		return time.Now().AddDate(0, 0, 1), nil
	default:
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date %q — use YYYY-MM-DD, 'today', or 'tomorrow'", s)
		}
		return t, nil
	}
}

// writeJSON writes v as indented JSON with correct headers and CORS.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v) //nolint:errcheck
}

// ── HTML calendar ────────────────────────────────────────────────────────────

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

func writeCalendarPage(w http.ResponseWriter, days map[string]menu.DayMenu, school nutrislice.School, year, month int, mealType, version string, sectionOrder []string, showSettings bool, authLink string) {
	monthName := time.Month(month).String()
	prevM, prevY := month-1, year
	if prevM == 0 {
		prevM, prevY = 12, year-1
	}
	nextM, nextY := month+1, year
	if nextM == 13 {
		nextM, nextY = 1, year+1
	}

	// JSON for modal
	jsonDays := make(map[string][]jsonSection)
	for k, day := range days {
		secs := make([]jsonSection, 0, len(day.Sections))
		for _, sec := range day.Sections {
			js := jsonSection{Name: sec.Name, Foods: make([]jsonFood, 0, len(sec.Foods))}
			for _, f := range sec.Foods {
				js.Foods = append(js.Foods, jsonFood{
					Name: f.Name, ImageURL: f.ImageURL,
					Calories: f.Calories, Tags: f.Tags,
				})
			}
			secs = append(secs, js)
		}
		jsonDays[k] = secs
	}
	menuJSON, _ := json.Marshal(jsonDays)

	// Dynamic CLR/EMOJI maps for JS from actual sections in the data
	clrMap := make(map[string][2]string)
	emojiMap := make(map[string]string)
	for _, day := range days {
		for _, sec := range day.Sections {
			if _, ok := clrMap[sec.Name]; ok {
				continue
			}
			text, bg, _ := optionColor(sec.Name)
			clrMap[sec.Name] = [2]string{text, bg}
			emojiMap[sec.Name] = sectionEmoji(sec.Name)
		}
	}
	clrJSON, _ := json.Marshal(clrMap)
	emojiJSON, _ := json.Marshal(emojiMap)

	// Dynamic legend — only option sections that appear in the data
	seen := make(map[string]bool)
	var legend strings.Builder
	for _, day := range days {
		for _, sec := range day.Sections {
			if seen[sec.Name] {
				continue
			}
			text, _, isOpt := optionColor(sec.Name)
			if !isOpt {
				continue
			}
			seen[sec.Name] = true
			fmt.Fprintf(&legend, `<div class="leg-item"><span class="leg-dot" style="background:%s"></span>%s</div>`,
				text, html.EscapeString(sec.Name))
		}
	}

	// Calendar rows
	var rows strings.Builder
	for _, week := range calendarWeeks(year, month) {
		rows.WriteString(`<div class="week-row">`)
		for _, cd := range week {
			key := cd.Date.Format("2006-01-02")
			dm, hasDayMenu := days[key]
			rows.WriteString(monthDayCell(cd, dm, hasDayMenu))
		}
		rows.WriteString(`</div>`)
	}

	// Section order JSON for JS modal — prefer configured includes order, fall back to default.
	order := modalSectionOrder
	if len(sectionOrder) > 0 {
		order = sectionOrder
	}
	orderJSON, _ := json.Marshal(order)

	// School+meal selector
	schoolSel := buildSchoolSelector("month", school.Slug, mealType, year, month, "")

	weekLink := fmt.Sprintf("/calendar?view=week&date=%s&school=%s&meal=%s",
		time.Now().Format("2006-01-02"), school.Slug, mealType)
	repl := strings.NewReplacer(
		"[[TITLE]]", html.EscapeString(school.Name)+" — "+strings.ToUpper(mealType[:1])+mealType[1:]+" — "+monthName+" "+strconv.Itoa(year),
		"[[MONTH_YEAR]]", monthName+" "+strconv.Itoa(year),
		"[[SCHOOL]]", html.EscapeString(school.Name),
		"[[SCHOOL_SHORT]]", html.EscapeString(school.ShortName),
		"[[MEAL_LABEL]]", strings.ToUpper(mealType[:1])+mealType[1:],
		"[[SCHOOL_SLUG]]", school.Slug,
		"[[MEAL]]", mealType,
		"[[PREV_YEAR]]", strconv.Itoa(prevY),
		"[[PREV_MONTH]]", strconv.Itoa(prevM),
		"[[PREV_ABBR]]", time.Month(prevM).String()[:3],
		"[[NEXT_YEAR]]", strconv.Itoa(nextY),
		"[[NEXT_MONTH]]", strconv.Itoa(nextM),
		"[[NEXT_ABBR]]", time.Month(nextM).String()[:3],
		"[[WEEK_LINK]]", weekLink,
		"[[TODAY_BTN]]", func() string {
			now := time.Now()
			if year == now.Year() && month == int(now.Month()) {
				return ""
			}
			href := fmt.Sprintf("/calendar?view=month&year=%d&month=%d&school=%s&meal=%s", now.Year(), int(now.Month()), school.Slug, mealType)
			return `<a class="today-link" href="` + href + `">&#x21A9; today</a>`
		}(),
		"[[SCHOOL_SEL]]", schoolSel,
		"[[LEGEND]]", legend.String(),
		"[[ROWS]]", rows.String(),
		"[[MENU_JSON]]", string(menuJSON),
		"[[ORDER_JSON]]", string(orderJSON),
		"[[CLR_JSON]]", string(clrJSON),
		"[[EMOJI_JSON]]", string(emojiJSON),
		"[[VERSION]]", version,
		"[[AUTH_LINK]]", authLink,
		"[[SETTINGS_LINK]]", func() string {
			if showSettings {
				return `<a class="nav-btn" href="/settings" title="Settings">&#x2699;</a>`
			}
			return ""
		}(),
		"[[ENABLE_REORDER]]", func() string {
			if showSettings {
				return "true"
			}
			return "false"
		}(),
	)
	fmt.Fprint(w, repl.Replace(calendarPage))
}

// buildSchoolSelector generates the school+meal tab bar HTML.
// view is "month" or "week"; for month pass year/month; for week pass anchorDate.
func buildSchoolSelector(view, activeSchool, activeMeal string, year, month int, anchorDate string) string {
	var sb strings.Builder
	for _, sch := range nutrislice.DefaultSchools {
		sb.WriteString(`<div class="sch-grp">`)
		sb.WriteString(`<span class="sch-name">` + html.EscapeString(sch.ShortName) + `</span>`)
		for _, meal := range []string{"breakfast", "lunch"} {
			active := ""
			if sch.Slug == activeSchool && meal == activeMeal {
				active = " active"
			}
			var href string
			if view == "week" {
				href = fmt.Sprintf("/calendar?view=week&date=%s&school=%s&meal=%s", anchorDate, sch.Slug, meal)
			} else {
				href = fmt.Sprintf("/calendar?view=month&year=%d&month=%d&school=%s&meal=%s", year, month, sch.Slug, meal)
			}
			sb.WriteString(fmt.Sprintf(`<a class="meal-btn%s" href="%s">%s</a>`, active, href, strings.ToUpper(meal[:1])+meal[1:]))
		}
		sb.WriteString(`</div>`)
	}
	return sb.String()
}

// calDay is one cell in the calendar grid.
type calDay struct {
	Date    time.Time
	InMonth bool
}

// calendarWeeks returns Mon–Fri rows for the month.
// Days outside the month are included (roll-over) with InMonth=false so the
// last partial week always shows through Friday.
func calendarWeeks(year, month int) [][]calDay {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	startMonday := first.AddDate(0, 0, -int(first.Weekday()-time.Monday))
	if first.Weekday() == time.Sunday {
		startMonday = first.AddDate(0, 0, 1)
	}
	last := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.Local)

	var weeks [][]calDay
	for d := startMonday; !d.After(last); d = d.AddDate(0, 0, 7) {
		week := make([]calDay, 5)
		anyInMonth := false
		for i := 0; i < 5; i++ {
			day := d.AddDate(0, 0, i)
			inMonth := int(day.Month()) == month
			week[i] = calDay{Date: day, InMonth: inMonth}
			if inMonth {
				anyInMonth = true
			}
		}
		if anyInMonth {
			weeks = append(weeks, week) // always full 5-day row (roll-over included)
		}
	}
	return weeks
}

// monthDayCell renders one cell in the monthly calendar grid.
func monthDayCell(cd calDay, day menu.DayMenu, hasMenu bool) string {
	d := cd.Date
	now := time.Now()
	y, m, dd := now.Date()
	today := time.Date(y, m, dd, 0, 0, 0, 0, now.Location())

	isToday := d.Equal(today)
	isPast := d.Before(today)

	todayCls := ""
	if isToday {
		todayCls = " today"
	}
	otherCls := ""
	if !cd.InMonth {
		otherCls = " other-month"
	}

	// Past days: show the cell but hide menu data — no point scrolling through yesterday's lunch.
	if isPast {
		return fmt.Sprintf(
			`<div class="day-cell no-school past%s%s"><div class="day-num"><span class="dow">%s</span>%d</div></div>`,
			todayCls, otherCls, d.Format("Mon"), d.Day())
	}

	if !hasMenu {
		return fmt.Sprintf(
			`<div class="day-cell no-school%s%s"><div class="day-num"><span class="dow">%s</span>%d</div><div class="no-data">—</div></div>`,
			todayCls, otherCls, d.Format("Mon"), d.Day())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, `<div class="day-cell%s%s" onclick="openDay('%s')">`, todayCls, otherCls, d.Format("2006-01-02"))
	fmt.Fprintf(&sb, `<div class="day-num"><span class="dow">%s</span>%d</div>`, d.Format("Mon"), d.Day())

	// Detect style: if any "Option N" section exists, only show those (Woodmen Roberts).
	// Otherwise show all non-empty sections (Eagleview named-section style).
	hasOptions := false
	for _, sec := range day.Sections {
		if _, _, ok := optionColor(sec.Name); ok {
			hasOptions = true
			break
		}
	}

	const maxVisible = 3
	shown := 0
	total := 0
	paletteIdx := 0
	for _, sec := range day.Sections {
		text, bg, isOpt := optionColor(sec.Name)
		if len(sec.Foods) == 0 {
			continue
		}
		if hasOptions && !isOpt {
			continue
		}
		if !hasOptions {
			c := optionPalette[paletteIdx%len(optionPalette)]
			text, bg = c[0], c[1]
			paletteIdx++
		}
		total++
		if shown >= maxVisible {
			continue
		}
		shown++
		p := sec.Foods[0]
		name := p.Name
		if len(name) > 22 {
			name = name[:20] + "…"
		}
		var label string
		if isOpt {
			label = "Opt " + strings.TrimPrefix(sec.Name, "Option ")
		} else {
			label = sec.Name
		}
		img := ""
		if p.ImageURL != "" {
			img = fmt.Sprintf(`<img src="%s" alt="%s" class="opt-img" loading="lazy">`,
				p.ImageURL, html.EscapeString(p.Name))
		}
		fmt.Fprintf(&sb,
			`<div class="opt" style="border-left-color:%s;background:%s"><span class="opt-lbl" style="color:%s">%s</span><span class="opt-name">%s</span>%s</div>`,
			text, bg, text, html.EscapeString(label), html.EscapeString(name), img)
	}
	if total > maxVisible {
		fmt.Fprintf(&sb, `<div class="opt-more">+%d more</div>`, total-maxVisible)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// writeWeekPage renders a detailed single-week view.
func writeWeekPage(w http.ResponseWriter, days map[string]menu.DayMenu, school nutrislice.School, anchor time.Time, mealType, version string, exclusions []string, sectionOrder []string, showSettings bool, authLink string) {
	// Find Mon of this week
	monday := anchor.AddDate(0, 0, -int(anchor.Weekday()-time.Monday))
	if anchor.Weekday() == time.Sunday {
		monday = anchor.AddDate(0, 0, 1)
	}

	weekLabel := monday.Format("Jan 2") + " – " + monday.AddDate(0, 0, 4).Format("Jan 2, 2006")
	prevDate := monday.AddDate(0, 0, -7).Format("2006-01-02")
	nextDate := monday.AddDate(0, 0, 7).Format("2006-01-02")
	monthLink := fmt.Sprintf("/calendar?view=month&year=%d&month=%d&school=%s&meal=%s",
		anchor.Year(), int(anchor.Month()), school.Slug, mealType)
	weekOrder := modalSectionOrder
	if len(sectionOrder) > 0 {
		weekOrder = sectionOrder
	}
	orderJSON, _ := json.Marshal(weekOrder)

	// Dynamic CLR/EMOJI for JS modal
	clrMap := make(map[string][2]string)
	emojiMap := make(map[string]string)
	for _, dm := range days {
		for _, sec := range dm.Sections {
			if _, ok := clrMap[sec.Name]; ok {
				continue
			}
			text, bg, _ := optionColor(sec.Name)
			clrMap[sec.Name] = [2]string{text, bg}
			emojiMap[sec.Name] = sectionEmoji(sec.Name)
		}
	}
	clrJSON, _ := json.Marshal(clrMap)
	emojiJSON, _ := json.Marshal(emojiMap)

	var cols strings.Builder
	for i := 0; i < 5; i++ {
		d := monday.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		todayCls := ""
		if key == time.Now().Format("2006-01-02") {
			todayCls = " wk-today"
		}
		fmt.Fprintf(&cols, `<div class="wk-col%s">`, todayCls)
		nsURL := fmt.Sprintf("https://%s.nutrislice.com/menu/%s/%s/%s/",
			school.District, school.Slug, mealType, d.Format("2006-01-02"))
		fmt.Fprintf(&cols, `<div class="wk-date"><div class="wk-dow">%s</div><div class="wk-day">%d</div><div class="wk-mon">%s</div><a class="wk-ns-link" href="%s" target="_blank" rel="noopener" title="View on Nutrislice">↗</a></div>`,
			d.Format("Monday"), d.Day(), d.Format("Jan"), nsURL)

		dm, ok := days[key]
		if !ok {
			cols.WriteString(`<div class="wk-no-school">No school</div>`)
		} else {
			for _, sec := range dm.Sections {
				text, bg, isOpt := optionColor(sec.Name)
				if len(sec.Foods) == 0 {
					continue
				}
				if isOpt {
					label := strings.TrimPrefix(sec.Name, "Option ")
					fmt.Fprintf(&cols,
						`<div class="wk-opt" style="border-top:3px solid %s;background:%s">`,
						text, bg)
					fmt.Fprintf(&cols,
						`<div class="wk-opt-lbl" style="color:%s">Option %s</div>`, text, label)
					for _, f := range sec.Foods {
						if menu.IsExcluded(f.Name, exclusions) {
							continue
						}
						img := ""
						if f.ImageURL != "" {
							img = fmt.Sprintf(`<img src="%s" alt="%s" class="wk-img" loading="lazy">`,
								f.ImageURL, html.EscapeString(f.Name))
						}
						cal := ""
						if f.Calories > 0 {
							cal = fmt.Sprintf(`<span class="wk-cal">%d cal</span>`, f.Calories)
						}
						fmt.Fprintf(&cols,
							`<div class="wk-food">%s<div class="wk-food-info"><div class="wk-food-name">%s</div>%s</div></div>`,
							img, html.EscapeString(f.Name), cal)
					}
					cols.WriteString(`</div>`)
				} else {
					// Sides: vegetable, fruit, milk, condiments
					var sideNames []string
					for _, f := range sec.Foods {
						if !menu.IsExcluded(f.Name, exclusions) {
							sideNames = append(sideNames, html.EscapeString(f.Name))
						}
					}
					names := sideNames
					fmt.Fprintf(&cols,
						`<div class="wk-side"><span class="wk-side-lbl">%s</span> %s</div>`,
						html.EscapeString(sec.Name), strings.Join(names, " · "))
				}
			}
		}
		cols.WriteString(`</div>`)
	}

	mealLabel := strings.ToUpper(mealType[:1]) + mealType[1:]
	schoolSel := buildSchoolSelector("week", school.Slug, mealType, 0, 0, anchor.Format("2006-01-02"))
	repl := strings.NewReplacer(
		"[[TITLE]]", html.EscapeString(school.Name)+" — "+mealLabel+" — "+weekLabel,
		"[[WEEK_LABEL]]", weekLabel,
		"[[SCHOOL]]", html.EscapeString(school.Name),
		"[[SCHOOL_SHORT]]", html.EscapeString(school.ShortName),
		"[[MEAL_LABEL]]", mealLabel,
		"[[PREV_DATE]]", prevDate,
		"[[NEXT_DATE]]", nextDate,
		"[[SCHOOL_SLUG]]", school.Slug,
		"[[MEAL]]", mealType,
		"[[MONTH_LINK]]", monthLink,
		"[[TODAY_BTN]]", func() string {
			now := time.Now()
			todayMon := now.AddDate(0, 0, -int(now.Weekday()-time.Monday))
			if monday.Format("2006-01-02") == todayMon.Format("2006-01-02") {
				return ""
			}
			href := fmt.Sprintf("/calendar?view=week&date=%s&school=%s&meal=%s", now.Format("2006-01-02"), school.Slug, mealType)
			return `<a class="today-link" href="` + href + `">&#x21A9; today</a>`
		}(),
		"[[SCHOOL_SEL]]", schoolSel,
		"[[COLS]]", cols.String(),
		"[[ORDER_JSON]]", string(orderJSON),
		"[[CLR_JSON]]", string(clrJSON),
		"[[EMOJI_JSON]]", string(emojiJSON),
		"[[VERSION]]", version,
		"[[AUTH_LINK]]", authLink,
		"[[SETTINGS_LINK]]", func() string {
			if showSettings {
				return `<a class="nav-btn" href="/settings" title="Settings">&#x2699;</a>`
			}
			return ""
		}(),
		"[[ENABLE_REORDER]]", func() string {
			if showSettings {
				return "true"
			}
			return "false"
		}(),
	)
	fmt.Fprint(w, repl.Replace(weekPage))
}

func sortedSections(sections []menu.Section) []menu.Section {
	idx := make(map[string]int, len(modalSectionOrder))
	for i, name := range modalSectionOrder {
		idx[name] = i
	}
	sorted := make([]menu.Section, len(sections))
	copy(sorted, sections)
	sort.Slice(sorted, func(i, j int) bool {
		oi, okI := idx[sorted[i].Name]
		oj, okJ := idx[sorted[j].Name]
		if !okI {
			oi = 99
		}
		if !okJ {
			oj = 99
		}
		return oi < oj
	})
	return sorted
}

// calendarPage is the self-contained HTML template; [[PLACEHOLDERS]] are substituted at runtime.
const calendarPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>[[TITLE]]</title>
  <style>
    *{box-sizing:border-box;margin:0;padding:0}
    body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#F1F5F9;color:#1E293B;min-height:100vh}
    header{background:#0F172A;color:#fff;padding:1rem 1.5rem;display:flex;align-items:center;justify-content:space-between;gap:1rem}
    .hdr-left h1{font-size:1.4rem;font-weight:700;letter-spacing:-0.02em}
    .hdr-left p{font-size:.8rem;color:#94A3B8;margin-top:.15rem}
    .ver-badge{font-size:.65rem;color:#475569;background:#1E293B;border:1px solid #334155;padding:.1rem .45rem;border-radius:20px;vertical-align:middle;margin-left:.4rem;font-weight:500}
    .month-nav{display:flex;gap:.5rem;align-items:center}
    .nav-btn{background:#1E293B;border:1px solid #334155;color:#CBD5E1;padding:.4rem .9rem;border-radius:6px;cursor:pointer;font-size:.85rem;text-decoration:none;transition:background .15s;white-space:nowrap}
    .nav-btn:hover{background:#334155;color:#fff}
    .today-link{font-size:.7rem;color:#60A5FA;text-decoration:none;border:1px solid #1E3A5F;border-radius:20px;padding:.08rem .5rem;margin-left:.5rem;transition:all .15s;vertical-align:middle}
    .today-link:hover{background:#1E3A5F;color:#BFDBFE}
    .view-toggle{display:flex;background:#1E293B;border:1px solid #334155;border-radius:6px;overflow:hidden;margin-left:.5rem}
    .view-btn{padding:.4rem .85rem;font-size:.82rem;font-weight:600;text-decoration:none;color:#94A3B8;transition:background .15s;white-space:nowrap}
    .view-btn:hover{background:#334155;color:#fff}
    .view-btn.active{background:#3B82F6;color:#fff}
    .legend{display:flex;gap:1.25rem;flex-wrap:wrap;padding:.6rem 1.5rem;background:#fff;border-bottom:1px solid #E2E8F0}
    .leg-item{display:flex;align-items:center;gap:.35rem;font-size:.78rem;color:#374151;font-weight:500}
    .leg-dot{width:9px;height:9px;border-radius:50%;flex-shrink:0}
    .cal-wrap{max-width:1440px;margin:1.25rem auto;padding:0 1rem}
    .day-headers{display:grid;grid-template-columns:repeat(5,1fr);gap:.4rem;margin-bottom:.4rem}
    .day-hdr{text-align:center;font-size:.72rem;font-weight:700;text-transform:uppercase;letter-spacing:.06em;color:#64748B;padding:.4rem}
    .week-row{display:grid;grid-template-columns:repeat(5,1fr);gap:.4rem;margin-bottom:.4rem}
    .day-cell{background:#fff;border:1px solid #E2E8F0;border-radius:10px;padding:.55rem .6rem;min-height:170px;cursor:pointer;transition:box-shadow .15s,transform .1s;overflow:hidden}
    .day-cell:hover{box-shadow:0 6px 24px rgba(0,0,0,.10);transform:translateY(-2px)}
    .day-cell.today{border:2px solid #3B82F6}
    .day-cell.no-school{background:#F8FAFC;cursor:default}
    .day-cell.no-school:hover,.day-cell.day-pad:hover,.day-cell.past:hover{box-shadow:none;transform:none}
    .day-cell.day-pad{background:transparent;border:1px dashed #CBD5E1;cursor:default;min-height:170px}
    .day-cell.other-month{background:#F1F5F9}
    .day-cell.past{background:#F8FAFC;cursor:default;opacity:.4}
    .no-data{color:#CBD5E1;font-size:.75rem;text-align:center;margin-top:.75rem}
    .opt-more{font-size:.65rem;color:#64748B;text-align:center;padding:.1rem 0;margin-top:.1rem}
    .school-bar{background:#1E293B;padding:.4rem 1.5rem;display:flex;gap:1.5rem;flex-wrap:wrap}
    .sch-grp{display:flex;align-items:center;gap:.35rem}
    .sch-name{font-size:.72rem;font-weight:600;color:#94A3B8;white-space:nowrap}
    .meal-btn{font-size:.72rem;padding:.25rem .65rem;border-radius:20px;text-decoration:none;color:#94A3B8;border:1px solid #334155;transition:background .15s}
    .meal-btn:hover{background:#334155;color:#CBD5E1}
    .meal-btn.active{background:#3B82F6;color:#fff;border-color:#3B82F6}
    .day-num{font-size:1rem;font-weight:700;color:#334155;margin-bottom:.45rem;display:flex;align-items:center;gap:.35rem}
    .dow{font-size:.65rem;font-weight:600;text-transform:uppercase;color:#94A3B8;letter-spacing:.04em}
    .today .day-num{color:#1D4ED8}
    .opt{display:flex;align-items:center;gap:.35rem;padding:.22rem .4rem;border-left:3px solid;border-radius:0 5px 5px 0;margin-bottom:.25rem;font-size:.72rem}
    .opt-lbl{font-weight:800;font-size:.6rem;white-space:nowrap;flex-shrink:0;min-width:30px}
    .opt-name{flex:1;color:#1E293B;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
    .opt-img{width:30px;height:30px;object-fit:cover;border-radius:4px;flex-shrink:0}
    .overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,.55);z-index:200;backdrop-filter:blur(3px);align-items:center;justify-content:center}
    .overlay.open{display:flex;animation:fadeIn .15s ease}
    @keyframes fadeIn{from{opacity:0}to{opacity:1}}
    .modal{background:#fff;border-radius:16px;width:92%;max-width:580px;max-height:88vh;overflow-y:auto;box-shadow:0 30px 70px rgba(0,0,0,.35);animation:slideUp .18s ease}
    @keyframes slideUp{from{transform:translateY(18px);opacity:0}to{transform:translateY(0);opacity:1}}
    .modal-hdr{padding:.9rem 1.1rem;border-bottom:1px solid #E2E8F0;display:flex;align-items:center;gap:.45rem;position:sticky;top:0;background:#fff;border-radius:16px 16px 0 0;z-index:1}
    .modal-title{font-size:1.1rem;font-weight:700;color:#0F172A}
    .modal-sub{font-size:.73rem;color:#64748B;margin-top:.1rem}
    .modal-hdr-mid{flex:1;min-width:0;text-align:center}
    .close-btn{background:#F1F5F9;border:none;width:30px;height:30px;border-radius:50%;cursor:pointer;font-size:1rem;color:#64748B;display:flex;align-items:center;justify-content:center;flex-shrink:0}
    .close-btn:hover{background:#E2E8F0}
    .modal-nav{background:none;border:1px solid #E2E8F0;color:#94A3B8;width:30px;height:30px;border-radius:8px;cursor:pointer;font-size:1.25rem;flex-shrink:0;display:flex;align-items:center;justify-content:center;padding:0;transition:all .15s}
    .modal-nav:hover:not([disabled]){background:#F1F5F9;color:#0F172A;border-color:#CBD5E1}
    .modal-nav[disabled]{opacity:.2;cursor:default}
    .modal-body{padding:1.1rem 1.4rem}
    .m-sec{margin-bottom:1.1rem}
    .m-sec-hdr{font-size:.72rem;font-weight:800;text-transform:uppercase;letter-spacing:.06em;padding:.3rem .6rem;border-radius:4px;margin-bottom:.5rem;border-left:3px solid;display:flex;align-items:center;gap:.35rem}
    .m-drag-handle{color:#CBD5E1;font-size:.85rem;cursor:grab;flex-shrink:0;line-height:1}
    .m-sec[draggable="true"]:hover .m-drag-handle{color:#94A3B8}
    .m-sec.m-drag-over{outline:2px dashed #3B82F6;outline-offset:2px;border-radius:8px}
    .m-sec[draggable="true"]{cursor:default;transition:opacity .15s}
    .m-sec[draggable="true"].dragging{opacity:.45}
    .m-save-bar{display:flex;align-items:center;gap:.75rem;padding:.75rem 0 .25rem;margin-top:.25rem;border-top:1px solid #E2E8F0}
    .m-save-btn{background:#3B82F6;border:none;color:#fff;padding:.38rem .9rem;border-radius:6px;font-size:.82rem;font-weight:600;cursor:pointer}
    .m-save-btn:hover{background:#2563EB}
    .m-food{display:flex;align-items:center;gap:.75rem;padding:.45rem 0;border-bottom:1px solid #F1F5F9}
    .m-food:last-child{border-bottom:none}
    .m-food-img{width:54px;height:54px;object-fit:cover;border-radius:8px;flex-shrink:0}
    .m-placeholder{width:54px;height:54px;border-radius:8px;background:#F8FAFC;border:1.5px dashed #E2E8F0;flex-shrink:0;display:flex;flex-direction:column;align-items:center;justify-content:center;font-size:1.3rem;text-decoration:none;gap:1px;transition:all .15s;color:inherit}
    .m-placeholder:hover{background:#EFF6FF;border-color:#93C5FD}
    .m-add-lbl{font-size:.48rem;font-weight:700;text-transform:uppercase;letter-spacing:.04em;color:#CBD5E1}
    .m-placeholder:hover .m-add-lbl{color:#3B82F6}
    .m-food-info{flex:1;min-width:0}
    .m-food-name{font-weight:600;font-size:.92rem;color:#1E293B}
    .m-food-meta{font-size:.75rem;color:#64748B;margin-top:.15rem}
    .m-tags{display:flex;gap:.35rem;flex-wrap:wrap;margin-top:.3rem}
    .tag{font-size:.65rem;padding:.15rem .45rem;border-radius:20px;background:#F1F5F9;color:#475569;font-weight:600}
    @media(max-width:640px){
      header{flex-wrap:wrap;padding:.75rem 1rem}
      .hdr-left h1{font-size:1.05rem}
      .ver-badge{display:none}
      .month-nav{gap:.3rem;flex-wrap:wrap}
      .nav-btn{padding:.3rem .55rem;font-size:.75rem}
      .view-btn{padding:.3rem .55rem;font-size:.72rem}
      .school-bar{padding:.35rem .75rem;gap:.5rem}
      .sch-name{display:none}
      .legend{padding:.4rem .75rem;gap:.6rem}
      .leg-item{font-size:.7rem}
      .cal-wrap{padding:0 .25rem;margin:.5rem auto;overflow-x:auto}
      .day-headers,.week-row{min-width:520px}
      .day-hdr{font-size:.58rem;padding:.25rem .1rem}
      .day-cell{min-height:80px;padding:.28rem .22rem}
      .day-cell.day-pad{min-height:80px}
      .day-num{font-size:.78rem;margin-bottom:.2rem}
      .opt{padding:.12rem .2rem;margin-bottom:.12rem;font-size:.62rem;gap:.2rem}
      .opt-lbl{font-size:.52rem;min-width:20px}
      .opt-img{width:20px;height:20px}
      .opt-name{font-size:.6rem}
      .opt-more{font-size:.58rem}
      .overlay{align-items:flex-end}
      .modal{width:100%;max-width:none;border-radius:16px 16px 0 0;position:fixed;bottom:0;top:auto;max-height:90vh}
      .modal-body{padding:.85rem 1rem}
      .m-food-img{width:42px;height:42px}
      .m-placeholder{width:42px;height:42px}
    }
  </style>
</head>
<body>
<header>
  <div class="hdr-left">
    <h1>[[MONTH_YEAR]]<span class="ver-badge">v[[VERSION]]</span></h1>
    <p>[[SCHOOL]] &middot; [[MEAL_LABEL]][[TODAY_BTN]]</p>
  </div>
  <nav class="month-nav">
    <a class="nav-btn" href="/calendar?view=month&year=[[PREV_YEAR]]&month=[[PREV_MONTH]]&school=[[SCHOOL_SLUG]]&meal=[[MEAL]]">&lsaquo; [[PREV_ABBR]]</a>
    <a class="nav-btn" href="/calendar?view=month&year=[[NEXT_YEAR]]&month=[[NEXT_MONTH]]&school=[[SCHOOL_SLUG]]&meal=[[MEAL]]">[[NEXT_ABBR]] &rsaquo;</a>
    <div class="view-toggle">
      <a class="view-btn active" href="#">Month</a>
      <a class="view-btn" href="[[WEEK_LINK]]">Week</a>
    </div>
    <a class="nav-btn" href="/api" title="API Explorer">API</a>
    [[SETTINGS_LINK]]
    [[AUTH_LINK]]
  </nav>
</header>

<div class="school-bar">[[SCHOOL_SEL]]</div>
<div class="legend">[[LEGEND]]</div>

<div class="cal-wrap">
  <div class="day-headers">
    <div class="day-hdr">Monday</div>
    <div class="day-hdr">Tuesday</div>
    <div class="day-hdr">Wednesday</div>
    <div class="day-hdr">Thursday</div>
    <div class="day-hdr">Friday</div>
  </div>
  [[ROWS]]
</div>

<div class="overlay" id="overlay" onclick="if(event.target===this)closeModal()">
  <div class="modal">
    <div class="modal-hdr">
      <button class="modal-nav" id="modal-prev" onclick="modalNav(-1)">&#x2039;</button>
      <div class="modal-hdr-mid">
        <div class="modal-title" id="modal-title"></div>
        <div class="modal-sub">[[SCHOOL_SHORT]] &middot; [[MEAL_LABEL]]</div>
      </div>
      <button class="modal-nav" id="modal-next" onclick="modalNav(1)">&#x203A;</button>
      <button class="close-btn" onclick="closeModal()">&#x2715;</button>
    </div>
    <div class="modal-body" id="modal-body"></div>
  </div>
</div>

<script>
var MENU = [[MENU_JSON]];
var ORDER = [[ORDER_JSON]];
var CLR = [[CLR_JSON]];
var EMOJI = [[EMOJI_JSON]];
var enableReorder = [[ENABLE_REORDER]];
var DATES = Object.keys(MENU).sort();
var CUR_DATE = null;
var SCHOOL = '[[SCHOOL_SLUG]]';
var MEAL = '[[MEAL]]';
var mDragSrc = null;

function esc(s){ return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;'); }

function updateModalNav() {
  var idx = DATES.indexOf(CUR_DATE);
  var prev = document.getElementById('modal-prev');
  var next = document.getElementById('modal-next');
  if (idx <= 0) { prev.setAttribute('disabled',''); } else { prev.removeAttribute('disabled'); }
  if (idx >= DATES.length - 1) { next.setAttribute('disabled',''); } else { next.removeAttribute('disabled'); }
}
function modalNav(dir) {
  var idx = DATES.indexOf(CUR_DATE);
  var n = idx + dir;
  if (n >= 0 && n < DATES.length) openDay(DATES[n]);
}

function openDay(dateStr) {
  var secs = MENU[dateStr];
  if (!secs || !secs.length) return;
  CUR_DATE = dateStr;
  updateModalNav();
  var d = new Date(dateStr + 'T12:00:00');
  document.getElementById('modal-title').textContent =
    d.toLocaleDateString('en-US', {weekday:'long', month:'long', day:'numeric', year:'numeric'});

  // Only apply ORDER sorting for "Option N" style menus (Woodmen Roberts).
  // Named-section schools (EMS) already have sections in the correct API order.
  var hasOptions = secs.some(function(s){ return /^Option \d+$/.test(s.name); });
  var sorted = hasOptions ? secs.slice().sort(function(a,b){
    var ai = ORDER.indexOf(a.name), bi = ORDER.indexOf(b.name);
    return (ai < 0 ? 99 : ai) - (bi < 0 ? 99 : bi);
  }) : secs;

  var isTouch = !enableReorder || ('ontouchstart' in window) || (navigator.maxTouchPoints > 0);
  var html = '';
  for (var i = 0; i < sorted.length; i++) {
    var sec = sorted[i];
    if (!sec.foods || !sec.foods.length) continue;
    var clr = CLR[sec.name] || ['#64748B','#F8FAFC'];
    var tc = clr[0], bg = clr[1];
    var em = EMOJI[sec.name] || '&#x1F374;';
    var dragAttrs = isTouch ? '' : ' draggable="true" data-sec-name="' + esc(sec.name) + '"';
    var handle = isTouch ? '' : '<span class="m-drag-handle">&#x2807;</span>';

    html += '<div class="m-sec"' + dragAttrs + '>';
    html += '<div class="m-sec-hdr" style="color:' + tc + ';background:' + bg + ';border-left-color:' + tc + '">' + handle + em + ' ' + esc(sec.name) + '</div>';

    for (var j = 0; j < sec.foods.length; j++) {
      var f = sec.foods[j];
      var img = f.image_url
        ? '<img src="' + esc(f.image_url) + '" alt="' + esc(f.name) + '" class="m-food-img" loading="lazy">'
        : '<a href="/settings?add_image=' + encodeURIComponent(f.name) + '" class="m-placeholder" title="Add image for ' + esc(f.name) + '">' + em + '<span class="m-add-lbl">+ image</span></a>';
      var cal = f.calories ? '<span>' + f.calories + ' cal</span>' : '';
      var tags = '';
      if (f.tags && f.tags.length) {
        for (var k = 0; k < f.tags.length; k++) {
          tags += '<span class="tag">' + esc(f.tags[k]) + '</span>';
        }
      }
      html += '<div class="m-food">' + img;
      html += '<div class="m-food-info">';
      html += '<div class="m-food-name">' + esc(f.name) + '</div>';
      html += '<div class="m-food-meta">' + cal + '</div>';
      if (tags) html += '<div class="m-tags">' + tags + '</div>';
      html += '</div></div>';
    }
    html += '</div>';
  }
  if (!isTouch) {
    html += '<div class="m-save-bar" id="m-save-bar" style="display:none">' +
      '<button class="m-save-btn" onclick="modalSaveOrder()">Save Section Order</button>' +
      '<span id="m-save-status" style="font-size:.78rem;color:#64748B"></span></div>';
  }

  var body = document.getElementById('modal-body');
  body.innerHTML = html;
  if (!isTouch) setupModalDrag(body);
  document.getElementById('overlay').classList.add('open');
  document.body.style.overflow = 'hidden';
}

function setupModalDrag(body) {
  var secs = body.querySelectorAll('.m-sec[draggable]');
  secs.forEach(function(sec) {
    sec.addEventListener('dragstart', function(e) {
      mDragSrc = sec;
      e.dataTransfer.effectAllowed = 'move';
      setTimeout(function(){ sec.classList.add('dragging'); }, 0);
    });
    sec.addEventListener('dragend', function() {
      mDragSrc = null;
      sec.classList.remove('dragging');
      body.querySelectorAll('.m-sec').forEach(function(s){ s.classList.remove('m-drag-over'); });
    });
    sec.addEventListener('dragover', function(e) {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      body.querySelectorAll('.m-sec').forEach(function(s){ s.classList.remove('m-drag-over'); });
      sec.classList.add('m-drag-over');
    });
    sec.addEventListener('dragleave', function(e) {
      if (!sec.contains(e.relatedTarget)) sec.classList.remove('m-drag-over');
    });
    sec.addEventListener('drop', function(e) {
      e.preventDefault();
      sec.classList.remove('m-drag-over');
      if (!mDragSrc || mDragSrc === sec) return;
      var rect = sec.getBoundingClientRect();
      if (e.clientY < rect.top + rect.height / 2) {
        body.insertBefore(mDragSrc, sec);
      } else {
        sec.after ? sec.after(mDragSrc) : body.insertBefore(mDragSrc, sec.nextSibling);
      }
      var bar = document.getElementById('m-save-bar');
      if (bar) bar.style.display = 'flex';
    });
  });
}

function modalSaveOrder() {
  var secs = document.getElementById('modal-body').querySelectorAll('.m-sec[data-sec-name]');
  var names = [];
  secs.forEach(function(s){ names.push(s.getAttribute('data-sec-name')); });
  var status = document.getElementById('m-save-status');
  status.textContent = 'Saving\u2026';
  fetch('/api/v1/section-includes/order', {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({school_slug: SCHOOL, meal_type: MEAL, sections: names})
  }).then(function(r) {
    if (r.ok || r.status === 204) {
      status.textContent = 'Saved!';
      ORDER = names;
    } else {
      status.textContent = 'Error saving.';
    }
  }).catch(function(){ status.textContent = 'Error saving.'; });
}

function closeModal() {
  document.getElementById('overlay').classList.remove('open');
  document.body.style.overflow = '';
}

document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') closeModal();
  if (document.getElementById('overlay').classList.contains('open')) {
    if (e.key === 'ArrowLeft') modalNav(-1);
    if (e.key === 'ArrowRight') modalNav(1);
  }
});
</script>
</body>
</html>`

// weekPage is the self-contained HTML template for the single-week view.
const weekPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>[[TITLE]]</title>
  <style>
    *{box-sizing:border-box;margin:0;padding:0}
    body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#F1F5F9;color:#1E293B;min-height:100vh}
    header{background:#0F172A;color:#fff;padding:1rem 1.5rem;display:flex;align-items:center;justify-content:space-between;gap:1rem;flex-wrap:wrap}
    .hdr-left h1{font-size:1.25rem;font-weight:700;letter-spacing:-0.02em}
    .hdr-left p{font-size:.8rem;color:#94A3B8;margin-top:.15rem}
    .ver-badge{font-size:.65rem;color:#475569;background:#1E293B;border:1px solid #334155;padding:.1rem .45rem;border-radius:20px;vertical-align:middle;margin-left:.4rem;font-weight:500}
    .hdr-right{display:flex;gap:.5rem;align-items:center;flex-wrap:wrap}
    .nav-btn{background:#1E293B;border:1px solid #334155;color:#CBD5E1;padding:.4rem .9rem;border-radius:6px;font-size:.85rem;text-decoration:none;transition:background .15s;white-space:nowrap}
    .nav-btn:hover{background:#334155;color:#fff}
    .today-link{font-size:.7rem;color:#60A5FA;text-decoration:none;border:1px solid #1E3A5F;border-radius:20px;padding:.08rem .5rem;margin-left:.5rem;transition:all .15s;vertical-align:middle}
    .today-link:hover{background:#1E3A5F;color:#BFDBFE}
    .view-toggle{display:flex;background:#1E293B;border:1px solid #334155;border-radius:6px;overflow:hidden}
    .view-btn{padding:.4rem .85rem;font-size:.82rem;font-weight:600;text-decoration:none;color:#94A3B8;transition:background .15s}
    .view-btn:hover{background:#334155;color:#fff}
    .view-btn.active{background:#3B82F6;color:#fff}
    .school-bar{background:#1E293B;padding:.4rem 1.5rem;display:flex;gap:1.5rem;flex-wrap:wrap}
    .sch-grp{display:flex;align-items:center;gap:.35rem}
    .sch-name{font-size:.72rem;font-weight:600;color:#94A3B8;white-space:nowrap}
    .meal-btn{font-size:.72rem;padding:.25rem .65rem;border-radius:20px;text-decoration:none;color:#94A3B8;border:1px solid #334155;transition:background .15s}
    .meal-btn:hover{background:#334155;color:#CBD5E1}
    .meal-btn.active{background:#3B82F6;color:#fff;border-color:#3B82F6}
    .wk-wrap{max-width:1440px;margin:1.25rem auto;padding:0 1rem;display:grid;grid-template-columns:repeat(5,1fr);gap:.75rem}
    .wk-col{background:#fff;border:1px solid #E2E8F0;border-radius:12px;overflow:hidden;display:flex;flex-direction:column}
    .wk-col.wk-today{border:2px solid #3B82F6}
    .wk-date{padding:.75rem 1rem;background:#F8FAFC;border-bottom:1px solid #E2E8F0;text-align:center}
    .wk-today .wk-date{background:#EFF6FF}
    .wk-dow{font-size:.7rem;font-weight:800;text-transform:uppercase;letter-spacing:.08em;color:#64748B}
    .wk-day{font-size:2rem;font-weight:800;color:#0F172A;line-height:1}
    .wk-today .wk-day{color:#1D4ED8}
    .wk-mon{font-size:.75rem;color:#94A3B8;font-weight:500;margin-top:.1rem}
    .wk-ns-link{display:block;margin-top:.3rem;font-size:.65rem;color:#94A3B8;text-decoration:none;opacity:.6;transition:opacity .15s}.wk-ns-link:hover{opacity:1;color:#3B82F6}
    .wk-opt{margin:.6rem .6rem 0;border-radius:8px;overflow:hidden}
    .wk-opt-lbl{font-size:.65rem;font-weight:800;text-transform:uppercase;letter-spacing:.06em;padding:.3rem .6rem}
    .wk-food{display:flex;align-items:center;gap:.5rem;padding:.4rem .6rem;background:rgba(255,255,255,.7)}
    .wk-food:not(:last-child){border-bottom:1px solid rgba(0,0,0,.05)}
    .wk-img{width:40px;height:40px;object-fit:cover;border-radius:6px;flex-shrink:0}
    .wk-food-info{flex:1;min-width:0}
    .wk-food-name{font-size:.82rem;font-weight:600;color:#1E293B;line-height:1.25}
    .wk-cal{font-size:.68rem;color:#94A3B8;margin-top:.1rem;display:block}
    .wk-side{margin:.35rem .6rem 0;font-size:.72rem;color:#64748B;padding:.2rem 0;border-top:1px solid #F1F5F9}
    .wk-side-lbl{font-weight:700;color:#475569}
    .wk-no-school{flex:1;display:flex;align-items:center;justify-content:center;color:#CBD5E1;font-size:.82rem;padding:2rem}
    .wk-col > *:last-child{margin-bottom:.6rem}
    @media(max-width:640px){
      header{padding:.75rem 1rem}
      .hdr-left h1{font-size:1rem}
      .ver-badge{display:none}
      .hdr-right{gap:.3rem;flex-wrap:wrap}
      .nav-btn{padding:.3rem .55rem;font-size:.75rem}
      .view-btn{padding:.3rem .55rem;font-size:.72rem}
      .school-bar{padding:.35rem .75rem;gap:.5rem}
      .sch-name{display:none}
      .wk-wrap{overflow-x:auto;grid-template-columns:repeat(5,minmax(145px,1fr));padding:0 .25rem;margin:.5rem auto}
      .wk-col{min-width:145px}
      .wk-day{font-size:1.5rem}
      .wk-food-name{font-size:.75rem}
      .wk-img{width:32px;height:32px}
      .wk-opt-lbl{font-size:.6rem}
    }
  </style>
</head>
<body>
<header>
  <div class="hdr-left">
    <h1>[[WEEK_LABEL]]<span class="ver-badge">v[[VERSION]]</span></h1>
    <p>[[SCHOOL]] &middot; [[MEAL_LABEL]][[TODAY_BTN]]</p>
  </div>
  <div class="hdr-right">
    <a class="nav-btn" href="/calendar?view=week&date=[[PREV_DATE]]&school=[[SCHOOL_SLUG]]&meal=[[MEAL]]">&lsaquo; Prev week</a>
    <a class="nav-btn" href="/calendar?view=week&date=[[NEXT_DATE]]&school=[[SCHOOL_SLUG]]&meal=[[MEAL]]">Next week &rsaquo;</a>
    <div class="view-toggle">
      <a class="view-btn" href="[[MONTH_LINK]]">Month</a>
      <a class="view-btn active" href="#">Week</a>
    </div>
    <a class="nav-btn" href="/api" title="API Explorer">API</a>
    [[SETTINGS_LINK]]
    [[AUTH_LINK]]
  </div>
</header>
<div class="school-bar">[[SCHOOL_SEL]]</div>
<div class="wk-wrap">
  [[COLS]]
</div>
</body>
</html>`
