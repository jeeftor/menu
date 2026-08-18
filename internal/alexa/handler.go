package alexa

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"menu/internal/menu"
)

// Config configures the Alexa request handler.
type Config struct {
	// ApplicationID is the Alexa skill ID. If empty, application ID is not checked.
	ApplicationID string
	// VerifyRequests enables Amazon request signature and timestamp verification.
	VerifyRequests bool
	// DefaultSchool is the school slug to use when the user does not specify one.
	DefaultSchool string
	// DefaultMeal is "lunch" or "breakfast".
	DefaultMeal string
	// ResolveSummary returns the menu summary for a date/school/meal triple.
	ResolveSummary func(date, school, meal string) (menu.Summary, error)
}

// Handler is an http.Handler that processes ASK requests.
type Handler struct {
	cfg Config
}

// New returns an Alexa handler.
func New(cfg Config) *Handler {
	if cfg.DefaultMeal == "" {
		cfg.DefaultMeal = "lunch"
	}
	if cfg.DefaultSchool == "" {
		cfg.DefaultSchool = "woodmen-roberts-elementary-school"
	}
	return &Handler{cfg: cfg}
}

// ServeHTTP handles an incoming Alexa request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		slog.Error("alexa: reading body", "err", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var env RequestEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		slog.Error("alexa: parsing request", "err", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if h.cfg.VerifyRequests {
		if err := verifyHTTPRequest(body, r, &env); err != nil {
			slog.Warn("alexa: request verification failed", "err", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	if h.cfg.ApplicationID != "" {
		id := applicationID(&env)
		if id != "" && id != h.cfg.ApplicationID {
			slog.Warn("alexa: application ID mismatch", "got", id)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	resp := h.handle(&env)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("alexa: encoding response", "err", err)
	}
}

func verifyHTTPRequest(body []byte, r *http.Request, env *RequestEnvelope) error {
	if err := VerifyTimestamp(env.Request.Timestamp, defaultMaxTimestampAge); err != nil {
		return err
	}
	return VerifySignature(body, r.Header.Get("Signature"), r.Header.Get("SignatureCertChainUrl"))
}

// applicationID returns the skill application ID from the request.
func applicationID(env *RequestEnvelope) string {
	if env.Session != nil {
		return env.Session.Application.ApplicationID
	}
	if env.Context != nil {
		return env.Context.System.Application.ApplicationID
	}
	return ""
}

func (h *Handler) handle(env *RequestEnvelope) *ResponseEnvelope {
	resp := &ResponseEnvelope{
		Version:  "1.0",
		Response: Response{ShouldEndSession: true},
	}
	if env.Session != nil {
		resp.SessionAttributes = env.Session.Attributes
	}

	switch env.Request.Type {
	case "LaunchRequest":
		resp.Response.ShouldEndSession = false
		resp.Response.OutputSpeech = &OutputSpeech{
			Type: "PlainText",
			Text: "Ask me what's for lunch today or tomorrow.",
		}
	case "IntentRequest":
		h.handleIntent(env.Request.Intent, resp)
	case "SessionEndedRequest":
		// No response content required.
	default:
		resp.Response.OutputSpeech = &OutputSpeech{
			Type: "PlainText",
			Text: "I'm not sure how to help with that.",
		}
	}
	return resp
}

func (h *Handler) handleIntent(intent *Intent, resp *ResponseEnvelope) {
	if intent == nil {
		resp.Response.OutputSpeech = unknownSpeech()
		return
	}

	switch intent.Name {
	case "MenuQueryIntent":
		h.handleMenuQuery(intent, resp)
	case "AMAZON.HelpIntent":
		resp.Response.OutputSpeech = &OutputSpeech{
			Type: "PlainText",
			Text: "You can ask me what's for lunch today, tomorrow, or any day of the week.",
		}
		resp.Response.ShouldEndSession = false
	case "AMAZON.CancelIntent", "AMAZON.StopIntent":
		resp.Response.OutputSpeech = &OutputSpeech{
			Type: "PlainText",
			Text: "Goodbye.",
		}
	case "AMAZON.FallbackIntent":
		resp.Response.OutputSpeech = unknownSpeech()
		resp.Response.ShouldEndSession = false
	default:
		resp.Response.OutputSpeech = unknownSpeech()
	}
}

func (h *Handler) handleMenuQuery(intent *Intent, resp *ResponseEnvelope) {
	dateSlot := slotValue(intent, "date")
	dateParam := h.resolveDateSlot(dateSlot)

	summary, err := h.cfg.ResolveSummary(dateParam, h.cfg.DefaultSchool, h.cfg.DefaultMeal)
	if err != nil {
		slog.Error("alexa: resolving summary", "err", err)
		resp.Response.OutputSpeech = &OutputSpeech{
			Type: "PlainText",
			Text: "I couldn't look up the menu right now. Please try again later.",
		}
		return
	}

	text := summary.Text
	if text == "" {
		text = fmt.Sprintf("There's no %s menu for %s at %s.", h.cfg.DefaultMeal, summary.Date, summary.School)
	}
	resp.Response.OutputSpeech = &OutputSpeech{
		Type: "PlainText",
		Text: fmt.Sprintf("For %s at %s, %s.", summary.Date, summary.School, text),
	}
	resp.Response.Card = &Card{
		Type:    "Simple",
		Title:   fmt.Sprintf("%s for %s", capitalize(h.cfg.DefaultMeal), summary.Date),
		Content: text,
	}
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] = r[0] - 'a' + 'A'
	}
	return string(r)
}

// resolveDateSlot maps an AMAZON.DATE slot value to a date parameter understood
// by the menu resolver. AMAZON.DATE returns ISO dates ("2026-08-19"), "PRESENT_REF"
// for today, and "FUTURE_REF" / "PAST_REF" for relative references.
func (h *Handler) resolveDateSlot(slot string) string {
	switch strings.ToUpper(slot) {
	case "", "PRESENT_REF":
		return "today"
	case "FUTURE_REF":
		return "tomorrow"
	case "PAST_REF":
		return "yesterday"
	}
	// ISO date or weekday name already compatible with the resolver.
	if _, err := time.Parse("2006-01-02", slot); err == nil {
		return slot
	}
	return slot
}

func slotValue(intent *Intent, name string) string {
	if intent == nil || intent.Slots == nil {
		return ""
	}
	if s, ok := intent.Slots[name]; ok {
		return s.Value
	}
	return ""
}

func unknownSpeech() *OutputSpeech {
	return &OutputSpeech{
		Type: "PlainText",
		Text: "I'm not sure about that. Try asking what's for lunch today or tomorrow.",
	}
}
