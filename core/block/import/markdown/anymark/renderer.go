package anymark

import (
	"bytes"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/types"
	"github.com/yuin/goldmark/ast"
	ext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/text"
)

var log = logging.Logger("anytype-anymark")

// BlockLengthSoftLimit is the soft limit for the length of a text block.
// In case text block length exceeds this limit and the soft line break found(e.g. single \n) the new text block will be started.
const TextBlockLengthSoftLimit = 1024

type Renderer struct {
	*blocksRenderer
}

// NewRenderer returns a new Renderer with given options.
func NewRenderer(br *blocksRenderer) *Renderer {
	return &Renderer{
		blocksRenderer: br,
	}
}

// RegisterFuncs implements NodeRenderer.RegisterFuncs .
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// blocks

	reg.Register(ast.KindDocument, r.renderDocument)
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindTextBlock, r.renderTextBlock)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)

	// inlines

	reg.Register(ast.KindAutoLink, r.renderAutoLink)
	reg.Register(ast.KindCodeSpan, r.renderCodeSpan)
	reg.Register(ast.KindEmphasis, r.renderEmphasis)
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
	reg.Register(ast.KindText, r.renderText)
	reg.Register(ast.KindString, r.renderString)
	reg.Register(ext.KindStrikethrough, r.renderStrikethrough)
}

func (r *Renderer) writeLines(source []byte, n ast.Node) {
	l := n.Lines().Len()
	for i := 0; i < l; i++ {
		line := n.Lines().At(i)
		r.AddTextToBuffer(string(line.Value(source)))
	}
}

func (r *Renderer) renderDocument(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	// nothing to do
	return ast.WalkContinue, nil
}

func (r *Renderer) renderHeading(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)

	var style model.BlockContentTextStyle

	switch n.Level {
	case 1:
		style = model.BlockContentText_Header1
	case 2:
		style = model.BlockContentText_Header2
	case 3:
		style = model.BlockContentText_Header3
	case 4:
		style = model.BlockContentText_Header3
	case 5:
		style = model.BlockContentText_Header3
	case 6:
		style = model.BlockContentText_Header3
	}

	r.openTextBlockWithStyle(entering, style, nil)

	return ast.WalkContinue, nil
}

func (r *Renderer) renderBlockquote(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	r.openTextBlockWithStyle(entering, model.BlockContentText_Quote, nil)
	return ast.WalkContinue, nil
}

func (r *Renderer) renderCodeBlock(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	r.openTextBlockWithStyle(entering, model.BlockContentText_Code, nil)
	return ast.WalkContinue, nil
}

func (r *Renderer) renderFencedCodeBlock(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.FencedCodeBlock)
	language := string(n.Language(source))
	var fields *types.Struct
	if language != "" {
		fields = &types.Struct{Fields: map[string]*types.Value{"lang": pbtypes.String(language)}}
	}
	if entering {
		r.openTextBlockWithStyle(entering, model.BlockContentText_Code, fields)
		r.writeLines(source, n)
	} else {
		r.openTextBlockWithStyle(entering, model.BlockContentText_Code, nil)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderHTMLBlock(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	// Do not render
	return ast.WalkContinue, nil
}

func (r *Renderer) renderList(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.List)

	r.SetListState(entering, n.IsOrdered())

	return ast.WalkContinue, nil
}

func (r *Renderer) renderListItem(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	tag := model.BlockContentText_Marked

	if r.GetIsNumberedList() {
		tag = model.BlockContentText_Numbered
	}

	r.openTextBlockWithStyle(entering, tag, nil)
	return ast.WalkContinue, nil
}

func (r *Renderer) renderParagraph(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	r.openTextBlockWithStyle(entering, model.BlockContentText_Paragraph, nil)
	return ast.WalkContinue, nil
}

func (r *Renderer) renderTextBlock(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if !entering {
		// TODO: check it
		// r.CloseTextBlock(model.BlockContentText_Paragraph)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderThematicBreak(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if r.inTable {
		return ast.WalkContinue, nil
	}
	if entering {
		r.ForceCloseTextBlock()
	} else {
		r.AddDivider()
	}

	return ast.WalkContinue, nil
}

func (r *Renderer) renderAutoLink(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.AutoLink)
	if !entering {
		return ast.WalkContinue, nil
	}

	destination := source
	label := n.Label(source)
	r.SetMarkStart()

	start := int32(text.UTF16RuneCountString(r.GetText()))
	labelLength := int32(text.UTF16RuneCount(label))

	linkPath, err := url.PathUnescape(string(destination))
	if err != nil {
		log.Errorf("failed to unescape destination %s", err)
		linkPath = string(destination)
	}

	// add basefilepath
	if !strings.HasPrefix(strings.ToLower(linkPath), "http://") &&
		!strings.HasPrefix(strings.ToLower(linkPath), "https://") {
		linkPath = filepath.Join(r.GetBaseFilepath(), linkPath)
	}

	r.AddMark(model.BlockContentTextMark{
		Range: &model.Range{From: start, To: start + labelLength},
		Type:  model.BlockContentTextMark_Link,
		Param: linkPath,
	})
	r.AddTextToBuffer(string(util.EscapeHTML(label)))
	return ast.WalkContinue, nil
}

func (r *Renderer) renderCodeSpan(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if entering {
		r.SetMarkStart()

		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			segment := c.(*ast.Text).Segment
			value := segment.Value(source)
			if bytes.HasSuffix(value, []byte("\n")) {
				r.AddTextToBuffer(string(value[:len(value)-1]))
				if c != n.LastChild() {
					r.AddTextToBuffer(" ")
				}
			} else {
				r.AddTextToBuffer(string(value))
			}
		}
		return ast.WalkSkipChildren, nil
	}
	to := int32(text.UTF16RuneCountString(r.GetText()))

	r.AddMark(model.BlockContentTextMark{
		Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
		Type:  model.BlockContentTextMark_Keyboard,
	})

	return ast.WalkContinue, nil
}

func (r *Renderer) renderEmphasis(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Emphasis)
	tag := model.BlockContentTextMark_Italic
	if n.Level == 2 {
		tag = model.BlockContentTextMark_Bold
	}

	if entering {
		r.SetMarkStart()
	} else {
		to := int32(text.UTF16RuneCountString(r.GetText()))

		r.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
			Type:  tag,
		})
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderLink(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)

	destination := n.Destination

	if entering {
		r.SetMarkStart()
	} else {

		linkPath, err := url.PathUnescape(string(destination))
		if err != nil {
			log.Errorf("failed to unescape destination %s", err)
			linkPath = string(destination)
		}

		if !strings.HasPrefix(strings.ToLower(linkPath), "http://") &&
			!strings.HasPrefix(strings.ToLower(linkPath), "https://") {
			linkPath = filepath.Join(r.GetBaseFilepath(), linkPath)
		}

		to := int32(text.UTF16RuneCountString(r.GetText()))

		r.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
			Type:  model.BlockContentTextMark_Link,
			Param: linkPath,
		})
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderImage(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*ast.Image)
	if r.inTable {
		return ast.WalkSkipChildren, nil
	}
	r.AddImageBlock(string(n.Destination))

	return ast.WalkSkipChildren, nil
}

func (r *Renderer) renderRawHTML(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n, ok := node.(*ast.RawHTML)
	if !ok {
		return ast.WalkSkipChildren, nil
	}
	for i := 0; i < n.Segments.Len(); i++ {
		segment := n.Segments.At(i)
		tag := segment.Value(source)
		switch string(tag) {
		case "<u>":
			if !entering {
				r.SetMarkStart()
			}
		case "</u>":
			if entering {
				tag := model.BlockContentTextMark_Underscored
				to := int32(text.UTF16RuneCountString(r.GetText()))
				r.AddMark(model.BlockContentTextMark{
					Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
					Type:  tag,
				})
			}
		default:
			return ast.WalkSkipChildren, nil
		}
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderText(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {

	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Text)
	segment := n.Segment

	r.AddTextToBuffer(string(segment.Value(source)))
	if n.HardLineBreak() || n.SoftLineBreak() && r.TextBufferLen() > TextBlockLengthSoftLimit {
		r.openTextBlockWithStyle(false, model.BlockContentText_Paragraph, nil)

	} else if n.SoftLineBreak() {
		r.AddTextToBuffer("\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderString(_ util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.String)

	r.AddTextToBuffer(string(n.Value))

	return ast.WalkContinue, nil
}

func (r *Renderer) renderStrikethrough(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	tag := model.BlockContentTextMark_Strikethrough
	if entering {
		r.SetMarkStart()
	} else {
		to := int32(text.UTF16RuneCountString(r.GetText()))
		r.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
			Type:  tag,
		})
	}
	return ast.WalkContinue, nil
}
