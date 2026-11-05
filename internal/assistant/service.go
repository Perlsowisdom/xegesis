package assistant

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Preset is a provider choice with its usual address.
type Preset struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	BaseURL  string `json:"base_url"`
	Local    bool   `json:"local" desc:"Runs on this computer (when its address is this computer)"`
	NeedsKey bool   `json:"needs_key"`
}

// Presets are the providers the OpenAI-compatible adapter serves. Claude Code and Codex (T36b)
// are not among them.
var Presets = []Preset{
	{"claude", "Claude", "https://api.anthropic.com/v1", false, true},
	{"openai", "OpenAI", "https://api.openai.com/v1", false, true},
	{"gemini", "Gemini", "https://generativelanguage.googleapis.com/v1beta/openai", false, true},
	{"openrouter", "OpenRouter", "https://openrouter.ai/api/v1", false, true},
	{"local", "Local model", "http://localhost:11434/v1", true, false},
	{"custom", "Custom", "", false, false},
}

func preset(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// config is what library.db keeps (settings key "assistant"). The key is never part of it.
type config struct {
	Provider  string            `json:"provider"`
	BaseURLs  map[string]string `json:"base_urls,omitempty"`
	Models    map[string]string `json:"models,omitempty"`
	Confirmed []string          `json:"confirmed,omitempty"` // provider|address pairs whose privacy panel was confirmed
}

const settingsKey = "assistant"

func keyName(provider string) string { return "assistant." + provider + ".key" }

// Config is the assistant's settings as shown to the person.
type Config struct {
	Provider  string   `json:"provider" desc:"off, or a preset id"`
	Label     string   `json:"label,omitempty"`
	BaseURL   string   `json:"base_url,omitempty"`
	Model     string   `json:"model,omitempty"`
	Local     bool     `json:"local" desc:"The provider runs on this computer, so personal copies of books may be sent to it"`
	KeySaved  bool     `json:"key_saved"`
	NeedsKey  bool     `json:"needs_key"`
	Confirmed bool     `json:"confirmed" desc:"The person has seen what is sent to this provider and agreed"`
	Presets   []Preset `json:"presets"`
}

// Option changes a service, for tests.
type Option func(*service)

// WithProvider makes providers with fn instead of the OpenAI-compatible adapter over the network.
func WithProvider(fn func(baseURL, key string, p Preset) Provider) Option {
	return func(s *service) { s.newProvider = fn }
}

type service struct {
	reg         *command.Registry
	keys        secrets.Store
	newProvider func(baseURL, key string, p Preset) Provider
	mu          sync.Mutex
	convs       map[string]*conversation
}

// Register adds the assistant commands to r. Tools run other commands through r.
func Register(r *command.Registry, keys secrets.Store, opts ...Option) {
	s := &service{reg: r, keys: keys, convs: map[string]*conversation{}, newProvider: openAIProvider}
	for _, o := range opts {
		o(s)
	}
	r.Add(
		command.Define("assistant.config.get", "Show the AI assistant's provider, address and model, and whether a key is saved. Uses no network.", false, s.configGet),
		command.Define("assistant.config.set", "Choose the AI assistant's provider, address or model, save its key in the system keychain, "+
			"or confirm what is sent to it. Uses no network.", true, s.configSet),
		command.Define("assistant.models", "List the models the chosen provider offers.", false, s.models),
		command.Define("assistant.test", "Check that the chosen provider answers and accepts the key.", false, s.test),
		command.Define("assistant.preview", "Show exactly what the first message of a chat would send, and where, without sending it.", false, s.preview),
		command.Define("assistant.chat", "Ask the AI assistant. It reads the library with read-only tools through the licence gate and streams "+
			"its reply as assistant:event events; it never changes the library.", false, s.chat),
		command.Define("assistant.cancel", "Stop the reply a chat is writing.", false, s.cancel),
		command.Define("assistant.conversation", "Show a chat of this session: its questions, replies and tool calls.", false, s.conversation),
		command.Define("assistant.save", "Keep a chat of this session as a note.", true, s.save),
	)
}

func openAIProvider(baseURL, key string, p Preset) Provider {
	o := &OpenAI{BaseURL: baseURL, Key: key}
	if p.ID == "claude" && key != "" {
		// Anthropic's own model list wants these headers; its OpenAI-compatible chat accepts the key either way.
		o.Header = map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"}
	}
	return o
}

func needStore(env *command.Env) error {
	if env.Store == nil {
		return command.Errorf("unavailable", "no library is open")
	}
	return nil
}

func load(ctx context.Context, env *command.Env) (config, error) {
	c := config{Provider: "off"}
	if err := needStore(env); err != nil {
		return c, err
	}
	var raw string
	err := env.Store.QueryRow(ctx, `SELECT value_json FROM settings WHERE key = ?`, settingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, fmt.Errorf("settings %s: %w", settingsKey, err)
	}
	if _, ok := preset(c.Provider); !ok {
		c.Provider = "off"
	}
	return c, nil
}

func saveConfig(ctx context.Context, env *command.Env, c config) error {
	raw, _ := json.Marshal(c)
	return env.Store.Write(ctx, "assistant.config.set", func(tx *store.Tx) error {
		_, err := tx.Get("settings", store.Row{"key": settingsKey})
		if errors.Is(err, store.ErrNotFound) {
			return tx.Create("settings", store.Row{"key": settingsKey, "value_json": string(raw)})
		} else if err != nil {
			return err
		}
		return tx.Update("settings", store.Row{"key": settingsKey}, store.Row{"value_json": string(raw)})
	})
}

func (c config) baseURL(p Preset) string {
	if u := c.BaseURLs[p.ID]; u != "" {
		return u
	}
	return p.BaseURL
}

// onThisComputer reports whether an address is this computer.
func onThisComputer(addr string) bool {
	u, err := url.Parse(addr)
	if err != nil {
		return false
	}
	h := u.Hostname()
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// local reports whether personal copies may go to this provider: the Local model or a custom
// address, and only while the address is this computer.
func local(p Preset, addr string) bool {
	return (p.Local || p.ID == "custom") && onThisComputer(addr)
}

func confirmKey(provider, addr string) string { return provider + "|" + addr }

func (s *service) view(c config) Config {
	out := Config{Provider: c.Provider, Presets: Presets}
	p, ok := preset(c.Provider)
	if !ok {
		out.Provider = "off"
		return out
	}
	out.Label, out.BaseURL, out.Model, out.NeedsKey = p.Label, c.baseURL(p), c.Models[p.ID], p.NeedsKey
	out.Local = local(p, out.BaseURL)
	_, err := s.keys.Get(keyName(p.ID))
	out.KeySaved = err == nil
	out.Confirmed = slices.Contains(c.Confirmed, confirmKey(p.ID, out.BaseURL))
	return out
}

func (s *service) configGet(ctx context.Context, env *command.Env, _ struct{}) (Config, error) {
	c, err := load(ctx, env)
	if err != nil {
		return Config{}, err
	}
	return s.view(c), nil
}

type SetParams struct {
	Provider *string `json:"provider,omitempty" desc:"off, claude, openai, gemini, openrouter, local or custom"`
	BaseURL  *string `json:"base_url,omitempty" desc:"The chosen provider's address; empty for its usual one"`
	Model    *string `json:"model,omitempty" desc:"The model to use with the chosen provider"`
	Key      *string `json:"key,omitempty" desc:"The chosen provider's key, stored in the system keychain and never shown again"`
	ClearKey bool    `json:"clear_key,omitempty" desc:"Remove the chosen provider's key from the keychain"`
	Confirm  bool    `json:"confirm,omitempty" desc:"The person has seen what is sent to the chosen provider and agrees"`
}

func (s *service) configSet(ctx context.Context, env *command.Env, p SetParams) (Config, error) {
	c, err := load(ctx, env)
	if err != nil {
		return Config{}, err
	}
	if p.Provider != nil {
		if _, ok := preset(*p.Provider); !ok && *p.Provider != "off" {
			return Config{}, command.Errorf("bad_params", "unknown provider %q", *p.Provider)
		}
		c.Provider = *p.Provider
	}
	pr, ok := preset(c.Provider)
	if !ok && (p.BaseURL != nil || p.Model != nil || p.Key != nil || p.ClearKey || p.Confirm) {
		return Config{}, command.Errorf("bad_params", "choose a provider first")
	}
	if p.BaseURL != nil {
		u := strings.TrimSpace(*p.BaseURL)
		if u != "" {
			if pu, err := url.Parse(u); err != nil || (pu.Scheme != "http" && pu.Scheme != "https") || pu.Host == "" {
				return Config{}, command.Errorf("bad_params", "the address must start with http:// or https://")
			}
		}
		if c.BaseURLs == nil {
			c.BaseURLs = map[string]string{}
		}
		if u == "" {
			delete(c.BaseURLs, pr.ID)
		} else {
			c.BaseURLs[pr.ID] = u
		}
	}
	if p.Model != nil {
		if c.Models == nil {
			c.Models = map[string]string{}
		}
		c.Models[pr.ID] = strings.TrimSpace(*p.Model)
	}
	if p.Confirm {
		k := confirmKey(pr.ID, c.baseURL(pr))
		if !slices.Contains(c.Confirmed, k) {
			c.Confirmed = append(c.Confirmed, k)
		}
	}
	// The key goes to the keychain only; library.db never sees it.
	if p.Key != nil && strings.TrimSpace(*p.Key) != "" {
		if err := s.keys.Set(keyName(pr.ID), strings.TrimSpace(*p.Key)); err != nil {
			return Config{}, command.Errorf("failed", "could not save the key in the system keychain: %v", err)
		}
	}
	if p.ClearKey {
		if err := s.keys.Delete(keyName(pr.ID)); err != nil {
			return Config{}, command.Errorf("failed", "could not remove the key from the system keychain: %v", err)
		}
	}
	if err := saveConfig(ctx, env, c); err != nil {
		return Config{}, err
	}
	return s.view(c), nil
}

// current is the chosen provider, ready to use.
type current struct {
	preset Preset
	addr   string
	model  string
	local  bool
	conf   Config
	p      Provider
}

func (s *service) current(ctx context.Context, env *command.Env) (current, error) {
	c, err := load(ctx, env)
	if err != nil {
		return current{}, err
	}
	pr, ok := preset(c.Provider)
	if !ok {
		return current{}, command.Errorf("unavailable", "The assistant is off. Choose a provider in Settings → Assistant → Model.")
	}
	v := s.view(c)
	if v.BaseURL == "" {
		return current{}, command.Errorf("unavailable", "Give the address of the %s provider in Settings → Assistant → Model.", pr.Label)
	}
	key, err := s.keys.Get(keyName(pr.ID))
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return current{}, command.Errorf("failed", "could not read the key from the system keychain: %v", err)
	}
	if key == "" && pr.NeedsKey {
		return current{}, command.Errorf("unavailable", "Save your %s key in Settings → Assistant → Model first.", pr.Label)
	}
	return current{preset: pr, addr: v.BaseURL, model: v.Model, local: v.Local, conf: v, p: s.newProvider(v.BaseURL, key, pr)}, nil
}

func plainErr(err error) error {
	var pe *Error
	if errors.As(err, &pe) {
		return command.Errorf("unavailable", "%s", pe.Message)
	}
	return err
}

type ModelsParams struct{}

func (s *service) models(ctx context.Context, env *command.Env, _ ModelsParams) ([]string, error) {
	cur, err := s.current(ctx, env)
	if err != nil {
		return nil, err
	}
	ms, err := cur.p.Models(ctx)
	return ms, plainErr(err)
}

type TestResult struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Models  []string `json:"models,omitempty"`
}

func (s *service) test(ctx context.Context, env *command.Env, _ struct{}) (TestResult, error) {
	cur, err := s.current(ctx, env)
	if err != nil {
		var ce *command.Error
		if errors.As(err, &ce) && ce.Code == "unavailable" {
			return TestResult{Message: ce.Message}, nil
		}
		return TestResult{}, err
	}
	ctx, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	ms, err := cur.p.Models(ctx)
	if err == nil && len(ms) == 0 && cur.model != "" {
		// No model list: a one-word request instead. It holds nothing of the person's.
		_, err = cur.p.Stream(ctx, Request{Model: cur.model, Messages: []Message{{Role: "user", Content: "Reply with the word OK."}}}, nil)
	}
	if err != nil {
		var pe *Error
		if errors.As(err, &pe) {
			return TestResult{Message: pe.Message}, nil
		}
		return TestResult{Message: "The test did not finish: " + err.Error()}, nil
	}
	msg := fmt.Sprintf("Connected to %s.", cur.preset.Label)
	if len(ms) > 0 {
		msg += fmt.Sprintf(" %d models are available.", len(ms))
	}
	return TestResult{OK: true, Message: msg, Models: ms}, nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "c_" + hex.EncodeToString(b)
}
