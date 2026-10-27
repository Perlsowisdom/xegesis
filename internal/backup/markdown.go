package backup

import "strings"

// Document bodies use any's canonical Markdown (docs.any.org/database/markdown-import-export):
// blocks are separated by one blank line, every further blank line is an empty paragraph, at the
// edges of the text every blank line is an empty paragraph, and one trailing newline is a
// terminator. A block here is a run of non-blank lines; Xegesis keeps block structure inside the
// text, so ParseBlocks and RenderBlocks only carry the blank-line rules.

// EncodeBody turns a stored body into the contents of an exported .md file: the body followed by
// the terminating newline. DecodeBody is its exact inverse for every body.
func EncodeBody(body string) string {
	if body == "" {
		return ""
	}
	return body + "\n"
}

// DecodeBody removes the terminator that EncodeBody added.
func DecodeBody(file string) string {
	return strings.TrimSuffix(file, "\n")
}

// ParseBlocks reads canonical Markdown (without its terminator) into blocks. An empty paragraph is
// the empty string.
func ParseBlocks(md string) []string {
	if md == "" {
		return nil
	}
	pieces := strings.Split(md, "\n")
	var blocks []string
	var run []string
	blanks := 0
	flushBlanks := func(edge bool) {
		n := blanks
		if !edge {
			n--
		}
		for i := 0; i < n; i++ {
			blocks = append(blocks, "")
		}
		blanks = 0
	}
	started := false
	for _, p := range pieces {
		if p == "" {
			if len(run) > 0 {
				blocks = append(blocks, strings.Join(run, "\n"))
				run = nil
			}
			blanks++
			continue
		}
		if blanks > 0 {
			flushBlanks(!started)
		}
		started = true
		run = append(run, p)
	}
	if len(run) > 0 {
		blocks = append(blocks, strings.Join(run, "\n"))
	}
	if blanks > 0 {
		flushBlanks(true)
	}
	return blocks
}

// RenderBlocks is the inverse of ParseBlocks.
func RenderBlocks(blocks []string) string {
	var sb strings.Builder
	empties, seen := 0, false
	for _, b := range blocks {
		if b == "" {
			empties++
			continue
		}
		if !seen {
			sb.WriteString(strings.Repeat("\n", empties))
		} else {
			sb.WriteString(strings.Repeat("\n", empties+2))
		}
		sb.WriteString(b)
		seen, empties = true, 0
	}
	if seen {
		sb.WriteString(strings.Repeat("\n", empties))
	} else if empties > 1 {
		sb.WriteString(strings.Repeat("\n", empties-1))
	}
	return sb.String()
}
