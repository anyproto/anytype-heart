// Package goldmark implements functions to convert markdown text to a desired format.
package anymark

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/anymark/blocksUtil"
	"github.com/anytypeio/go-anytype-middleware/anymark/renderer"
	"github.com/anytypeio/go-anytype-middleware/anymark/renderer/html"
	"github.com/anytypeio/go-anytype-middleware/anymark/spaceReplace"

	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	htmlConverter "github.com/JohannesKaufmann/html-to-markdown"
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

var (
	defaultMarkdown = New()
	linkRegexp      = regexp.MustCompile(`\[([\s\S]*?)\]\((.*?)\)`)
	markRightEdge   = regexp.MustCompile(`([^\*\~\_\s])([\*\~\_]+)(\S)`)
	linkLeftEdge    = regexp.MustCompile(`(\S)\[`)
	reEmptyLinkText = regexp.MustCompile(`\[[\s]*?\]\(([\s\S]*?)\)`)
	reWikiCode      = regexp.MustCompile(`<span[\s\S]*?>([\s\S]*?)</span>`)
)

// Convert interprets a UTF-8
//bytes source in Markdown and
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

	ConvertBlocks(source []byte, bWriter blocksUtil.RWriter, opts ...parser.ParseOption) error
	HTMLToBlocks(source []byte) (err error, blocks []*model.Block, rootBlockIDs []string)
	MarkdownToBlocks(markdownSource []byte, baseFilepath string, allFileShortPaths []string) (blocks []*model.Block, rootBlockIDs []string, err error)
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
	bWriter := blocksUtil.NewRWriter(writer, "", []string{})
	//bWriter := blocksUtil.ExtendWriter(writer, &rState)

	return m.renderer.Render(bWriter, source, doc)
}

func (m *markdown) ConvertBlocks(source []byte, bWriter blocksUtil.RWriter, opts ...parser.ParseOption) error {
	reader := text.NewReader(source)
	doc := m.parser.Parse(reader, opts...)
	return m.renderer.Render(bWriter, source, doc)
}

func (m *markdown) MarkdownToBlocks(markdownSource []byte, baseFilepath string, allFileShortPaths []string) (blocks []*model.Block, rootBlockIDs []string, err error) {
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	bWriter := blocksUtil.NewRWriter(writer, baseFilepath, allFileShortPaths)
	// allFileShortPaths,
	err = m.ConvertBlocks(markdownSource, bWriter)
	if err != nil {
		return nil, nil, err
	}

	return bWriter.GetBlocks(), bWriter.GetRootBlockIDs(), nil
}

func (m *markdown) HTMLToBlocks(source []byte) (err error, blocks []*model.Block, rootBlockIDs []string) {
	preprocessedSource := string(source)

	// special wiki spaces
	preprocessedSource = strings.ReplaceAll(preprocessedSource, "<span> </span>", " ")

	// Pattern: <pre> <span>\n console \n</span> <span>\n . \n</span> <span>\n log \n</span>
	preprocessedSource = reWikiCode.ReplaceAllString(preprocessedSource, `$1`)

	strikethrough := htmlConverter.Rule{
		Filter: []string{"span"},
		Replacement: func(content string, selec *goquery.Selection, opt *htmlConverter.Options) *string {
			// If the span element has not the classname `bb_strike` return nil.
			// That way the next rules will apply. In this case the commonmark rules.
			// -> return nil -> next rule applies
			if !selec.HasClass("bb_strike") {
				return nil
			}

			// Trim spaces so that the following does NOT happen: `~ and cake~`.
			// Because of the space it is not recognized as strikethrough.
			// -> trim spaces at begin&end of string when inside strong/italic/...
			content = strings.TrimSpace(content)
			return htmlConverter.String("~" + content + "~")
		},
	}

	br := htmlConverter.Rule{
		Filter: []string{"br"},
		Replacement: func(content string, selec *goquery.Selection, opt *htmlConverter.Options) *string {
			content = strings.TrimSpace(content)
			return htmlConverter.String("\n" + content)
		},
	}

	converter := htmlConverter.NewConverter("", true, &htmlConverter.Options{
		DisableEscaping:  true,
		AllowHeaderBreak: true,
		EmDelimiter:      "*",
	})
	converter.AddRules(strikethrough, br)

	md, _ := converter.ConvertString(preprocessedSource)

	//md := html2md.Convert(preprocessedSource)
	md = spaceReplace.WhitespaceNormalizeString(md)
	//md = strings.ReplaceAll(md, "*  *", "* *")

	md = reEmptyLinkText.ReplaceAllString(md, `[$1]($1)`)

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	bWriter := blocksUtil.NewRWriter(writer, "", []string{})

	err = m.ConvertBlocks([]byte(md), bWriter)
	if err != nil {
		return err, nil, nil
	}

	return nil, bWriter.GetBlocks(), bWriter.GetRootBlockIDs()
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
