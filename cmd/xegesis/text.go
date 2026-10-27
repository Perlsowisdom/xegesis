package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// wantText reports whether help and describe should print text: with -format text, or with
// -format auto when stdout is a terminal.
func wantText(format string, stdout io.Writer) (bool, error) {
	switch format {
	case "text":
		return true, nil
	case "json":
		return false, nil
	case "auto":
		f, ok := stdout.(*os.File)
		if !ok {
			return false, nil
		}
		st, err := f.Stat()
		return err == nil && st.Mode()&os.ModeCharDevice != 0, nil
	}
	return false, fmt.Errorf("unknown -format %q (use auto, text or json)", format)
}

func writeHelp(w io.Writer, list []command.Description) error {
	fmt.Fprintln(w, "Xegesis commands (xegesis describe <id> for details):")
	width := 0
	for _, d := range list {
		width = max(width, len(d.ID))
	}
	for _, d := range list {
		fmt.Fprintf(w, "  %-*s  %s\n", width, d.ID, d.Summary)
	}
	_, err := fmt.Fprintln(w, "\nRun one with: xegesis run <id> key=value ...")
	return err
}

func writeDescription(w io.Writer, d command.Description) error {
	fmt.Fprintf(w, "%s\n  %s\n", d.ID, d.Summary)
	if d.Writes {
		fmt.Fprintln(w, "  Changes the library; recorded in history.")
	}
	fmt.Fprintln(w, "\nParameters:")
	if d.Params == nil || len(d.Params.Properties) == 0 {
		fmt.Fprintln(w, "  none")
	} else {
		writeProps(w, d.Params, "  ")
	}
	if d.Result != nil {
		fmt.Fprintf(w, "\nResult: %s\n", typeName(d.Result))
		sub := d.Result
		if sub.Items != nil {
			sub = sub.Items
		}
		if len(sub.Properties) > 0 {
			writeProps(w, sub, "  ")
		}
	}
	return nil
}

func writeProps(w io.Writer, s *command.Schema, indent string) {
	names := make([]string, 0, len(s.Properties))
	for n := range s.Properties {
		names = append(names, n)
	}
	sort.Strings(names)
	required := map[string]bool{}
	for _, n := range s.Required {
		required[n] = true
	}
	for _, n := range names {
		p := s.Properties[n]
		line := fmt.Sprintf("%s%s  %s", indent, n, typeName(p))
		if required[n] {
			line += "  (required)"
		}
		fmt.Fprintln(w, line)
		if p.Description != "" {
			fmt.Fprintf(w, "%s    %s\n", indent, p.Description)
		}
		if len(p.Enum) > 0 {
			fmt.Fprintf(w, "%s    one of: %s\n", indent, strings.Join(p.Enum, ", "))
		}
		sub := p
		if p.Items != nil {
			sub = p.Items
		}
		if len(sub.Properties) > 0 {
			writeProps(w, sub, indent+"    ")
		}
	}
}

func typeName(s *command.Schema) string {
	if s.Type == "array" && s.Items != nil {
		return "array of " + typeName(s.Items)
	}
	return s.Type
}
