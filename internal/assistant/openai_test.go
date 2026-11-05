package assistant

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func adapter(f *fakeServer, key string) *OpenAI {
	return &OpenAI{BaseURL: f.URL + "/v1", Key: key, Client: f.Client()}
}

func TestStreamTextAndToolCalls(t *testing.T) {
	f := newFake(t, func(map[string]any) step {
		return step{text: []string{"Grace ", "and peace."}, calls: [][2]string{{"read_passage", `{"ref":"Romans 8:28"}`}, {"list_notes", `{}`}}}
	})
	var pieces []string
	turn, err := adapter(f, "k").Stream(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}, Tools: ToolDefs()},
		func(s string) { pieces = append(pieces, s) })
	if err != nil {
		t.Fatal(err)
	}
	if turn.Text != "Grace and peace." || len(pieces) != 2 {
		t.Fatalf("text %q pieces %v", turn.Text, pieces)
	}
	if len(turn.ToolCalls) != 2 || turn.ToolCalls[0].Name != "read_passage" || turn.ToolCalls[0].Args != `{"ref":"Romans 8:28"}` || turn.ToolCalls[1].ID != "call_1" {
		t.Fatalf("calls %+v", turn.ToolCalls)
	}
	body := f.requests()[0]
	if body["stream"] != true || body["model"] != "m" || len(body["tools"].([]any)) != len(Tools()) {
		t.Fatalf("body %v", body)
	}
}

func TestModels(t *testing.T) {
	f := newFake(t, nil)
	ms, err := adapter(f, "k").Models(context.Background())
	if err != nil || len(ms) != 2 || ms[0] != "model-a" {
		t.Fatalf("%v %v", ms, err)
	}
	// A server without a model list gives none, not an error.
	o := &OpenAI{BaseURL: f.URL + "/nothing", Client: f.Client()}
	if ms, err := o.Models(context.Background()); err != nil || len(ms) != 0 {
		t.Fatalf("%v %v", ms, err)
	}
	if _, err := adapter(f, "bad").Models(context.Background()); kind(err) != "bad_key" {
		t.Fatalf("bad key: %v", err)
	}
}

func kind(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

func TestErrorsInPlainWords(t *testing.T) {
	cases := []struct {
		status int
		msg    string
		want   string
	}{
		{http.StatusUnauthorized, "nope", "bad_key"},
		{http.StatusTooManyRequests, "slow down", "rate_limited"},
		{http.StatusBadRequest, "registry.ollama.ai/library/gemma does not support tools", "no_tools"},
		{http.StatusNotFound, "model not found", "no_model"},
		{http.StatusInternalServerError, "boom", "failed"},
	}
	for _, c := range cases {
		f := newFake(t, func(map[string]any) step { return step{status: c.status, errMsg: c.msg} })
		_, err := adapter(f, "k").Stream(context.Background(), Request{Model: "m"}, nil)
		if kind(err) != c.want {
			t.Errorf("%d %q: got %v, want %s", c.status, c.msg, err, c.want)
		}
		f.Close()
	}
	// Nothing listening: no connection.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	o := &OpenAI{BaseURL: "http://" + addr + "/v1", Client: &http.Client{}}
	if _, err := o.Stream(context.Background(), Request{Model: "m"}, nil); kind(err) != "no_connection" {
		t.Fatalf("no connection: %v", err)
	}
}

func TestStreamCancel(t *testing.T) {
	f := newFake(t, func(map[string]any) step { return step{text: []string{"first", "never"}} })
	f.block = make(chan struct{})
	defer close(f.block)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := adapter(f, "").Stream(ctx, Request{Model: "m"}, func(string) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want cancelled, got %v", err)
	}
}
