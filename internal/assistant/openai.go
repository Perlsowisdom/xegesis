package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// OpenAI talks to any service with an OpenAI-compatible Chat Completions API: OpenAI, OpenRouter,
// Gemini's and Anthropic's compatible endpoints, and local servers such as Ollama and LM Studio.
type OpenAI struct {
	BaseURL string // e.g. https://api.openai.com/v1
	Key     string // may be empty for local servers
	Client  *http.Client
	Header  map[string]string // extra headers some services want
}

// DefaultClient is the HTTP client providers use: no overall timeout, since replies stream, but
// a limit on connecting and on waiting for the first byte.
func DefaultClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 2 * time.Minute
	return &http.Client{Transport: t}
}

func (o *OpenAI) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return DefaultClient()
}

func (o *OpenAI) endpoint(path string) string {
	return strings.TrimRight(o.BaseURL, "/") + path
}

func (o *OpenAI) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, o.endpoint(path), rd)
	if err != nil {
		return nil, &Error{Kind: "failed", Message: "The service address is not valid: " + err.Error()}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if o.Key != "" {
		req.Header.Set("Authorization", "Bearer "+o.Key)
	}
	for k, v := range o.Header {
		req.Header.Set(k, v)
	}
	return req, nil
}

type wireToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireTool struct {
	Type     string  `json:"type"`
	Function ToolDef `json:"function"`
}

// Body is the JSON sent for req: what the privacy panel shows.
func Body(req Request, stream bool) map[string]any {
	msgs := make([]wireMessage, len(req.Messages))
	for i, m := range req.Messages {
		content := m.Content
		w := wireMessage{Role: m.Role, Content: &content, ToolCallID: m.ToolCallID}
		if m.Role == "assistant" && content == "" && len(m.ToolCalls) > 0 {
			w.Content = nil
		}
		for _, c := range m.ToolCalls {
			tc := wireToolCall{ID: c.ID, Type: "function"}
			tc.Function.Name, tc.Function.Arguments = c.Name, c.Args
			w.ToolCalls = append(w.ToolCalls, tc)
		}
		msgs[i] = w
	}
	body := map[string]any{"model": req.Model, "messages": msgs}
	if stream {
		body["stream"] = true
	}
	if len(req.Tools) > 0 {
		tools := make([]wireTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = wireTool{Type: "function", Function: t}
		}
		body["tools"] = tools
	}
	return body
}

type chunk struct {
	Choices []struct {
		Delta struct {
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls"`
		} `json:"delta"`
		Message struct { // a reply that was not streamed
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error json.RawMessage `json:"error"`
}

func (o *OpenAI) Stream(ctx context.Context, req Request, onText func(string)) (Turn, error) {
	raw, err := json.Marshal(Body(req, true))
	if err != nil {
		return Turn{}, err
	}
	hr, err := o.newRequest(ctx, http.MethodPost, "/chat/completions", raw)
	if err != nil {
		return Turn{}, err
	}
	hr.Header.Set("Accept", "text/event-stream")
	resp, err := o.client().Do(hr)
	if err != nil {
		return Turn{}, o.netError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return Turn{}, statusError(resp, req.Model)
	}
	var acc turnBuilder
	if !strings.Contains(resp.Header.Get("Content-Type"), "event-stream") {
		// The service ignored "stream": one JSON reply.
		var c chunk
		if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&c); err != nil {
			return Turn{}, &Error{Kind: "failed", Message: "The service sent a reply Xegesis could not read."}
		}
		if err := chunkError(c); err != nil {
			return Turn{}, err
		}
		for _, ch := range c.Choices {
			acc.text(ch.Message.Content, onText)
			acc.calls(ch.Message.ToolCalls)
		}
		return acc.turn(), nil
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var c chunk
		if json.Unmarshal([]byte(data), &c) != nil {
			continue
		}
		if err := chunkError(c); err != nil {
			return Turn{}, err
		}
		for _, ch := range c.Choices {
			acc.text(ch.Delta.Content, onText)
			acc.calls(ch.Delta.ToolCalls)
		}
	}
	if err := sc.Err(); err != nil {
		return Turn{}, o.netError(ctx, err)
	}
	return acc.turn(), nil
}

// turnBuilder puts streamed pieces back together. Tool calls arrive in pieces keyed by index.
type turnBuilder struct {
	b     strings.Builder
	order []int
	tc    map[int]*ToolCall
}

func (t *turnBuilder) text(s string, onText func(string)) {
	if s == "" {
		return
	}
	t.b.WriteString(s)
	if onText != nil {
		onText(s)
	}
}

func (t *turnBuilder) calls(cs []wireToolCall) {
	if t.tc == nil {
		t.tc = map[int]*ToolCall{}
	}
	for _, c := range cs {
		i := len(t.order)
		if c.Index != nil {
			i = *c.Index
		} else if c.ID == "" && len(t.order) > 0 {
			i = t.order[len(t.order)-1] // a continuation without an index
		}
		cur, ok := t.tc[i]
		if !ok {
			cur = &ToolCall{}
			t.tc[i] = cur
			t.order = append(t.order, i)
		}
		if c.ID != "" {
			cur.ID = c.ID
		}
		cur.Name += c.Function.Name
		cur.Args += c.Function.Arguments
	}
}

func (t *turnBuilder) turn() Turn {
	out := Turn{Text: t.b.String()}
	sort.Ints(t.order)
	for n, i := range t.order {
		c := *t.tc[i]
		if c.ID == "" {
			c.ID = fmt.Sprintf("call_%d", n)
		}
		if strings.TrimSpace(c.Args) == "" {
			c.Args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, c)
	}
	return out
}

// Models lists the service's models, or none when it does not offer the list.
func (o *OpenAI) Models(ctx context.Context) ([]string, error) {
	hr, err := o.newRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.client().Do(hr)
	if err != nil {
		return nil, o.netError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return []string{}, nil
	}
	if resp.StatusCode/100 != 2 {
		return nil, statusError(resp, "")
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&list); err != nil {
		return nil, &Error{Kind: "failed", Message: "The service sent a model list Xegesis could not read."}
	}
	out := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		if id := strings.TrimPrefix(m.ID, "models/"); id != "" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (o *OpenAI) host() string {
	if u, err := url.Parse(o.BaseURL); err == nil && u.Host != "" {
		return u.Host
	}
	return o.BaseURL
}

func (o *OpenAI) netError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var ne net.Error
	var ue *url.Error
	if errors.As(err, &ne) || errors.As(err, &ue) {
		return &Error{Kind: "no_connection", Message: fmt.Sprintf("Could not connect to %s. Check the address and your internet connection, or that the local model is running.", o.host())}
	}
	return &Error{Kind: "no_connection", Message: "The connection to the service broke off: " + err.Error()}
}

// apiMessage is the message in an error body: {"error": {"message": ...}} or {"error": "..."}.
func apiMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &o) == nil && o.Message != "" {
		return o.Message
	}
	return string(raw)
}

func chunkError(c chunk) error {
	if m := apiMessage(c.Error); m != "" {
		return classify(0, m, "")
	}
	return nil
}

func statusError(resp *http.Response, model string) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var body struct {
		Error json.RawMessage `json:"error"`
	}
	msg := strings.TrimSpace(string(b))
	if json.Unmarshal(b, &body) == nil {
		if m := apiMessage(body.Error); m != "" {
			msg = m
		}
	}
	return classify(resp.StatusCode, msg, model)
}

// classify turns a service's error into plain words.
func classify(status int, msg, model string) error {
	low := strings.ToLower(msg)
	short := msg
	if len(short) > 200 {
		short = short[:200] + "…"
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden ||
		strings.Contains(low, "invalid api key") || strings.Contains(low, "incorrect api key"):
		return &Error{Kind: "bad_key", Message: "The service did not accept the key. Check it in Settings → Assistant → Model."}
	case status == http.StatusTooManyRequests || strings.Contains(low, "rate limit"):
		return &Error{Kind: "rate_limited", Message: "The service is limiting requests (rate limited). Wait a minute and try again."}
	case strings.Contains(low, "tool") && (strings.Contains(low, "support") || strings.Contains(low, "unsupported")):
		return &Error{Kind: "no_tools", Message: "This model cannot use tools, so it cannot read your library. Choose another model."}
	case status == http.StatusNotFound:
		if model != "" {
			return &Error{Kind: "no_model", Message: fmt.Sprintf("The service has no model %q, or its address is wrong.", model)}
		}
		return &Error{Kind: "no_model", Message: "The service address is wrong: nothing answers there."}
	}
	if short == "" {
		short = fmt.Sprintf("status %d", status)
	}
	return &Error{Kind: "failed", Message: "The service reported a problem: " + short}
}
