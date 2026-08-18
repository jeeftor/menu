// Package alexa handles Amazon Alexa Skills Kit (ASK) requests for the menu server.
package alexa

// RequestEnvelope is the top-level JSON Alexa sends for every request.
type RequestEnvelope struct {
	Version string   `json:"version"`
	Session *Session `json:"session"`
	Context *Context `json:"context"`
	Request *Request `json:"request"`
}

// Session carries user/session metadata.
type Session struct {
	New         bool                   `json:"new"`
	SessionID   string                 `json:"sessionId"`
	Application Application            `json:"application"`
	Attributes  map[string]interface{} `json:"attributes"`
	User        User                   `json:"user"`
}

// Context carries the runtime context, including the skill application ID.
type Context struct {
	System struct {
		Application Application `json:"application"`
	} `json:"system"`
}

// Application identifies the calling skill.
type Application struct {
	ApplicationID string `json:"applicationId"`
}

// User identifies the caller.
type User struct {
	UserID string `json:"userId"`
}

// Request is the request-specific payload.
type Request struct {
	Type      string  `json:"type"`
	RequestID string  `json:"requestId"`
	Timestamp string  `json:"timestamp"`
	Locale    string  `json:"locale"`
	Intent    *Intent `json:"intent,omitempty"`
	Reason    string  `json:"reason,omitempty"`
}

// Intent represents an intent invocation and its slots.
type Intent struct {
	Name  string          `json:"name"`
	Slots map[string]Slot `json:"slots"`
}

// Slot carries the resolved value for an intent slot.
type Slot struct {
	Name        string           `json:"name"`
	Value       string           `json:"value"`
	Resolutions *SlotResolutions `json:"resolutions,omitempty"`
}

// SlotResolutions holds entity resolutions (optional).
type SlotResolutions struct {
	ResolutionsPerAuthority []struct {
		Values []struct {
			Value struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"value"`
		} `json:"values"`
	} `json:"resolutionsPerAuthority"`
}

// ResponseEnvelope is the JSON returned to Alexa.
type ResponseEnvelope struct {
	Version           string                 `json:"version"`
	SessionAttributes map[string]interface{} `json:"sessionAttributes,omitempty"`
	Response          Response               `json:"response"`
}

// Response contains the actual spoken reply.
type Response struct {
	OutputSpeech     *OutputSpeech `json:"outputSpeech,omitempty"`
	Card             *Card         `json:"card,omitempty"`
	Reprompt         *Reprompt     `json:"reprompt,omitempty"`
	ShouldEndSession bool          `json:"shouldEndSession"`
}

// OutputSpeech describes what Alexa says.
type OutputSpeech struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	SSML string `json:"ssml,omitempty"`
}

// Reprompt is played if the user does not reply.
type Reprompt struct {
	OutputSpeech OutputSpeech `json:"outputSpeech"`
}

// Card is shown in the Alexa app.
type Card struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
}
