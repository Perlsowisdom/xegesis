// Command xegesis runs Xegesis commands from the terminal, over a JSON channel and as an MCP server.
//
//	xegesis commands             list every command as JSON
//	xegesis help [id]            list commands, or describe one: text on a terminal, JSON otherwise
//	xegesis describe <id>        describe one command
//	xegesis run <id> [params]    run a command; params is JSON, - to read JSON from stdin, or key=value pairs
//	xegesis serve                read one JSON request per line on stdin, write one response per line
//	xegesis mcp                  serve the commands to an MCP client over stdio
//	xegesis mcp-assistant        serve only the AI assistant's read-only tools over stdio
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/assistant"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/all"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/mcp"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

const usage = "usage: xegesis [-library path] [-format auto|text|json] commands | help [id] | describe <id> | run <id> [params | key=value ...] | serve | mcp | mcp-assistant"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "xegesis:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("xegesis", flag.ContinueOnError)
	lib := fs.String("library", defaultLibrary(), "path to library.db")
	format := fs.String("format", "auto", "output of help and describe: auto, text or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	args = fs.Args()
	if len(args) == 0 {
		return errors.New(usage)
	}
	text, err := wantText(*format, stdout)
	if err != nil {
		return err
	}
	reg := all.Registry(filepath.Dir(*lib), secrets.OS())
	out := json.NewEncoder(stdout)
	out.SetIndent("", "  ")

	switch args[0] {
	case "commands":
		return out.Encode(reg.List())
	case "help":
		if len(args) == 2 {
			return describe(reg, args[1], text, out, stdout)
		}
		if text {
			return writeHelp(stdout, reg.List())
		}
		return out.Encode(reg.List())
	case "describe":
		if len(args) != 2 {
			return errors.New("usage: xegesis describe <id>")
		}
		return describe(reg, args[1], text, out, stdout)
	case "run":
		if len(args) < 2 {
			return errors.New("usage: xegesis run <id> [params | key=value ...]")
		}
		params, err := runParams(args[2:], stdin)
		if err != nil {
			return err
		}
		env, closeEnv, err := openEnv(*lib)
		if err != nil {
			return err
		}
		defer closeEnv()
		res, err := reg.Run(context.Background(), env, args[1], params)
		if err != nil {
			return err
		}
		return out.Encode(res)
	case "serve":
		env, closeEnv, err := openEnv(*lib)
		if err != nil {
			return err
		}
		defer closeEnv()
		return serve(reg, env, stdin, stdout)
	case "mcp":
		env, closeEnv, err := openEnv(*lib)
		if err != nil {
			return err
		}
		defer closeEnv()
		s := &mcp.Server{Reg: reg, Env: env, Name: "xegesis", Version: core.Version}
		return s.Serve(context.Background(), stdin, stdout)
	case "mcp-assistant":
		// Only the assistant's read-only tools, through the licence gate (docs/mcp.md).
		env, closeEnv, err := openEnv(*lib)
		if err != nil {
			return err
		}
		defer closeEnv()
		return assistant.ServeMCP(context.Background(), reg, env, core.Version, stdin, stdout)
	}
	return fmt.Errorf("unknown subcommand %q", args[0])
}

func describe(reg *command.Registry, id string, text bool, out *json.Encoder, w io.Writer) error {
	d, ok := reg.Describe(id)
	if !ok {
		return fmt.Errorf("no command %q", id)
	}
	if text {
		return writeDescription(w, d)
	}
	return out.Encode(d)
}

// runParams reads parameters from one JSON argument, - for stdin, or key=value arguments.
func runParams(args []string, stdin io.Reader) (json.RawMessage, error) {
	if len(args) == 0 {
		return json.RawMessage("{}"), nil
	}
	if len(args) == 1 {
		switch {
		case args[0] == "-":
			b, err := io.ReadAll(stdin)
			return json.RawMessage(b), err
		case strings.HasPrefix(args[0], "{") || !strings.Contains(args[0], "="):
			return json.RawMessage(args[0]), nil
		}
	}
	return keyValueParams(args)
}

// keyValueParams builds a JSON object from key=value pairs. A value is JSON when it parses as
// JSON and a string otherwise; a dotted key such as a.b=1 makes a nested object.
func keyValueParams(args []string) (json.RawMessage, error) {
	root := map[string]any{}
	for _, a := range args {
		key, val, ok := strings.Cut(a, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("parameter %q is not key=value", a)
		}
		var v any = val
		if json.Valid([]byte(val)) {
			v = json.RawMessage(val)
		}
		parts := strings.Split(key, ".")
		m := root
		for _, p := range parts[:len(parts)-1] {
			next, ok := m[p].(map[string]any)
			if !ok {
				next = map[string]any{}
				m[p] = next
			}
			m = next
		}
		m[parts[len(parts)-1]] = v
	}
	return json.Marshal(root)
}

// Request and Response are the JSON channel's messages, one per line.
type Request struct {
	ID      any             `json:"id,omitempty"`
	Command string          `json:"command"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	ID     any            `json:"id,omitempty"`
	Result any            `json:"result,omitempty"`
	Error  *command.Error `json:"error,omitempty"`
}

func serve(reg *command.Registry, env *command.Env, stdin io.Reader, stdout io.Writer) error {
	in := bufio.NewScanner(stdin)
	in.Buffer(make([]byte, 1<<20), 64<<20)
	out := json.NewEncoder(stdout)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		var resp Response
		if err := json.Unmarshal(line, &req); err != nil {
			resp.Error = command.Errorf("bad_request", "%v", err)
		} else {
			resp.ID = req.ID
			var err error
			if req.Command == "commands" {
				resp.Result = reg.List()
			} else if resp.Result, err = reg.Run(context.Background(), env, req.Command, req.Params); err != nil {
				errors.As(err, &resp.Error)
			}
		}
		if err := out.Encode(resp); err != nil {
			return err
		}
	}
	return in.Err()
}

func openEnv(path string) (*command.Env, func(), error) {
	s, err := store.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return &command.Env{Store: s}, func() { s.Close() }, nil
}

func defaultLibrary() string {
	if p := os.Getenv("XEGESIS_LIBRARY"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "Xegesis", "library.db")
}
