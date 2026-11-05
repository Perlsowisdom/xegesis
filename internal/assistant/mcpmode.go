package assistant

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/mcp"
)

// MCPTools are Tools as MCP tools, each running through gate. They are the only tools the MCP
// assistant mode offers: none writes, and the registry's other commands are not reachable.
func MCPTools(d *Deps) []mcp.Custom {
	ts := Tools()
	out := make([]mcp.Custom, len(ts))
	for i, t := range ts {
		name := t.Def.Name
		out[i] = mcp.Custom{
			Name: name, Description: t.Def.Description, InputSchema: t.Def.Parameters,
			Call: func(ctx context.Context, args json.RawMessage) (string, bool) {
				r := RunTool(ctx, d, name, string(args))
				return r.Content, strings.HasPrefix(r.Content, "Error: ")
			},
		}
	}
	return out
}

// ServeMCP serves the read-only assistant tools over MCP on in and out ("xegesis mcp-assistant").
// The tools are for a command-line tool that sends what it reads to an online model, so the gate
// treats the reader as online: personal copies of books are withheld and quoting limits apply.
// XEGESIS_ASSISTANT_BIBLE names the Bible open in the Reader, if any.
func ServeMCP(ctx context.Context, reg *command.Registry, env *command.Env, version string, in io.Reader, out io.Writer) error {
	d := &Deps{Run: RegistryRunner(reg, env), Gate: Gate{Local: false}, Bible: os.Getenv("XEGESIS_ASSISTANT_BIBLE")}
	s := &mcp.Server{Name: "xegesis-assistant", Version: version, Only: MCPTools(d)}
	return s.Serve(ctx, in, out)
}
