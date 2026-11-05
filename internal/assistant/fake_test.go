package assistant

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeServer is an OpenAI-compatible server for tests: it streams replies chosen by reply from
// the request it got, and records every request.
type fakeServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
	reply  func(body map[string]any) step
	block  chan struct{} // when set, a reply waits on it after its first piece
}

// step is one reply: text pieces, then tool calls (name, JSON arguments).
type step struct {
	status int
	errMsg string
	text   []string
	calls  [][2]string
}

func newFake(t *testing.T, reply func(map[string]any) step) *fakeServer {
	f := &fakeServer{reply: reply}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer bad" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":{"message":"Invalid API key"}}`)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"model-b"},{"id":"model-a"}]}`)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.mu.Unlock()
		s := f.reply(body)
		if s.status != 0 {
			w.WriteHeader(s.status)
			fmt.Fprintf(w, `{"error":{"message":%q}}`, s.errMsg)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		send := func(v any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
		for i, t := range s.text {
			send(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": t}}}})
			if i == 0 && f.block != nil {
				select {
				case <-f.block:
				case <-r.Context().Done():
					return
				}
			}
		}
		for i, c := range s.calls {
			// Each call in two pieces, as services send them.
			half := len(c[1]) / 2
			send(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{
				map[string]any{"index": i, "id": fmt.Sprintf("call_%d", i), "type": "function", "function": map[string]any{"name": c[0], "arguments": c[1][:half]}}}}}}})
			send(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{
				map[string]any{"index": i, "function": map[string]any{"arguments": c[1][half:]}}}}}}})
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServer) requests() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any{}, f.bodies...)
}

// lastMessage is the content of a request's last message.
func lastMessage(body map[string]any) (role, content string) {
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		return "", ""
	}
	m, _ := msgs[len(msgs)-1].(map[string]any)
	role, _ = m["role"].(string)
	content, _ = m["content"].(string)
	return role, content
}

func contains(s string, subs ...string) bool {
	for _, x := range subs {
		if !strings.Contains(s, x) {
			return false
		}
	}
	return true
}
