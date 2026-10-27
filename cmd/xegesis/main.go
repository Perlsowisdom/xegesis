// Command xegesis runs Xegesis commands from the terminal and over a JSON channel.
//
//	xegesis commands             list every command as JSON
//	xegesis describe <id>        describe one command
//	xegesis run <id> [params]    run a command; params is JSON, or - to read it from stdin
//	xegesis serve                read one JSON request per line on stdin, write one response per line
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

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "xegesis:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("xegesis", flag.ContinueOnError)
	lib := fs.String("library", defaultLibrary(), "path to library.db")
	if err := fs.Parse(args); err != nil {
		return err
	}
	args = fs.Args()
	if len(args) == 0 {
		return errors.New("usage: xegesis [-library path] commands | describe <id> | run <id> [params] | serve")
	}
	reg := command.NewRegistry()
	core.Register(reg)
	out := json.NewEncoder(stdout)
	out.SetIndent("", "  ")

	switch args[0] {
	case "commands":
		return out.Encode(reg.List())
	case "describe":
		if len(args) != 2 {
			return errors.New("usage: xegesis describe <id>")
		}
		d, ok := reg.Describe(args[1])
		if !ok {
			return fmt.Errorf("no command %q", args[1])
		}
		return out.Encode(d)
	case "run":
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: xegesis run <id> [params]")
		}
		env, closeEnv, err := openEnv(*lib)
		if err != nil {
			return err
		}
		defer closeEnv()
		params := "{}"
		if len(args) == 3 {
			params = args[2]
		}
		if params == "-" {
			b, err := io.ReadAll(stdin)
			if err != nil {
				return err
			}
			params = string(b)
		}
		res, err := reg.Run(context.Background(), env, args[1], json.RawMessage(params))
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
	}
	return fmt.Errorf("unknown subcommand %q", args[0])
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
