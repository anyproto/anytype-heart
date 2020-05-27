package html

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/util/slice"

	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/anymark/blocksUtil"
	"github.com/anytypeio/go-anytype-middleware/anymark/renderer"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"
)

var log = logging.Logger("anytype-anymark")

// A Config struct has configurations for the HTML based renderers.
type Config struct {
	Writer    Writer
	HardWraps bool
	XHTML     bool
	Unsafe    bool
}

// NewConfig returns a new Config with defaults.
func NewConfig() Config {
	return Config{
		Writer:    DefaultWriter,
		HardWraps: false,
		XHTML:     false,
		Unsafe:    false,
	}
}

// SetOption implements renderer.NodeRenderer.SetOption.
func (c *Config) SetOption(name renderer.OptionName, value interface{}) {
	switch name {
	case optHardWraps:
		c.HardWraps = value.(bool)
	case optXHTML:
		c.XHTML = value.(bool)
	case optUnsafe:
		c.Unsafe = value.(bool)
	case optTextWriter:
		c.Writer = value.(Writer)
	}
}

// An Option interface sets options for HTML based renderers.
type Option interface {
	SetHTMLOption(*Config)
}

// TextWriter is an option name used in WithWriter.
const optTextWriter renderer.OptionName = "Writer"

type withWriter struct {
	value Writer
}

func (o *withWriter) SetConfig(c *renderer.Config) {
	c.Options[optTextWriter] = o.value
}

func (o *withWriter) SetHTMLOption(c *Config) {
	c.Writer = o.value
}

// WithWriter is a functional option that allow you to set the given writer to
// the renderer.
func WithWriter(writer Writer) interface {
	renderer.Option
	Option
} {
	return &withWriter{writer}
}

// HardWraps is an option name used in WithHardWraps.
const optHardWraps renderer.OptionName = "HardWraps"

type withHardWraps struct {
}

func (o *withHardWraps) SetConfig(c *renderer.Config) {
	c.Options[optHardWraps] = true
}

func (o *withHardWraps) SetHTMLOption(c *Config) {
	c.HardWraps = true
}

// WithHardWraps is a functional option that indicates whether softline breaks
// should be rendered as '<br>'.
func WithHardWraps() interface {
	renderer.Option
	Option
} {
	return &withHardWraps{}
}

// XHTML is an option name used in WithXHTML.
const optXHTML renderer.OptionName = "XHTML"

type withXHTML struct {
}

func (o *withXHTML) SetConfig(c *renderer.Config) {
	c.Options[optXHTML] = true
}

func (o *withXHTML) SetHTMLOption(c *Config) {
	c.XHTML = true
}

// WithXHTML is a functional option indicates that nodes should be rendered in
// xhtml instead of HTML5.
func WithXHTML() interface {
	Option
	renderer.Option
} {
	return &withXHTML{}
}

// Unsafe is an option name used in WithUnsafe.
const optUnsafe renderer.OptionName = "Unsafe"

type withUnsafe struct {
}

func (o *withUnsafe) SetConfig(c *renderer.Config) {
	c.Options[optUnsafe] = true
}

func (o *withUnsafe) SetHTMLOption(c *Config) {
	c.Unsafe = true
}

// WithUnsafe is a functional option that renders dangerous contents
// (raw htmls and potentially dangerous links) as it is.
func WithUnsafe() interface {
	renderer.Option
	Option
} {
	return &withUnsafe{}
}

// A Renderer struct is an implementation of renderer.NodeRenderer that renders
// nodes as (X)HTML.
type Renderer struct {
	Config
}

// NewRenderer returns a new Renderer with given options.
func NewRenderer(opts ...Option) renderer.NodeRenderer {
	r := &Renderer{
		Config: NewConfig(),
	}

	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
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
}

func (r *Renderer) writeLines(w blocksUtil.RWriter, source []byte, n ast.Node) {
	l := n.Lines().Len()
	for i := 0; i < l; i++ {
		line := n.Lines().At(i)
		w.AddTextToBuffer(string(line.Value(source)))
	}
}

// GlobalAttributeFilter defines attribute names which any elements can have.
var GlobalAttributeFilter = util.NewBytesFilter(
	[]byte("accesskey"),
	[]byte("autocapitalize"),
	[]byte("class"),
	[]byte("contenteditable"),
	[]byte("contextmenu"),
	[]byte("dir"),
	[]byte("draggable"),
	[]byte("dropzone"),
	[]byte("hidden"),
	[]byte("id"),
	[]byte("itemprop"),
	[]byte("lang"),
	[]byte("slot"),
	[]byte("spellcheck"),
	[]byte("style"),
	[]byte("tabindex"),
	[]byte("title"),
	[]byte("translate"),
)

func (r *Renderer) renderDocument(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// nothing to do
	return ast.WalkContinue, nil
}

// HeadingAttributeFilter defines attribute names which heading elements can have
var HeadingAttributeFilter = GlobalAttributeFilter

func (r *Renderer) renderHeading(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
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
		style = model.BlockContentText_Header4
	case 5:
		style = model.BlockContentText_Header4
	case 6:
		style = model.BlockContentText_Header4
	}

	if entering {
		w.OpenNewTextBlock(style)
	} else {
		w.CloseTextBlock(style)
	}
	return ast.WalkContinue, nil
}

// BlockquoteAttributeFilter defines attribute names which blockquote elements can have
var BlockquoteAttributeFilter = GlobalAttributeFilter.Extend(
	[]byte("cite"),
)

func (r *Renderer) renderBlockquote(w blocksUtil.RWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.OpenNewTextBlock(model.BlockContentText_Quote)
	} else {
		w.CloseTextBlock(model.BlockContentText_Quote)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderCodeBlock(w blocksUtil.RWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.OpenNewTextBlock(model.BlockContentText_Code)
	} else {
		w.CloseTextBlock(model.BlockContentText_Code)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderFencedCodeBlock(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.FencedCodeBlock)
	if entering {
		w.OpenNewTextBlock(model.BlockContentText_Code)
		r.writeLines(w, source, n)
	} else {
		w.CloseTextBlock(model.BlockContentText_Code)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderHTMLBlock(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// Do not render
	return ast.WalkContinue, nil
}

// ListAttributeFilter defines attribute names which list elements can have.
var ListAttributeFilter = GlobalAttributeFilter.Extend(
	[]byte("start"),
	[]byte("reversed"),
)

func (r *Renderer) renderList(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.List)

	if n.IsOrdered() && entering {
		w.SetIsNumberedList(true)
	} else {
		w.SetIsNumberedList(false)
	}

	return ast.WalkContinue, nil
}

// ListItemAttributeFilter defines attribute names which list item elements can have.
var ListItemAttributeFilter = GlobalAttributeFilter.Extend(
	[]byte("value"),
)

func (r *Renderer) renderListItem(w blocksUtil.RWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	tag := model.BlockContentText_Marked
	if w.GetIsNumberedList() {
		tag = model.BlockContentText_Numbered
	}

	if entering {
		w.OpenNewTextBlock(tag)
	} else {
		w.CloseTextBlock(tag)
	}
	return ast.WalkContinue, nil
}

// ParagraphAttributeFilter defines attribute names which paragraph elements can have.
var ParagraphAttributeFilter = GlobalAttributeFilter

func (r *Renderer) renderParagraph(w blocksUtil.RWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.OpenNewTextBlock(model.BlockContentText_Paragraph)
	} else {
		w.CloseTextBlock(model.BlockContentText_Paragraph)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderTextBlock(w blocksUtil.RWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		// TODO: check it
		//w.CloseTextBlock(model.BlockContentText_Paragraph)
	}
	return ast.WalkContinue, nil
}

// ThematicAttributeFilter defines attribute names which hr elements can have.
var ThematicAttributeFilter = GlobalAttributeFilter.Extend(
	[]byte("align"),   // [Deprecated]
	[]byte("color"),   // [Not Standardized]
	[]byte("noshade"), // [Deprecated]
	[]byte("size"),    // [Deprecated]
	[]byte("width"),   // [Deprecated]
)

func (r *Renderer) renderThematicBreak(w blocksUtil.RWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.ForceCloseTextBlock()
	} else {
		w.AddDivider()
	}

	return ast.WalkContinue, nil
}

// LinkAttributeFilter defines attribute names which link elements can have.
var LinkAttributeFilter = GlobalAttributeFilter.Extend(
	[]byte("download"),
	// []byte("href"),
	[]byte("hreflang"),
	[]byte("media"),
	[]byte("ping"),
	[]byte("referrerpolicy"),
	[]byte("rel"),
	[]byte("shape"),
	[]byte("target"),
)

func (r *Renderer) renderAutoLink(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.AutoLink)
	if !entering {
		return ast.WalkContinue, nil
	}

	destination := source
	label := n.Label(source)
	w.SetMarkStart()

	start := int32(utf8.RuneCountInString(w.GetText()))
	labelLength := int32(utf8.RuneCount(label))
	if slice.FindPos(w.GetAllFileShortPaths(), strings.Replace(string(source), "%20", " ", -1)) != -1 {

	}

	destinationConverted := strings.Replace(string(destination), "%20", " ", -1)
	for _, sPath := range w.GetAllFileShortPaths() {
		if strings.Contains(sPath, destinationConverted) {
			destination = []byte(sPath)
		}
	}

	w.AddMark(model.BlockContentTextMark{
		Range: &model.Range{From: start, To: start + labelLength},
		Type:  model.BlockContentTextMark_Link,
		Param: string(n.URL(destination)),
	})

	_, _ = w.Write(util.EscapeHTML(label))
	return ast.WalkContinue, nil
}

// CodeAttributeFilter defines attribute names which code elements can have.
var CodeAttributeFilter = GlobalAttributeFilter

func (r *Renderer) renderCodeSpan(w blocksUtil.RWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.SetMarkStart()

		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			segment := c.(*ast.Text).Segment
			value := segment.Value(source)
			if bytes.HasSuffix(value, []byte("\n")) {
				w.AddTextToBuffer(string(value[:len(value)-1]))
				if c != n.LastChild() {
					w.AddTextToBuffer(" ")
				}
			} else {
				w.AddTextToBuffer(string(value))
			}
		}
		return ast.WalkSkipChildren, nil
	} else {
		to := int32(utf8.RuneCountInString(w.GetText()))

		w.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(w.GetMarkStart()), To: to},
			Type:  model.BlockContentTextMark_Keyboard,
		})
	}
	return ast.WalkContinue, nil
}

// EmphasisAttributeFilter defines attribute names which emphasis elements can have.
var EmphasisAttributeFilter = GlobalAttributeFilter

func (r *Renderer) renderEmphasis(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Emphasis)
	tag := model.BlockContentTextMark_Italic
	if n.Level == 2 {
		tag = model.BlockContentTextMark_Bold
	}

	if entering {
		w.SetMarkStart()
	} else {
		to := int32(utf8.RuneCountInString(w.GetText()))

		w.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(w.GetMarkStart()), To: to},
			Type:  tag,
		})
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderLink(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)

	destination := n.Destination

	if entering {
		w.SetMarkStart()
	} else {

		destinationConverted := strings.Replace(string(destination), "%20", " ", -1)
		for _, sPath := range w.GetAllFileShortPaths() {
			if strings.Contains(sPath, destinationConverted) {
				destination = []byte(sPath)
			}
		}

		to := int32(utf8.RuneCountInString(w.GetText()))
		w.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(w.GetMarkStart()), To: to},
			Type:  model.BlockContentTextMark_Link,
			Param: string(destination),
		})
	}
	return ast.WalkContinue, nil
}

// ImageAttributeFilter defines attribute names which image elements can have.
var ImageAttributeFilter = GlobalAttributeFilter.Extend(
	[]byte("align"),
	[]byte("border"),
	[]byte("crossorigin"),
	[]byte("decoding"),
	[]byte("height"),
	[]byte("importance"),
	[]byte("intrinsicsize"),
	[]byte("ismap"),
	[]byte("loading"),
	[]byte("referrerpolicy"),
	[]byte("sizes"),
	[]byte("srcset"),
	[]byte("usemap"),
	[]byte("width"),
)

func (r *Renderer) renderImage(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*ast.Image)
	w.AddImageBlock(string(n.Destination))

	return ast.WalkSkipChildren, nil
}

func (r *Renderer) renderRawHTML(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// DO NOT RENDER
	return ast.WalkSkipChildren, nil
}

func (r *Renderer) renderText(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {

	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Text)
	segment := n.Segment

	r.Writer.Write(w, segment.Value(source))
	if n.HardLineBreak() || (n.SoftLineBreak() && r.HardWraps) {
		w.ForceCloseTextBlock()

	} else if n.SoftLineBreak() {
		w.AddTextToBuffer("\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderString(w blocksUtil.RWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.String)

	w.AddTextToBuffer(string(n.Value))

	return ast.WalkContinue, nil
}

var dataPrefix = []byte("data-")

// A Writer interface wirtes textual contents to a writer.
type Writer interface {
	// Write writes the given source to writer with resolving references and unescaping
	// backslash escaped characters.
	Write(writer blocksUtil.RWriter, source []byte)

	// RawWrite wirtes the given source to writer without resolving references and
	// unescaping backslash escaped characters.
	RawWrite(writer blocksUtil.RWriter, source []byte)
}

type defaultWriter struct {
}

func (d *defaultWriter) RawWrite(writer blocksUtil.RWriter, source []byte) {
	writer.AddTextToBuffer(string(source))
}

func (d *defaultWriter) Write(writer blocksUtil.RWriter, source []byte) {
	d.RawWrite(writer, source)
}

// DefaultWriter is a default implementation of the Writer.
var DefaultWriter = &defaultWriter{}

var bDataImage = []byte("data:image/")
var bPng = []byte("png;")
var bGif = []byte("gif;")
var bJpeg = []byte("jpeg;")
var bWebp = []byte("webp;")
var bJs = []byte("javascript:")
var bVb = []byte("vbscript:")
var bFile = []byte("file:")
var bData = []byte("data:")
