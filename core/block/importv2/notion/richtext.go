package notion

import (
	"regexp"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

// renderedText is a rich-text run sequence rendered to anytype form: text
// with UTF-16 mark ranges, plus standalone latex blocks for equation runs.
type renderedText struct {
	text      string
	marks     []*model.BlockContentTextMark
	equations []string
}

// renderedPiece is one ordered segment of a rich-text sequence: either a
// text span (with piece-relative UTF-16 mark ranges) or an inline equation.
// Anytype has no inline latex, so equations split the text at their exact
// position instead of being torn out of order (the v1 behavior left a gap
// mid-sentence and appended the formula elsewhere).
type renderedPiece struct {
	text     string
	marks    []*model.BlockContentTextMark
	equation string
}

// renderRichTextPieces converts rich-text runs preserving equation order.
// Mark offsets are UTF-16 (the range convention shared with Notion/JS).
func (c *Converter) renderRichTextPieces(runs []richText) []renderedPiece {
	var pieces []renderedPiece
	var current renderedPiece
	offset := 0
	flush := func() {
		if current.text != "" {
			pieces = append(pieces, current)
		}
		current = renderedPiece{}
		offset = 0
	}
	appendText := func(text string, marks ...*model.BlockContentTextMark) {
		if text == "" {
			return
		}
		length := textutil.UTF16RuneCountString(text)
		for _, mark := range marks {
			mark.Range = &model.Range{From: int32(offset), To: int32(offset + length)}
			current.marks = append(current.marks, mark)
		}
		current.text += text
		offset += length
	}

	for _, run := range runs {
		switch {
		case run.Equation != nil:
			flush()
			pieces = append(pieces, renderedPiece{equation: run.Equation.Expression})
		case run.Mention != nil:
			c.renderMention(run, appendText)
		default:
			appendText(run.PlainText, c.annotationMarks(run)...)
		}
	}
	flush()
	return pieces
}

// renderRichText flattens the pieces into one text span (equations
// collected separately) — for contexts that cannot split, like table cells
// and code blocks.
func (c *Converter) renderRichText(runs []richText) renderedText {
	var rendered renderedText
	offset := 0
	for _, piece := range c.renderRichTextPieces(runs) {
		if piece.equation != "" {
			rendered.equations = append(rendered.equations, piece.equation)
			continue
		}
		for _, mark := range piece.marks {
			mark.Range.From += int32(offset)
			mark.Range.To += int32(offset)
		}
		rendered.marks = append(rendered.marks, piece.marks...)
		rendered.text += piece.text
		offset += textutil.UTF16RuneCountString(piece.text)
	}
	return rendered
}

func (c *Converter) annotationMarks(run richText) []*model.BlockContentTextMark {
	var marks []*model.BlockContentTextMark
	add := func(markType model.BlockContentTextMarkType, param string) {
		marks = append(marks, &model.BlockContentTextMark{Type: markType, Param: param})
	}
	if a := run.Annotations; a != nil {
		if a.Bold {
			add(model.BlockContentTextMark_Bold, "")
		}
		if a.Italic {
			add(model.BlockContentTextMark_Italic, "")
		}
		if a.Strikethrough {
			add(model.BlockContentTextMark_Strikethrough, "")
		}
		if a.Underline {
			add(model.BlockContentTextMark_Underscored, "")
		}
		if a.Code {
			add(model.BlockContentTextMark_Keyboard, "")
		}
		if a.Color != "" && a.Color != "default" {
			if strings.Contains(a.Color, "background") {
				add(model.BlockContentTextMark_BackgroundColor, anytypeColor(a.Color))
			} else {
				add(model.BlockContentTextMark_TextColor, anytypeColor(a.Color))
			}
		}
	}
	if target := c.linkTarget(run); target != nil {
		marks = append(marks, target)
	}
	return marks
}

// linkTarget maps a run's link: intra-workspace notion URLs become mentions
// of the imported object, everything else stays a web link.
func (c *Converter) linkTarget(run richText) *model.BlockContentTextMark {
	href := run.Href
	if run.Text != nil && run.Text.Link != nil && run.Text.Link.Url != "" {
		href = run.Text.Link.Url
	}
	if href == "" {
		return nil
	}
	if id, ok := c.entityIdFromUrl(href); ok {
		return &model.BlockContentTextMark{Type: model.BlockContentTextMark_Mention, Param: id}
	}
	return &model.BlockContentTextMark{Type: model.BlockContentTextMark_Link, Param: href}
}

func (c *Converter) renderMention(run richText, appendText func(string, ...*model.BlockContentTextMark)) {
	m := run.Mention
	display := run.PlainText
	// Mentions keep the run's own annotations (bold/italic/color survive).
	baseMarks := c.annotationMarks(run)
	withMention := func(param string) []*model.BlockContentTextMark {
		return append(append([]*model.BlockContentTextMark{}, baseMarks...),
			&model.BlockContentTextMark{Type: model.BlockContentTextMark_Mention, Param: param})
	}
	switch m.Type {
	case "page":
		if m.Page != nil {
			appendText(display, withMention(m.Page.Id)...)
			return
		}
	case "database":
		// Mentions reference the database, not its data sources.
		if m.Database != nil {
			if target, ok := c.resolveDatabaseRef(m.Database.Id); ok {
				appendText(display, withMention(target)...)
			} else {
				appendText(display, baseMarks...)
			}
			return
		}
	case "date":
		if m.Date != nil && len(m.Date.Start) >= 10 {
			// The date-object id carries only the calendar date; a timed
			// mention keeps its date part (v1 dropped it entirely).
			appendText(display, withMention(addr.DatePrefix+m.Date.Start[:10])...)
			return
		}
	case "user":
		if m.User != nil && m.User.Name != "" {
			appendText(m.User.Name, baseMarks...)
			return
		}
	case "link_preview":
		if m.LinkPreview != nil && m.LinkPreview.Url != "" {
			// v1 emitted a Link mark with an empty param here.
			appendText(display, append(append([]*model.BlockContentTextMark{}, baseMarks...),
				&model.BlockContentTextMark{Type: model.BlockContentTextMark_Link, Param: m.LinkPreview.Url})...)
			return
		}
	case "custom_emoji":
		if m.CustomEmoji != nil {
			appendText(":"+m.CustomEmoji.Name+":", baseMarks...)
			return
		}
	case "template_mention":
		// @Today/@Now/@Me placeholders: emit a stable label instead of
		// whatever plain_text Notion happened to freeze.
		if m.TemplateMention != nil {
			label := m.TemplateMention.Date
			if m.TemplateMention.Type == "template_mention_user" {
				label = m.TemplateMention.User
			}
			if label == "" {
				label = display
			}
			appendText(label, baseMarks...)
			return
		}
	}
	appendText(display, baseMarks...)
}

var notionIdInUrl = regexp.MustCompile(`([0-9a-fA-F]{32})(?:[?#].*)?$`)

// entityIdFromUrl recognizes notion.so links to imported entities.
func (c *Converter) entityIdFromUrl(rawUrl string) (string, bool) {
	if !strings.Contains(rawUrl, "notion.so") {
		return "", false
	}
	match := notionIdInUrl.FindStringSubmatch(rawUrl)
	if match == nil {
		return "", false
	}
	id := canonicalUuid(match[1])
	if _, ok := c.entityById[id]; ok {
		return id, true
	}
	// The URL may address a database whose collections were imported as
	// data sources.
	return c.resolveDatabaseRef(id)
}

func canonicalUuid(hex32 string) string {
	hex32 = strings.ToLower(hex32)
	return hex32[0:8] + "-" + hex32[8:12] + "-" + hex32[12:16] + "-" + hex32[16:20] + "-" + hex32[20:32]
}
