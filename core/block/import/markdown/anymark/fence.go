package anymark

import (
	"strings"
	"unicode"
)

// minFenceLen is the shortest run of backticks or tildes that opens a fenced
// code block, per CommonMark.
const minFenceLen = 3

// trimMarkdownOutsideFences is a fence-aware replacement for the default after
// hook of html-to-markdown.
//
// That hook (from.go, NewConverter) right-trims every line of the finished
// markdown and collapses runs of blank lines, with no awareness of code fences.
// Applied to fenced content that is significant whitespace, it silently rewrites
// the code: a copied code block ending in trailing spaces came back with them
// gone, and blank lines inside a code block were collapsed (GO-7515).
//
// Outside fences the behaviour is deliberately identical to the hook it
// replaces: the document is trimmed, each line is right-trimmed, and runs of
// blank lines collapse to one. Inside a fence every line is preserved verbatim.
func trimMarkdownOutsideFences(markdown string) string {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return markdown
	}

	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))

	var (
		inFence      bool
		fenceChar    byte
		fenceLen     int
		pendingBlank bool
	)
	for _, line := range lines {
		if inFence {
			// Code content: verbatim, including trailing spaces and blank lines.
			out = append(out, line)
			if isFenceClose(line, fenceChar, fenceLen) {
				inFence = false
			}
			continue
		}

		trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
		if trimmed == "" {
			pendingBlank = true
			continue
		}
		if pendingBlank && len(out) > 0 {
			out = append(out, "")
		}
		pendingBlank = false
		out = append(out, trimmed)

		if char, length, ok := parseFenceOpen(trimmed); ok {
			inFence, fenceChar, fenceLen = true, char, length
		}
	}

	return strings.Join(out, "\n")
}

// parseFenceOpen reports whether line opens a fenced code block, and with which
// delimiter. Per CommonMark the opener is indented at most three spaces and is a
// run of at least three backticks or tildes; the info string of a BACKTICK fence
// may not contain a backtick, which is what stops an inline code span from being
// mistaken for a fence.
func parseFenceOpen(line string) (char byte, length int, ok bool) {
	rest, ok := stripFenceIndent(line)
	if !ok {
		return 0, 0, false
	}
	char = rest[0]
	if char != '`' && char != '~' {
		return 0, 0, false
	}
	for length < len(rest) && rest[length] == char {
		length++
	}
	if length < minFenceLen {
		return 0, 0, false
	}
	if char == '`' && strings.ContainsRune(rest[length:], '`') {
		return 0, 0, false
	}
	return char, length, true
}

// isFenceClose reports whether line closes a fence opened with fenceLen
// occurrences of fenceChar: at least as many of the same character, followed by
// nothing but whitespace.
func isFenceClose(line string, fenceChar byte, fenceLen int) bool {
	rest, ok := stripFenceIndent(line)
	if !ok || rest[0] != fenceChar {
		return false
	}
	n := 0
	for n < len(rest) && rest[n] == fenceChar {
		n++
	}
	return n >= fenceLen && strings.TrimSpace(rest[n:]) == ""
}

// stripFenceIndent removes the up-to-three leading spaces a fence delimiter may
// carry. A deeper indent makes the line indented code rather than a fence.
func stripFenceIndent(line string) (rest string, ok bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' && indent < minFenceLen+1 {
		indent++
	}
	if indent > minFenceLen {
		return "", false
	}
	rest = line[indent:]
	return rest, rest != ""
}
