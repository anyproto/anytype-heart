package v2service

// chatmarkdown.go parses a chat message's markdown into the form the chat
// stores. A space chat stores a message as ONE text with marks, so of block
// markdown it can show only what marks express: a heading line becomes a
// bold line, and a fenced code block becomes a code mark over the code — the
// shape the desktop composer stores for ``` fences and renders as a code
// block (the language has nowhere to live and is dropped). Heading and
// fence-opener lines are recognized by the AnyBlock markdown parser one line
// at a time, so they match what an object body's markdown makes of them;
// everything else — lists, quotes, blank lines — goes through the inline
// codec as before.
//
// A discussion stores blocks, so its markdown goes through the AnyBlock
// parser and fragment import whole, and each imported text block becomes a
// chat text block of the same style: the shapes the desktop discussion
// composer writes. Chat blocks are flat, so nesting is flattened; a divider
// is the desktop's "---" paragraph, and a table, which has no chat form,
// becomes one line per row with a warning.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	textblock "github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

type chatLineKind int

const (
	chatLineInline chatLineKind = iota
	chatLineHeading
	chatLineFence
)

// chatLine is one classified line: a heading carries its inline text, a
// fence opener its marker run.
type chatLine struct {
	kind   chatLineKind
	text   string
	marker string
}

// chatBody is a message text parsed into the chat's stored form: content
// for a space chat; for a discussion, blocks beside an EMPTY content object
// (the store serializer dereferences the content unconditionally —
// chatmodel.MarshalAnyenc — and the desktop's discussion composer writes the
// same empty object).
type chatBody struct {
	content  *model.ChatMessageMessageContent
	blocks   []*model.ChatMessageMessageBlock
	warnings []v2model.Issue
}

// parseChatBody parses markdown for the chat's store.
func parseChatBody(layout model.ObjectTypeLayout, md string) (*chatBody, error) {
	if chatWritesBlocks(layout) {
		return parseDiscussionBody(md)
	}
	text, marks, err := parseChatContent(md)
	if err != nil {
		return nil, err
	}
	return &chatBody{content: &model.ChatMessageMessageContent{
		Text:  text,
		Style: model.BlockContentText_Paragraph,
		Marks: marks,
	}}, nil
}

// text is what the message says, the text the length cap counts: the
// content's text, or the blocks' texts newline-joined.
func (b *chatBody) text() string {
	if len(b.blocks) == 0 {
		return b.content.Text
	}
	texts := make([]string, 0, len(b.blocks))
	for _, block := range b.blocks {
		texts = append(texts, block.GetText().GetText())
	}
	return strings.Join(texts, "\n")
}

// expandLinks expands the cross-space object links in every mark.
func (b *chatBody) expandLinks(links *spaceLinkExpander) {
	links.Marks(b.content.Marks)
	for _, block := range b.blocks {
		if tb := block.GetText(); tb != nil {
			links.Marks(tb.Marks)
		}
	}
}

// parseDiscussionBody parses markdown into discussion blocks.
func parseDiscussionBody(md string) (*chatBody, error) {
	body := &chatBody{content: &model.ChatMessageMessageContent{Style: model.BlockContentText_Paragraph}}
	run := anyblockjson.ParseMarkdownBlocks(md)
	if len(run) == 0 {
		return body, nil
	}
	next := 0
	imported, topIds, err := anyblockjson.UnmarshalBlocks(run, anyblockjson.Options{GenerateId: func() string {
		next++
		return strconv.Itoa(next)
	}})
	if err != nil {
		return nil, fmt.Errorf("import markdown blocks: %w", err)
	}
	byId := make(map[string]*model.Block, len(imported))
	for _, block := range imported {
		byId[block.Id] = block
	}
	for _, id := range topIds {
		body.addBlock(byId, id)
	}
	return body, nil
}

// addBlock appends an imported block and its children, depth first.
func (b *chatBody) addBlock(byId map[string]*model.Block, id string) {
	block := byId[id]
	if block == nil {
		return
	}
	switch content := block.Content.(type) {
	case *model.BlockContentOfText:
		tb := &model.ChatMessageMessageBlockText{
			Text:    content.Text.Text,
			Style:   content.Text.Style,
			Checked: content.Text.Checked,
		}
		if content.Text.Marks != nil {
			tb.Marks = content.Text.Marks.Marks
		}
		if tb.Style == model.BlockContentText_Code {
			tb.Lang = pbtypes.GetString(block.Fields, textblock.CodeLangFieldName)
		}
		b.addText(tb)
	case *model.BlockContentOfDiv:
		b.addText(&model.ChatMessageMessageBlockText{Text: "---", Style: model.BlockContentText_Paragraph})
	case *model.BlockContentOfTable:
		b.addTable(byId, block)
		return
	}
	for _, child := range block.ChildrenIds {
		b.addBlock(byId, child)
	}
}

// addText appends a text block. The desktop discussion renderer shows no
// break for a newline inside a block, so any block but code becomes one
// block per line; empty blocks — an empty heading, an empty fence — are
// dropped, as the desktop composer drops them.
func (b *chatBody) addText(tb *model.ChatMessageMessageBlockText) {
	if strings.TrimSpace(tb.Text) == "" {
		return
	}
	if tb.Style == model.BlockContentText_Code {
		b.blocks = append(b.blocks, chatTextBlock(tb))
		return
	}
	b.blocks = append(b.blocks, splitTextBlock(tb)...)
}

// addTable appends one paragraph per table row, its cells joined by " | "
// in column order. The import leaves an empty cell out of its row, so cells
// are looked up by the editor's <row id>-<column id> cell ids, and a missing
// one keeps its column empty.
func (b *chatBody) addTable(byId map[string]*model.Block, table *model.Block) {
	var columns, rows []string
	for _, sectionId := range table.ChildrenIds {
		for _, id := range byId[sectionId].GetChildrenIds() {
			switch {
			case byId[id].GetTableColumn() != nil:
				columns = append(columns, id)
			case byId[id].GetTableRow() != nil:
				rows = append(rows, id)
			}
		}
	}
	for _, rowId := range rows {
		line := markedTextBuilder{sep: " | "}
		for _, columnId := range columns {
			cell := byId[rowId+"-"+columnId].GetText()
			line.add(cell.GetText(), cell.GetMarks().GetMarks())
		}
		b.addText(&model.ChatMessageMessageBlockText{
			Text:  line.text.String(),
			Style: model.BlockContentText_Paragraph,
			Marks: line.marks,
		})
	}
	b.warnings = append(b.warnings, v2model.Issue{
		Path:    "/text",
		Message: "a chat message cannot hold a table: each row was posted as one line, its cells separated by |",
	})
}

// parseChatContent parses markdown into the text and marks of a space chat
// message's content.
func parseChatContent(md string) (string, []*model.BlockContentTextMark, error) {
	out := markedTextBuilder{sep: "\n"}
	var run []string
	flush := func() error {
		if run == nil {
			return nil
		}
		text, marks, err := anyblockjson.ParseInlineText(strings.Join(run, "\n"))
		if err != nil {
			return err
		}
		out.add(text, marks)
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
			out.add(text, headingMarks(text, marks))
		case chatLineFence:
			end := chatFenceEnd(lines, i+1, line.marker)
			// an empty fence is dropped, as the desktop composer drops it
			if code := fenceCode(lines[i:min(end+1, len(lines))]); strings.TrimSpace(code) != "" {
				out.add(code, []*model.BlockContentTextMark{{
					Range: &model.Range{From: 0, To: utf16Len(code)},
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

// classifyChatLine asks the AnyBlock markdown parser what a line alone is
// (v2model.MarkdownLine, the classifier the read side escapes prose with).
func classifyChatLine(line string) chatLine {
	blockType, text := v2model.MarkdownLine(line)
	switch {
	case strings.HasPrefix(blockType, "heading_"):
		return chatLine{kind: chatLineHeading, text: text}
	case blockType == "code":
		trimmed := strings.TrimLeft(line, " \t")
		return chatLine{kind: chatLineFence, marker: trimmed[:len(trimmed)-len(strings.TrimLeft(trimmed, trimmed[:1]))]}
	}
	return chatLine{}
}

// fenceCode is the code of a fence, opener through closer (or the end): the
// AnyBlock parser's own reading of those lines, indentation included.
func fenceCode(lines []string) string {
	for _, raw := range anyblockjson.ParseMarkdownBlocks(strings.Join(lines, "\n")) {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &block) == nil && block.Type == "code" {
			return block.Text
		}
	}
	return ""
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

func utf16Len(s string) int32 {
	return int32(len(textutil.StrToUTF16(s)))
}

// markedTextBuilder joins texts with a separator, rebasing each text's marks
// onto the joined text in UTF-16 units.
type markedTextBuilder struct {
	sep     string
	text    strings.Builder
	units   int32
	started bool
	marks   []*model.BlockContentTextMark
}

func (b *markedTextBuilder) add(text string, marks []*model.BlockContentTextMark) {
	if b.started {
		b.text.WriteString(b.sep)
		b.units += utf16Len(b.sep)
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
