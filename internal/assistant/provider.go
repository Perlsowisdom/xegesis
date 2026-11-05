// Package assistant is the AI assistant: a chat with a model chosen by the user that can read the
// library through read-only tools, each result passing a licence gate, and suggests drafts the user
// inserts. It never changes the library itself (ADR 0011).
package assistant

import "context"

// Message is one turn of a conversation, in the shape Chat Completions uses.
type Message struct {
	Role       string     `json:"role"` // system, user, assistant or tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is the model asking to run a tool.
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"arguments"` // JSON object
}

// ToolDef describes a tool to the model.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Request is one call to a model.
type Request struct {
	Model    string
	Messages []Message
	Tools    []ToolDef
}

// Turn is the model's finished reply to one Request: text, tool calls, or both.
type Turn struct {
	Text      string
	ToolCalls []ToolCall
}

// Provider is a model service. Stream sends the request and passes reply text to onText as it
// arrives; cancelling ctx stops the reply. Models lists the models the service offers.
type Provider interface {
	Stream(ctx context.Context, req Request, onText func(string)) (Turn, error)
	Models(ctx context.Context) ([]string, error)
}

// Error is a provider failure in plain words, with a kind callers can act on.
type Error struct {
	Kind    string // bad_key, no_connection, rate_limited, no_tools, no_model, failed
	Message string
}

func (e *Error) Error() string { return e.Message }
