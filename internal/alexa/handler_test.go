package alexa

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menu/internal/menu"
)

func TestHandlerLaunchRequest(t *testing.T) {
	h := New(Config{
		VerifyRequests: false,
		ResolveSummary: func(_, _, _ string) (menu.Summary, error) {
			return menu.Summary{}, nil
		},
	})
	resp := post(t, h, &RequestEnvelope{
		Version: "1.0",
		Request: &Request{Type: "LaunchRequest"},
	})
	if !strings.Contains(resp.Response.OutputSpeech.Text, "Ask me") {
		t.Fatalf("unexpected response: %q", resp.Response.OutputSpeech.Text)
	}
	if resp.Response.ShouldEndSession {
		t.Fatal("launch request should keep session open")
	}
}

func TestHandlerMenuQueryIntent(t *testing.T) {
	h := New(Config{
		VerifyRequests: false,
		DefaultSchool:  "woodmen",
		DefaultMeal:    "lunch",
		ResolveSummary: func(_, _, _ string) (menu.Summary, error) {
			return menu.Summary{
				Date:    "2026-08-19",
				School:  "Woodmen Roberts Elementary",
				Options: []string{"Pizza", "Burger"},
				Text:    "Pizza or Burger",
			}, nil
		},
	})
	resp := post(t, h, &RequestEnvelope{
		Version: "1.0",
		Request: &Request{
			Type: "IntentRequest",
			Intent: &Intent{
				Name:  "MenuQueryIntent",
				Slots: map[string]Slot{"date": {Value: "tomorrow"}},
			},
		},
	})
	want := "For 2026-08-19 at Woodmen Roberts Elementary, Pizza or Burger."
	if resp.Response.OutputSpeech.Text != want {
		t.Fatalf("got %q, want %q", resp.Response.OutputSpeech.Text, want)
	}
	if resp.Response.Card == nil || resp.Response.Card.Type != "Simple" {
		t.Fatal("expected simple card")
	}
}

func TestHandlerMenuQueryNoMenu(t *testing.T) {
	h := New(Config{
		VerifyRequests: false,
		ResolveSummary: func(_, _, _ string) (menu.Summary, error) {
			return menu.Summary{Date: "2026-08-19", School: "Test School"}, nil
		},
	})
	resp := post(t, h, &RequestEnvelope{
		Version: "1.0",
		Request: &Request{
			Type:   "IntentRequest",
			Intent: &Intent{Name: "MenuQueryIntent"},
		},
	})
	if !strings.Contains(resp.Response.OutputSpeech.Text, "no lunch menu") {
		t.Fatalf("unexpected response: %q", resp.Response.OutputSpeech.Text)
	}
}

func TestHandlerMenuQueryResolverError(t *testing.T) {
	h := New(Config{
		VerifyRequests: false,
		ResolveSummary: func(_, _, _ string) (menu.Summary, error) {
			return menu.Summary{}, errors.New("boom")
		},
	})
	resp := post(t, h, &RequestEnvelope{
		Version: "1.0",
		Request: &Request{
			Type:   "IntentRequest",
			Intent: &Intent{Name: "MenuQueryIntent"},
		},
	})
	if !strings.Contains(resp.Response.OutputSpeech.Text, "couldn't look up") {
		t.Fatalf("unexpected response: %q", resp.Response.OutputSpeech.Text)
	}
}

func TestHandlerHelpIntent(t *testing.T) {
	h := New(Config{
		VerifyRequests: false,
		ResolveSummary: func(_, _, _ string) (menu.Summary, error) { return menu.Summary{}, nil },
	})
	resp := post(t, h, &RequestEnvelope{
		Version: "1.0",
		Request: &Request{Type: "IntentRequest", Intent: &Intent{Name: "AMAZON.HelpIntent"}},
	})
	if !strings.Contains(resp.Response.OutputSpeech.Text, "You can ask") {
		t.Fatalf("unexpected response: %q", resp.Response.OutputSpeech.Text)
	}
}

func TestHandlerStopIntent(t *testing.T) {
	h := New(Config{
		VerifyRequests: false,
		ResolveSummary: func(_, _, _ string) (menu.Summary, error) { return menu.Summary{}, nil },
	})
	resp := post(t, h, &RequestEnvelope{
		Version: "1.0",
		Request: &Request{Type: "IntentRequest", Intent: &Intent{Name: "AMAZON.StopIntent"}},
	})
	if !strings.Contains(resp.Response.OutputSpeech.Text, "Goodbye") {
		t.Fatalf("unexpected response: %q", resp.Response.OutputSpeech.Text)
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	h := New(Config{VerifyRequests: false})
	req := httptest.NewRequest(http.MethodGet, "/alexa", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestResolveDateSlot(t *testing.T) {
	h := &Handler{cfg: Config{}}
	cases := []struct{ in, want string }{
		{"", "today"},
		{"PRESENT_REF", "today"},
		{"FUTURE_REF", "tomorrow"},
		{"PAST_REF", "yesterday"},
		{"2026-08-19", "2026-08-19"},
		{"monday", "monday"},
	}
	for _, tc := range cases {
		got := h.resolveDateSlot(tc.in)
		if got != tc.want {
			t.Errorf("resolveDateSlot(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func post(t *testing.T, h http.Handler, env *RequestEnvelope) *ResponseEnvelope {
	t.Helper()
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/alexa", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp ResponseEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return &resp
}
