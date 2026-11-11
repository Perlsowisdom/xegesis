// Package command is the single way to use Xegesis. Every feature is a registered command with typed
// parameters and results; the desktop app, the CLI, the JSON channel and the MCP server all call
// commands through a Registry, and each command describes itself so people and agents can find it.
package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Env is what commands work with.
type Env struct {
	Store *store.Store
	// Books maps a place in a library book to Bible verses. Registry.Run fills it in from the
	// linker registered with SetBookLinker, so callers need not set it.
	Books BookLinker
}

// BookLinker tells which verses a place in a library book typed Bible falls in (locatorJSON is a
// note's or highlight's book place), so a note or highlight there shows with those verses.
type BookLinker interface {
	BookVerses(ctx context.Context, env *Env, itemID, locatorJSON string) (verse.Range, bool)
}

// Error is a command failure with a stable code that callers can act on.
type Error struct {
	Code    string `json:"code"` // bad_params, not_found, unknown_command, unavailable, failed
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf makes an Error.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Command is one registered action.
type Command struct {
	ID      string
	Summary string
	Writes  bool // changes library.db, so it is logged in the change history
	params  any
	result  any
	run     func(context.Context, *Env, json.RawMessage) (any, error)
}

// Define makes a command from a typed function. P must be a struct (use struct{} for none); its
// fields' json tags name the parameters and desc tags describe them.
func Define[P, R any](id, summary string, writes bool, fn func(context.Context, *Env, P) (R, error)) Command {
	var p P
	var r R
	return Command{
		ID: id, Summary: summary, Writes: writes, params: p, result: r,
		run: func(ctx context.Context, env *Env, raw json.RawMessage) (any, error) {
			var params P
			if len(bytes.TrimSpace(raw)) > 0 && string(bytes.TrimSpace(raw)) != "null" {
				d := json.NewDecoder(bytes.NewReader(raw))
				d.DisallowUnknownFields()
				if err := d.Decode(&params); err != nil {
					return nil, Errorf("bad_params", "%s: %v", id, err)
				}
			}
			if err := checkRequired(params); err != nil {
				return nil, Errorf("bad_params", "%s: %v", id, err)
			}
			if writes && env.Store == nil {
				return nil, Errorf("unavailable", "%s needs an open library", id)
			}
			return fn(ctx, env, params)
		},
	}
}

// Registry holds every command.
type Registry struct {
	cmds  map[string]Command
	books BookLinker
}

// SetBookLinker registers what maps places in books to verses (the reader does this at start-up).
func (r *Registry) SetBookLinker(l BookLinker) { r.books = l }

func NewRegistry() *Registry { return &Registry{cmds: map[string]Command{}} }

// Add registers commands. IDs are dotted lowercase names such as "note.create".
func (r *Registry) Add(cmds ...Command) {
	for _, c := range cmds {
		if c.ID == "" || strings.ToLower(c.ID) != c.ID || strings.ContainsAny(c.ID, " /") {
			panic(fmt.Sprintf("command: bad ID %q", c.ID))
		}
		if _, dup := r.cmds[c.ID]; dup {
			panic("command: duplicate ID " + c.ID)
		}
		r.cmds[c.ID] = c
	}
}

// Run executes a command with JSON parameters.
func (r *Registry) Run(ctx context.Context, env *Env, id string, params json.RawMessage) (any, error) {
	c, ok := r.cmds[id]
	if !ok {
		return nil, Errorf("unknown_command", "no command %q", id)
	}
	if env != nil && env.Books == nil && r.books != nil {
		e := *env
		e.Books = r.books
		env = &e
	}
	res, err := c.run(ctx, env, params)
	if err != nil {
		var ce *Error
		if errors.As(err, &ce) {
			return nil, ce
		}
		if errors.Is(err, store.ErrNotFound) {
			return nil, Errorf("not_found", "%v", err)
		}
		return nil, Errorf("failed", "%s: %v", id, err)
	}
	return res, nil
}

// Description is a command's self-description, as shown to people and agents.
type Description struct {
	ID      string  `json:"id"`
	Summary string  `json:"summary"`
	Writes  bool    `json:"writes"`
	Params  *Schema `json:"params"`
	Result  *Schema `json:"result,omitempty"`
}

// Describe returns one command's description.
func (r *Registry) Describe(id string) (Description, bool) {
	c, ok := r.cmds[id]
	if !ok {
		return Description{}, false
	}
	return Description{ID: c.ID, Summary: c.Summary, Writes: c.Writes, Params: schemaOf(c.params), Result: schemaOf(c.result)}, true
}

// List describes every command, sorted by ID.
func (r *Registry) List() []Description {
	ids := make([]string, 0, len(r.cmds))
	for id := range r.cmds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Description, len(ids))
	for i, id := range ids {
		out[i], _ = r.Describe(id)
	}
	return out
}
