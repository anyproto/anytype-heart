package anyblockjson

// markdownblocks.go — the markdown→flat-blocks parser (the
// Phase-5 critical-path build item): block-level markdown slicing into the §4
// flat run. Inline content is NOT parsed here — a flat block's `text` is
// already §8 inline markup source, and the fragment import (UnmarshalBlocks)
// parses it with the same inline grammar reads render with. So this file owns
// exactly what §3's build-item note scopes: headings, lists and their
// indentation, fences, quotes, dividers and tables; everything inline rides
// the existing codec, keeping authoring and reading on ONE dialect.
//
// Markdown always parses: there is no error path. Unrecognized block-level
// constructs degrade to paragraphs, an unterminated fence runs to the end of
// the input, a malformed table degrades to paragraph lines, and over-deep
// indentation is clamped to the previous block's level + 1 (CommonMark's "a
// level that hasn't been established cannot be opened", the same rule as
// Options.NormalizeIndent). The clamp also respects the two §12 containment
// rules the +1 rule alone would break: a §5 leaf block (divider, table)
// cannot parent, so a deeper line after one stays its sibling; and the F4
// absolute nesting bound (indent ≤ 32) caps every level. So the produced run
// always satisfies the §4 strict indent rules, the V2 leaf-containment rule
// and the F4 depth bound — the "a run always imports" contract is tested
// through UnmarshalBlocks over every block type this parser can emit.
//
// Deliberate scope bounds (deterministic > clever, recorded for SKILL/docs):
//   - ATX headings only (`#`…); `---` after a paragraph is a divider, never a
//     setext underline. Levels 4–6 clamp to heading_3 (§5's own alias rule).
//   - One quote level: `>` prefixes strip one level; consecutive quote lines
//     join into one quote block. No lazy continuation — a plain line after a
//     quote starts a new paragraph.
//   - List indentation: each 2 leading spaces (or one tab) = one level,
//     then clamped. A more-indented plain line under a list item becomes a
//     child paragraph of that item.
//   - Tables need the `|`-leading header row + `---` separator row; cells are
//     inline markup source, `\|` escapes a literal pipe. Ragged rows widen
//     the column set (never a validation error).
//   - No images-as-file-blocks (file blocks need uploaded file object ids —
//     R11), no toggle/callout syntax, no HTML passthrough: all degrade to
//     paragraph text.

import (
	"encoding/json"
	"regexp"
	"strings"
)

// mdBlock is one parsed block before JSON encoding.
type mdBlock struct {
	indent  int
	typ     string
	text    string
	hasText bool
	extra   map[string]any // checked, language, …
}

var (
	mdHeadingRe = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	mdDividerRe = regexp.MustCompile(`^\s*(?:(?:-\s*){3,}|(?:\*\s*){3,}|(?:_\s*){3,})$`)
	mdFenceRe   = regexp.MustCompile("^(`{3,}|~{3,})\\s*(\\S*).*$")
	// mdFenceLangRe bounds the info string to a language-ish token; noise
	// (backtick runs, punctuation soup) is dropped rather than stored.
	mdFenceLangRe = regexp.MustCompile(`^[A-Za-z0-9_+#.-]{0,32}$`)
	mdBulletRe    = regexp.MustCompile(`^([-*+])\s+(.*)$`)
	mdNumberRe    = regexp.MustCompile(`^(\d{1,9})[.)]\s+(.*)$`)
	mdCheckRe     = regexp.MustCompile(`^\[( |x|X)\]\s+(.*)$`)
	mdTableSepRe  = regexp.MustCompile(`^\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
)

// ParseMarkdownBlocks converts markdown into a flat §4 blocks run (id-less
// JSON block objects with run-relative indents). It never fails — see the
// file comment for the degradation rules. An input of only whitespace
// produces an empty run.
func ParseMarkdownBlocks(md string) []json.RawMessage {
	run, _ := ParseMarkdownBlocksLimit(md, 0)
	return run
}

// ParseMarkdownBlocksLimit parses like ParseMarkdownBlocks but stops as soon
// as the run exceeds maxBlocks (0 = unbounded), reporting the excess instead
// of parsing the rest. The markdown payload channel is byte-bounded by its
// schema but a few bytes can encode one block, so callers that cap a blocks
// array must cap the parsed run with the same number — and the early stop
// keeps a maximum-size hostile body from costing an unbounded parse. When
// exceeded is true the returned run holds maxBlocks+1 blocks (proof of the
// excess, not the full parse) and must not be imported.
func ParseMarkdownBlocksLimit(md string, maxBlocks int) (run []json.RawMessage, exceeded bool) {
	p := &mdParser{}
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	for _, line := range lines {
		p.feed(strings.TrimSuffix(line, "\r"))
		if maxBlocks > 0 && len(p.blocks) > maxBlocks {
			return encodeMdBlocks(p.blocks[:maxBlocks+1]), true
		}
	}
	p.flush()
	if p.inFence {
		p.closeFence()
	}
	if maxBlocks > 0 && len(p.blocks) > maxBlocks {
		return encodeMdBlocks(p.blocks[:maxBlocks+1]), true
	}
	return encodeMdBlocks(p.blocks), false
}

type mdParser struct {
	blocks []mdBlock

	// paragraph accumulator
	para       []string
	paraIndent int
	inPara     bool

	// quote accumulator
	quote       []string
	quoteIndent int
	inQuote     bool

	// fence accumulator
	inFence     bool
	fenceMarker string
	fenceIndent int
	fenceStrip  int
	fenceLang   string
	fenceLines  []string

	// table accumulator
	tableLines  []string
	tableIndent int
	inTable     bool
}

// feed processes one input line.
func (p *mdParser) feed(line string) {
	if p.inFence {
		// a closing marker tolerates at most 3 leading spaces (CommonMark);
		// deeper-indented marker runs are fence CONTENT (an indented markdown
		// example inside a fence must not terminate it)
		lead := 0
		for lead < len(line) && line[lead] == ' ' {
			lead++
		}
		trimmed := strings.TrimRight(line[lead:], " \t")
		if lead <= 3 &&
			strings.HasPrefix(trimmed, p.fenceMarker[:1]) &&
			strings.TrimRight(trimmed, string(p.fenceMarker[0])) == "" &&
			len(trimmed) >= len(p.fenceMarker) {
			p.closeFence()
			return
		}
		p.fenceLines = append(p.fenceLines, stripIndentPrefix(line, p.fenceStrip))
		return
	}

	stripped, level, cols := splitMdIndent(line)

	if strings.TrimSpace(stripped) == "" {
		p.flush()
		return
	}

	// fence open
	if m := mdFenceRe.FindStringSubmatch(stripped); m != nil {
		p.flush()
		p.inFence = true
		p.fenceMarker = m[1]
		p.fenceIndent = level
		p.fenceStrip = cols
		p.fenceLang = m[2]
		if !mdFenceLangRe.MatchString(p.fenceLang) {
			p.fenceLang = ""
		}
		p.fenceLines = nil
		return
	}

	// heading
	if m := mdHeadingRe.FindStringSubmatch(stripped); m != nil {
		p.flush()
		depth := len(m[1])
		if depth > 3 {
			depth = 3
		}
		p.emit(mdBlock{indent: level, typ: "heading_" + string(rune('0'+depth)), text: m[2], hasText: true})
		return
	}

	// divider (before list: `- - -` has no item content)
	if mdDividerRe.MatchString(stripped) {
		p.flush()
		p.emit(mdBlock{indent: level, typ: "divider"})
		return
	}

	// table rows
	if strings.HasPrefix(stripped, "|") {
		if !p.inTable {
			p.flush()
			p.inTable = true
			p.tableIndent = level
			p.tableLines = nil
		}
		p.tableLines = append(p.tableLines, stripped)
		return
	}
	if p.inTable {
		p.flush()
	}

	// quote
	if strings.HasPrefix(stripped, ">") {
		if !p.inQuote {
			p.flush()
			p.inQuote = true
			p.quoteIndent = level
			p.quote = nil
		}
		content := strings.TrimPrefix(stripped, ">")
		content = strings.TrimPrefix(content, " ")
		p.quote = append(p.quote, content)
		return
	}
	if p.inQuote {
		p.flush()
	}

	// list items
	if m := mdBulletRe.FindStringSubmatch(stripped); m != nil {
		p.flush()
		rest := m[2]
		if cm := mdCheckRe.FindStringSubmatch(rest); cm != nil {
			block := mdBlock{indent: level, typ: "checkbox", text: cm[2], hasText: true}
			if cm[1] != " " {
				block.extra = map[string]any{"checked": true}
			}
			p.emit(block)
			return
		}
		p.emit(mdBlock{indent: level, typ: "bulleted_list_item", text: rest, hasText: true})
		return
	}
	if m := mdNumberRe.FindStringSubmatch(stripped); m != nil {
		p.flush()
		p.emit(mdBlock{indent: level, typ: "numbered_list_item", text: m[2], hasText: true})
		return
	}

	// paragraph (join consecutive plain lines)
	if p.inPara {
		p.para = append(p.para, strings.TrimRight(stripped, " \t"))
		return
	}
	p.inPara = true
	p.paraIndent = level
	p.para = []string{strings.TrimRight(stripped, " \t")}
}

// flush closes every open accumulator except the fence (fences only close on
// their marker or at end of input).
func (p *mdParser) flush() {
	if p.inPara {
		p.emit(mdBlock{indent: p.paraIndent, typ: "paragraph", text: strings.Join(p.para, "\n"), hasText: true})
		p.inPara = false
		p.para = nil
	}
	if p.inQuote {
		p.emit(mdBlock{indent: p.quoteIndent, typ: "quote", text: strings.Join(p.quote, "\n"), hasText: true})
		p.inQuote = false
		p.quote = nil
	}
	if p.inTable {
		p.emitTable()
		p.inTable = false
		p.tableLines = nil
	}
}

func (p *mdParser) closeFence() {
	block := mdBlock{indent: p.fenceIndent, typ: "code", text: strings.Join(p.fenceLines, "\n"), hasText: true}
	if p.fenceLang != "" {
		block.extra = map[string]any{"language": p.fenceLang}
	}
	p.emit(block)
	p.inFence = false
	p.fenceLines = nil
}

// emit appends a block, clamping its indent so the run always imports:
// at most the previous block's level + 1 (§4 strict monotonicity, first
// block at 0), never deeper than a §5 leaf predecessor's own level (leaf
// blocks cannot have children — V2), and never past the F4 absolute bound.
func (p *mdParser) emit(b mdBlock) {
	max := 0
	if n := len(p.blocks); n > 0 {
		prev := p.blocks[n-1]
		max = prev.indent + 1
		if leafBlockTypes[prev.typ] {
			// a line cannot open a level under a block that cannot have
			// children — it stays the leaf's sibling
			max = prev.indent
		}
	}
	if max > maxBlockIndent {
		max = maxBlockIndent
	}
	if b.indent > max {
		b.indent = max
	}
	p.blocks = append(p.blocks, b)
}

// emitTable converts the accumulated `|` lines into a §6.1 table block, or
// degrades them to one paragraph when the separator row is missing.
func (p *mdParser) emitTable() {
	if len(p.tableLines) < 2 || !mdTableSepRe.MatchString(p.tableLines[1]) {
		p.emit(mdBlock{indent: p.tableIndent, typ: "paragraph", text: strings.Join(p.tableLines, "\n"), hasText: true})
		return
	}
	header := splitMdRow(p.tableLines[0])
	width := len(header)
	rows := make([]map[string]any, 0, len(p.tableLines)-1)
	rows = append(rows, map[string]any{"is_header": true, "cells": mdCells(header)})
	for _, line := range p.tableLines[2:] {
		cells := splitMdRow(line)
		if len(cells) > width {
			width = len(cells)
		}
		rows = append(rows, map[string]any{"cells": mdCells(cells)})
	}
	columns := make([]map[string]any, width)
	for i := range columns {
		columns[i] = map[string]any{}
	}
	p.emit(mdBlock{indent: p.tableIndent, typ: "table", extra: map[string]any{
		"columns": columns,
		"rows":    rows,
	}})
}

// mdCells maps row cells onto the §6.1 cell forms: null for empties, the
// string shorthand otherwise (cells are inline markup source).
func mdCells(cells []string) []any {
	out := make([]any, len(cells))
	for i, c := range cells {
		if c == "" {
			out[i] = nil
			continue
		}
		out[i] = c
	}
	return out
}

// splitMdRow splits one `| a | b |` line into trimmed cell strings,
// honouring `\|` escapes.
func splitMdRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	var cells []string
	var cur strings.Builder
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			if r != '|' {
				cur.WriteByte('\\')
			}
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '|':
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if escaped {
		cur.WriteByte('\\')
	}
	cells = append(cells, strings.TrimSpace(cur.String()))
	return cells
}

// splitMdIndent strips leading whitespace and reports the nesting level it
// implies (2 spaces or one tab per level) plus the number of leading
// whitespace characters consumed.
func splitMdIndent(line string) (stripped string, level, cols int) {
	spaces := 0
	i := 0
	for ; i < len(line); i++ {
		switch line[i] {
		case ' ':
			spaces++
		case '\t':
			spaces += 2
		default:
			return line[i:], spaces / 2, i
		}
	}
	return "", 0, i
}

// stripIndentPrefix removes up to n leading whitespace characters (fence
// content keeps its own deeper indentation).
func stripIndentPrefix(line string, n int) string {
	for i := 0; i < n && line != ""; i++ {
		if line[0] != ' ' && line[0] != '\t' {
			break
		}
		line = line[1:]
	}
	return line
}

// encodeMdBlocks renders parsed blocks as flat JSON block objects (canonical
// omissions: no indent 0, no empty text).
func encodeMdBlocks(blocks []mdBlock) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(blocks))
	for _, b := range blocks {
		obj := map[string]any{"type": b.typ}
		if b.indent > 0 {
			obj["indent"] = b.indent
		}
		if b.hasText && b.text != "" {
			obj["text"] = b.text
		}
		for k, v := range b.extra {
			obj[k] = v
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			// map[string]any of JSON-safe values cannot fail to marshal
			continue
		}
		out = append(out, raw)
	}
	return out
}
