package v2service

// chatmarkdown.go parses a space chat's message text. A space chat stores a
// message as ONE text with marks, so of block markdown it can show only what
// marks express: a heading line becomes a bold line, and a fenced code block
// becomes a code mark over the code — the shape the desktop composer stores
// for ``` fences and renders as a code block (the language has nowhere to
// live and is dropped). Heading and fence-opener lines are recognized by the
// AnyBlock markdown parser one line at a time, so they match what an object
// body's markdown makes of them; everything else — lists, quotes, blank
// lines — goes through the inline codec as before.

import (
	"encoding/json"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

type chatLineKind int

const (
	chatLineInline chatLineKind = iota
	chatLineHeading
	chatLineFence
)

// chatLine is one classified line: a heading carries its inline text, a
// fence opener its marker run and indentation.
type chatLine struct {
	kind   chatLineKind
	text   string
	marker string
	indent int
}

// parseChatText parses a message text for the chat's store: a space chat's
// content takes headings and fences as marks, a discussion's blocks the
// inline markup.
func parseChatText(layout model.ObjectTypeLayout, md string) (string, []*model.BlockContentTextMark, error) {
	if chatWritesBlocks(layout) {
		return anyblockjson.ParseInlineText(md)
	}
	return parseChatContent(md)
}

// parseChatContent parses markdown into the text and marks of a space chat
// message's content.
func parseChatContent(md string) (string, []*model.BlockContentTextMark, error) {
	var out chatContentBuilder
	var run []string
	flush := func() error {
		if run == nil {
			return nil
		}
		text, marks, err := anyblockjson.ParseInlineText(strings.Join(run, "\n"))
		if err != nil {
			return err
		}
		out.appendLine(text, marks)
		run = nil
		return nil
	}
	lines := strings.Split(md, "\n")
	for i := 0; i < len(lines); i++ {
		line := classifyChatLine(lines[i])
		if line.kind == chatLineInline {
			run = append(run, lines[i])
			continue
		}
		if err := flush(); err != nil {
			return "", nil, err
		}
		switch line.kind {
		case chatLineHeading:
			text, marks, err := anyblockjson.ParseInlineText(line.text)
			if err != nil {
				return "", nil, err
			}
			out.appendLine(text, headingMarks(text, marks))
		case chatLineFence:
			end := chatFenceEnd(lines, i+1, line.marker)
			code := make([]string, 0, end-i-1)
			for _, codeLine := range lines[i+1 : end] {
				code = append(code, trimLeadingSpaces(codeLine, line.indent))
			}
			// an empty fence is dropped, as the desktop composer drops it
			if joined := strings.Join(code, "\n"); strings.TrimSpace(joined) != "" {
				out.appendLine(joined, []*model.BlockContentTextMark{{
					Range: &model.Range{From: 0, To: utf16Len(joined)},
					Type:  model.BlockContentTextMark_Keyboard,
				}})
			}
			i = end
		}
	}
	if err := flush(); err != nil {
		return "", nil, err
	}
	return out.text.String(), out.marks, nil
}

// classifyChatLine asks the AnyBlock markdown parser what a line alone is.
// Only a line starting with #, ` or ~ can be a heading or a fence opener, so
// ordinary lines skip the parser.
func classifyChatLine(line string) chatLine {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || !strings.ContainsRune("#`~", rune(trimmed[0])) {
		return chatLine{}
	}
	run := anyblockjson.ParseMarkdownBlocks(line)
	if len(run) != 1 {
		return chatLine{}
	}
	var block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(run[0], &block); err != nil {
		return chatLine{}
	}
	switch {
	case strings.HasPrefix(block.Type, "heading_"):
		return chatLine{kind: chatLineHeading, text: block.Text}
	case block.Type == "code":
		marker := trimmed[:len(trimmed)-len(strings.TrimLeft(trimmed, trimmed[:1]))]
		return chatLine{kind: chatLineFence, marker: marker, indent: len(line) - len(trimmed)}
	}
	return chatLine{}
}

// chatFenceEnd finds the line that closes a fence: at most three leading
// spaces, then a run of the opener's character at least as long as the
// opener and nothing else — the AnyBlock parser's closing rule. An unclosed
// fence runs to the end.
func chatFenceEnd(lines []string, from int, marker string) int {
	for i := from; i < len(lines); i++ {
		lead := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		trimmed := strings.TrimRight(lines[i][lead:], " \t\r")
		if lead <= 3 && len(trimmed) >= len(marker) && strings.Trim(trimmed, marker[:1]) == "" {
			return i
		}
	}
	return len(lines)
}

// headingMarks makes a heading's whole line bold; its own bold marks would
// only overlap that one, so they go.
func headingMarks(text string, marks []*model.BlockContentTextMark) []*model.BlockContentTextMark {
	var out []*model.BlockContentTextMark
	if text != "" {
		out = append(out, &model.BlockContentTextMark{
			Range: &model.Range{From: 0, To: utf16Len(text)},
			Type:  model.BlockContentTextMark_Bold,
		})
	}
	for _, mark := range marks {
		if mark != nil && mark.Type != model.BlockContentTextMark_Bold {
			out = append(out, mark)
		}
	}
	return out
}

func trimLeadingSpaces(line string, n int) string {
	i := 0
	for i < n && i < len(line) && line[i] == ' ' {
		i++
	}
	return line[i:]
}

func utf16Len(s string) int32 {
	return int32(len(textutil.StrToUTF16(s)))
}

// chatContentBuilder joins parsed lines with newlines, rebasing each line's
// marks onto the joined text in UTF-16 units.
type chatContentBuilder struct {
	text    strings.Builder
	units   int32
	started bool
	marks   []*model.BlockContentTextMark
}

func (b *chatContentBuilder) appendLine(text string, marks []*model.BlockContentTextMark) {
	if b.started {
		b.text.WriteByte('\n')
		b.units++
	}
	b.started = true
	for _, mark := range marks {
		if mark == nil || mark.Range == nil {
			continue
		}
		rebased := *mark
		rebased.Range = &model.Range{From: mark.Range.From + b.units, To: mark.Range.To + b.units}
		b.marks = append(b.marks, &rebased)
	}
	b.text.WriteString(text)
	b.units += utf16Len(text)
}
