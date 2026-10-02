package v2model

// chatmarkdown.go classifies single markdown lines for the chat text
// bridge. The write side recognizes block syntax with it, and the read side
// escapes prose a post would mistake for block syntax, so the two agree by
// construction: both ask the AnyBlock markdown parser.

import (
	"encoding/json"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// MarkdownParagraph is the block type of a line that is plain prose.
const MarkdownParagraph = "paragraph"

// blockSyntaxStarts are the characters a line of block syntax starts with,
// after its indentation; any other line is prose without asking the parser.
const blockSyntaxStarts = "#`~-+*_>|0123456789"

// MarkdownLine reports what one line of markdown is on its own: the AnyBlock
// block type the markdown parser gives it (MarkdownParagraph for prose) and
// that block's text.
func MarkdownLine(line string) (blockType, text string) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || !strings.ContainsRune(blockSyntaxStarts, rune(trimmed[0])) {
		return MarkdownParagraph, line
	}
	run := anyblockjson.ParseMarkdownBlocks(line)
	if len(run) != 1 {
		return MarkdownParagraph, line
	}
	var block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(run[0], &block); err != nil {
		return MarkdownParagraph, line
	}
	return block.Type, block.Text
}

// escapeBlockStart backslash-escapes the character that makes a line read as
// block syntax. An ordered-list number cannot be escaped, so its . or ) is.
func escapeBlockStart(line string) string {
	lead := len(line) - len(strings.TrimLeft(line, " \t"))
	rest := line[lead:]
	if digits := len(rest) - len(strings.TrimLeft(rest, "0123456789")); digits > 0 && digits < len(rest) {
		return line[:lead+digits] + `\` + rest[digits:]
	}
	return line[:lead] + `\` + rest
}

// escapeProse escapes each line of rendered markdown that a post would read
// as block syntax. lineStarts says, per line, whether the line starts a line
// of prose — false inside a code span, where a backslash is literal — and
// isSyntax which block types the write side gives meaning to. A rendering
// whose line count differs from lineStarts is returned unchanged.
func escapeProse(rendered string, lineStarts []bool, isSyntax func(blockType string) bool) string {
	lines := strings.Split(rendered, "\n")
	if len(lines) != len(lineStarts) {
		return rendered
	}
	for i, line := range lines {
		if !lineStarts[i] {
			continue
		}
		if blockType, _ := MarkdownLine(line); blockType != MarkdownParagraph && isSyntax(blockType) {
			lines[i] = escapeBlockStart(line)
		}
	}
	return strings.Join(lines, "\n")
}

// spaceChatSyntax is the block syntax a space chat post gives meaning to:
// headings and fences. Lists, quotes and the rest stay literal there.
func spaceChatSyntax(blockType string) bool {
	return blockType == "code" || strings.HasPrefix(blockType, "heading_")
}
