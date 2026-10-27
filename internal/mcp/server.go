// Package mcp serves a command registry over the Model Context Protocol (JSON-RPC 2.0 on stdio,
// one JSON message per line). Each registered command becomes one tool; the server adds no features
// of its own.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// Supported protocol versions, newest first. The server echoes the client's version when it is
// listed and otherwise offers the newest.
var versions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// Server exposes every command in Reg as a tool.
type Server struct {
	Reg     *command.Registry
	Env     *command.Env
	Name    string
	Version string

	tools  []tool
	byTool map[string]string // tool name to command ID
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations annotations    `json:"annotations"`
}

type annotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
}

var badName = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// ToolName turns a command ID such as "note.create" into a tool name such as "note_create".
func ToolName(id string) string {
	name := badName.ReplaceAllString(strings.ReplaceAll(id, ".", "_"), "_")
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

func (s *Server) build() {
	s.tools, s.byTool = nil, map[string]string{}
	for _, d := range s.Reg.List() {
		name := ToolName(d.ID)
		if _, dup := s.byTool[name]; dup {
			continue
		}
		s.byTool[name] = d.ID
		desc := d.Summary
		if d.Writes {
			desc += " Changes the library; recorded in history."
		}
		s.tools = append(s.tools, tool{
			Name: name, Description: desc, InputSchema: inputSchema(d.Params),
			Annotations: annotations{
				Title:           d.ID,
				ReadOnlyHint:    !d.Writes,
				DestructiveHint: strings.HasSuffix(d.ID, ".delete") || strings.HasSuffix(d.ID, ".remove"),
			},
		})
	}
}

func inputSchema(p *command.Schema) map[string]any {
	m := schemaMap(p)
	m["type"] = "object"
	if _, ok := m["properties"]; !ok {
		m["properties"] = map[string]any{}
	}
	return m
}

func schemaMap(s *command.Schema) map[string]any {
	m := map[string]any{}
	if s == nil {
		return m
	}
	if s.Type != "" && s.Type != "any" {
		m["type"] = s.Type
	}
	if s.Description != "" {
		m["description"] = s.Description
	}
	if len(s.Properties) > 0 {
		props := map[string]any{}
		for k, v := range s.Properties {
			props[k] = schemaMap(v)
		}
		m["properties"] = props
		m["additionalProperties"] = false
	}
	if len(s.Required) > 0 {
		m["required"] = s.Required
	}
	if s.Items != nil {
		m["items"] = schemaMap(s.Items)
	}
	if len(s.Enum) > 0 {
		m["enum"] = s.Enum
	}
	return m
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests from in and writes responses to out until in ends or ctx is done.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.build()
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		if resp := s.handle(ctx, line); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// handle returns nil when no response is due.
func (s *Server) handle(ctx context.Context, line []byte) *response {
	null := json.RawMessage("null")
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		if _, isType := err.(*json.UnmarshalTypeError); isType {
			return fail(null, codeInvalidRequest, "invalid request")
		}
		return fail(null, codeParse, "parse error: "+err.Error())
	}
	if req.Method == "" {
		if req.ID == nil { // junk
			return fail(null, codeInvalidRequest, "invalid request")
		}
		return nil // a response to a request we never sent
	}
	if req.ID == nil { // notification
		return nil
	}
	if req.JSONRPC != "2.0" {
		return fail(req.ID, codeInvalidRequest, `jsonrpc must be "2.0"`)
	}
	result, rerr := s.dispatch(ctx, req)
	if rerr != nil {
		return &response{JSONRPC: "2.0", ID: req.ID, Error: rerr}
	}
	return &response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func fail(id json.RawMessage, code int, msg string) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.initialize(req.Params)
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.tools}, nil
	case "tools/call":
		return s.call(ctx, req.Params)
	}
	return nil, &rpcError{codeMethodNotFound, "method not found: " + req.Method}
}

func (s *Server) initialize(raw json.RawMessage) (any, *rpcError) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, "invalid initialize params"}
	}
	version := versions[0]
	for _, v := range versions {
		if v == p.ProtocolVersion {
			version = v
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
		"instructions":    "Xegesis Bible study tools. Each tool is one Xegesis command.",
	}, nil
}

func (s *Server) call(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Name == "" {
		return nil, &rpcError{codeInvalidParams, "tools/call needs a tool name"}
	}
	id, ok := s.byTool[p.Name]
	if !ok {
		return nil, &rpcError{codeInvalidParams, "unknown tool: " + p.Name}
	}
	res, err := s.Reg.Run(ctx, s.Env, id, p.Arguments)
	if err != nil {
		msg := err.Error()
		if ce, ok := err.(*command.Error); ok {
			msg = ce.Code + ": " + ce.Message
		}
		return toolResult(msg, nil, true), nil
	}
	b, err := json.Marshal(res)
	if err != nil {
		return nil, &rpcError{codeInternal, err.Error()}
	}
	var structured any
	if len(b) > 0 && b[0] == '{' {
		structured = json.RawMessage(b)
	}
	return toolResult(string(b), structured, false), nil
}

func toolResult(text string, structured any, isError bool) map[string]any {
	m := map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
		"isError": isError,
	}
	if structured != nil {
		m["structuredContent"] = structured
	}
	return m
}
