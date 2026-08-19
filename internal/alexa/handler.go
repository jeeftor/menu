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
		cfg.DefaultSchool = "woodman-roberts-elementary-school"
	}
	return &Handler{cfg: cfg}
}

// ServeHTTP handles an incoming Alexa request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		slog.Warn("alexa: rejected non-POST request", "method", r.Method, "remote", r.RemoteAddr)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		slog.Error("alexa: reading body", "err", err, "remote", r.RemoteAddr)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var env RequestEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		slog.Error("alexa: parsing request", "err", err, "remote", r.RemoteAddr, "body", string(body))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if env.Request == nil {
		slog.Warn("alexa: missing request object", "remote", r.RemoteAddr)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	reqID := env.Request.RequestID
	slog.Info("alexa: request received",
		"request_id", reqID,
		"request_type", env.Request.Type,
		"app_id", applicationID(&env),
		"timestamp", env.Request.Timestamp,
		"locale", env.Request.Locale,
		"remote", r.RemoteAddr,
	)

	if h.cfg.VerifyRequests {
		if err := verifyHTTPRequest(body, r, &env); err != nil {
			slog.Warn("alexa: request verification failed",
				"request_id", reqID,
				"err", err,
				"signature", r.Header.Get("Signature") != "",
				"cert_url", r.Header.Get("SignatureCertChainUrl"),
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		slog.Info("alexa: request signature verified", "request_id", reqID)
	}

	if h.cfg.ApplicationID != "" {
		id := applicationID(&env)
		if id != "" && id != h.cfg.ApplicationID {
			slog.Warn("alexa: application ID mismatch",
				"request_id", reqID,
				"got", id,
				"expected", h.cfg.ApplicationID,
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		slog.Info("alexa: application ID matched", "request_id", reqID, "app_id", id)
	}

	resp := h.handle(&env)
	respText := ""
	if resp.Response.OutputSpeech != nil {
		respText = resp.Response.OutputSpeech.Text
	}
	slog.Info("alexa: response sent",
		"request_id", reqID,
		"request_type", env.Request.Type,
		"response", respText,
		"should_end_session", resp.Response.ShouldEndSession,
	)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("alexa: encoding response", "request_id", reqID, "err", err)
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
		slog.Warn("alexa: received nil intent")
		resp.Response.OutputSpeech = unknownSpeech()
		return
	}

	slog.Info("alexa: handling intent", "intent", intent.Name, "slots", slotMap(intent))

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
		slog.Warn("alexa: unknown intent", "intent", intent.Name)
		resp.Response.OutputSpeech = unknownSpeech()
	}
}

func (h *Handler) handleMenuQuery(intent *Intent, resp *ResponseEnvelope) {
	dateSlot := slotValue(intent, "date")
	dateParam := h.resolveDateSlot(dateSlot)

	slog.Info("alexa: menu query",
		"raw_slot", dateSlot,
		"resolved_date_param", dateParam,
		"school", h.cfg.DefaultSchool,
		"meal", h.cfg.DefaultMeal,
	)

	summary, err := h.cfg.ResolveSummary(dateParam, h.cfg.DefaultSchool, h.cfg.DefaultMeal)
	if err != nil {
		slog.Error("alexa: resolving summary failed",
			"date_param", dateParam,
			"school", h.cfg.DefaultSchool,
			"meal", h.cfg.DefaultMeal,
			"err", err,
		)
		resp.Response.OutputSpeech = &OutputSpeech{
			Type: "PlainText",
			Text: "I couldn't look up the menu right now. Please try again later.",
		}
		return
	}

	slog.Info("alexa: summary resolved",
		"date_param", dateParam,
		"resolved_date", summary.Date,
		"school", summary.School,
		"text", summary.Text,
	)

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

// slotMap returns a flat map of slot names to values for logging.
func slotMap(intent *Intent) map[string]string {
	if intent == nil || intent.Slots == nil {
		return nil
	}
	m := make(map[string]string, len(intent.Slots))
	for k, v := range intent.Slots {
		m[k] = v.Value
	}
	return m
}

func unknownSpeech() *OutputSpeech {
	return &OutputSpeech{
		Type: "PlainText",
		Text: "I'm not sure about that. Try asking what's for lunch today or tomorrow.",
	}
}
