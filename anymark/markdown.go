// Package goldmark implements functions to convert markdown text to a desired format.
package anymark

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/anytypeio/go-anytype-library/pb/model"

	"github.com/anytypeio/go-anytype-middleware/anymark/blocksUtil"
	"github.com/anytypeio/go-anytype-middleware/anymark/renderer"
	"github.com/anytypeio/go-anytype-middleware/anymark/renderer/html"
	"github.com/anytypeio/go-anytype-middleware/anymark/spaceReplace"

	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"io"
	"regexp"
	"strings"

	"github.com/lunny/html2md"
)

// DefaultParser returns a new Parser that is configured by default values.
func DefaultParser() parser.Parser {
	return parser.NewParser(parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
}

// DefaultRenderer returns a new Renderer that is configured by default values.
func DefaultRenderer() renderer.Renderer {
	return renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(), 1000)))
}

var defaultMarkdown = New()

// Convert interprets a UTF-8 bytes source in Markdown and
// write rendered contents to a writer w.
func Convert(source []byte, w io.Writer, opts ...parser.ParseOption) error {
	return defaultMarkdown.Convert(source, w, opts...)
}

// A Markdown interface offers functions to convert Markdown text to
// a desired format.
type Markdown interface {
	// Convert interprets a UTF-8 bytes source in Markdown and write rendered
	// contents to a writer w.
	Convert(source []byte, writer io.Writer, opts ...parser.ParseOption) error

	ConvertBlocks(source []byte, BR blocksUtil.RWriter, opts ...parser.ParseOption) error
	HTMLToBlocks(source []byte) (error, []*model.Block)

	// Parser returns a Parser that will be used for conversion.
	Parser() parser.Parser

	// SetParser sets a Parser to this object.
	SetParser(parser.Parser)

	// Parser returns a Renderer that will be used for conversion.
	Renderer() renderer.Renderer

	// SetRenderer sets a Renderer to this object.
	SetRenderer(renderer.Renderer)
}

// Option is a functional option type for Markdown objects.
type Option func(*markdown)

// WithExtensions adds extensions.
func WithExtensions(ext ...Extender) Option {
	return func(m *markdown) {
		m.extensions = append(m.extensions, ext...)
	}
}

// WithParser allows you to override the default parser.
func WithParser(p parser.Parser) Option {
	return func(m *markdown) {
		m.parser = p
	}
}

// WithParserOptions applies options for the parser.
func WithParserOptions(opts ...parser.Option) Option {
	return func(m *markdown) {
		m.parser.AddOptions(opts...)
	}
}

// WithRenderer allows you to override the default renderer.
func WithRenderer(r renderer.Renderer) Option {
	return func(m *markdown) {
		m.renderer = r
	}
}

// WithRendererOptions applies options for the renderer.
func WithRendererOptions(opts ...renderer.Option) Option {
	return func(m *markdown) {
		m.renderer.AddOptions(opts...)
	}
}

type markdown struct {
	parser     parser.Parser
	renderer   renderer.Renderer
	extensions []Extender
}

// New returns a new Markdown with given options.
func New(options ...Option) Markdown {
	md := &markdown{
		parser:     DefaultParser(),
		renderer:   DefaultRenderer(),
		extensions: []Extender{},
	}
	for _, opt := range options {
		opt(md)
	}
	for _, e := range md.extensions {
		e.Extend(md)
	}
	return md
}

func (m *markdown) Convert(source []byte, w io.Writer, opts ...parser.ParseOption) error {
	reader := text.NewReader(source)
	doc := m.parser.Parse(reader, opts...)

	writer := bufio.NewWriter(w)
	BR := blocksUtil.NewRWriter(writer)
	//BR := blocksUtil.ExtendWriter(writer, &rState)

	return m.renderer.Render(BR, source, doc)
}

func (m *markdown) ConvertBlocks(source []byte, BR blocksUtil.RWriter, opts ...parser.ParseOption) error {
	reader := text.NewReader(source)
	doc := m.parser.Parse(reader, opts...)

	return m.renderer.Render(BR, source, doc)
}

func (m *markdown) HTMLToBlocks(source []byte) (error, []*model.Block) {
	preprocessedSource := string(source)

	// special wiki spaces
	preprocessedSource = strings.ReplaceAll(preprocessedSource, "<span> </span>", " ")

	// Pattern: <pre> <span>\n console \n</span> <span>\n . \n</span> <span>\n log \n</span>
	reWikiCode := regexp.MustCompile(`<span[\s\S]*?>([\s\S]*?)</span>`)
	preprocessedSource = reWikiCode.ReplaceAllString(preprocessedSource, `$1`)

	md := html2md.Convert(preprocessedSource)
	md = spaceReplace.WhitespaceNormalizeString(md)

	reLinkBreaks := regexp.MustCompile(`\[[\s]*?([\s\S])[\s]*?\]\(([\s\S]*?)\)`)
	md = reLinkBreaks.ReplaceAllString(md, `[$1]($2)`)

	// Pattern: <a href> <div style=background-image:...>  </div> <a>
	reEmptyLinkText := regexp.MustCompile(`\[[\s]*?\]\(([\s\S]*?)\)`)
	md = reEmptyLinkText.ReplaceAllString(md, `[$1]($1)`)

	md = strings.ReplaceAll(md, "`", "@@@")
	reCode := regexp.MustCompile(`\n(@@@([\s\S]*?)@@@)\n`)
	md = reCode.ReplaceAllString(md, `@@@@@@@@@$2@@@@@@@@@`)
	reCodeStart := regexp.MustCompile(`@@@@@@@@@([\S]*?)`)
	md = reCodeStart.ReplaceAllString(md, "\n@@@@@@@@@\n$1")
	md = strings.ReplaceAll(md, "@@@", "`")

	fmt.Println("MD:", md)

	//md = "\n```code```\n ## 123123"

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	BR := blocksUtil.NewRWriter(writer)

	err := m.ConvertBlocks([]byte(md), BR)
	if err != nil {
		return err, nil
	}

	return nil, BR.GetBlocks()
}

func (m *markdown) Parser() parser.Parser {
	return m.parser
}

func (m *markdown) SetParser(v parser.Parser) {
	m.parser = v
}

func (m *markdown) Renderer() renderer.Renderer {
	return m.renderer
}

func (m *markdown) SetRenderer(v renderer.Renderer) {
	m.renderer = v
}

// An Extender interface is used for extending Markdown.
type Extender interface {
	// Extend extends the Markdown.
	Extend(Markdown)
}
