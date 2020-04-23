// Package goldmark implements functions to convert markdown text to a desired format.
package anymark

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/PuerkitoBio/goquery"
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

	htmlConverter "github.com/anytypeio/html-to-markdown"
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

	ConvertBlocks(source []byte, bWriter blocksUtil.RWriter, opts ...parser.ParseOption) error
	HTMLToBlocks(source []byte) (error, []*model.Block)
	MarkdownToBlocks(markdownSource []byte) ([]*model.Block, error)
	DirWithMarkdownToBlocks(directoryPath string) (nameToBlock map[string][]*model.Block, isPageLinked map[string]bool, err error)
	GetRootLinks(nameToBlock map[string][]*model.Block, nameToId map[string]string, isPageLinked map[string]bool) (rootLinks []*model.Block)
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
	bWriter := blocksUtil.NewRWriter(writer)
	//bWriter := blocksUtil.ExtendWriter(writer, &rState)

	return m.renderer.Render(bWriter, source, doc)
}

func (m *markdown) ConvertBlocks(source []byte, bWriter blocksUtil.RWriter, opts ...parser.ParseOption) error {
	reader := text.NewReader(source)
	doc := m.parser.Parse(reader, opts...)

	return m.renderer.Render(bWriter, source, doc)
}

func (m *markdown) DirWithMarkdownToBlocks(directoryPath string) (nameToBlock map[string][]*model.Block, isPageLinked map[string]bool, err error) {
	nameToBlocks := make(map[string][]*model.Block)
	linkRegexp := regexp.MustCompile(`\[([\s\S]*?)\]\((.*?)\)`)

	allFileShortPaths := []string{}

	err = filepath.Walk(directoryPath,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() {
				shortPath := strings.Replace(path, directoryPath+"/", "", -1)
				allFileShortPaths = append(allFileShortPaths, shortPath)
			}

			return nil
		})

	if err != nil {
		return nameToBlocks, isPageLinked, err
	}

	err = filepath.Walk(directoryPath,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() {
				extension := filepath.Ext(path)

				dat, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}

				// TODO: media
				if extension == ".png" {
				}

				if extension == ".md" {
					datStr := string(dat)
					linkSubmatches := linkRegexp.FindAllStringSubmatch(datStr, -1)

					shortPath := strings.Replace(path, directoryPath+"/", "", -1)

					for _, linkSubmatch := range linkSubmatches {
						l := strings.Replace(linkSubmatch[2], "%20", " ", -1)

						for _, sPath := range allFileShortPaths {
							// TODO: strings.Contains(l, ".md"
							if strings.Contains(sPath, l) && strings.Contains(l, ".md") {

								datStr = strings.Replace(datStr, linkSubmatch[2], strings.Replace(sPath, " ", "%20", -1), -1)
							}
						}
					}

					nameToBlocks[shortPath], err = m.MarkdownToBlocks([]byte(datStr))
					if err != nil {
						return err
					}
				}

			}

			return nil
		})

	isPageLinked = make(map[string]bool)
	for name, _ := range nameToBlocks {
		for i, block := range nameToBlocks[name] {
			nameToBlocks[name][i].Id = uuid.New().String()

			txt := block.GetText()
			if txt != nil && txt.Marks != nil && len(txt.Marks.Marks) == 1 &&
				txt.Marks.Marks[0].Type == model.BlockContentTextMark_Link {

				linkConverted := strings.Replace(txt.Marks.Marks[0].Param, "%20", " ", -1)

				fmt.Println("LINK:", name, linkConverted, len(nameToBlocks[linkConverted]))

				if nameToBlocks[linkConverted] != nil {
					fmt.Println("@@@ CONVERT:", nameToBlocks[name])
					nameToBlocks[name][i], isPageLinked = m.convertTextToPageLink(block, isPageLinked)
				}
			}

		}
	}

	return nameToBlocks, isPageLinked, err
}

func (m *markdown) GetRootLinks(nameToBlock map[string][]*model.Block, nameToId map[string]string, isPageLinked map[string]bool) (rootLinks []*model.Block) {
	for name := range nameToBlock {
		if !isPageLinked[name] {
			rootLinks = append(rootLinks, &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: nameToId[name],
						Style:         model.BlockContentLink_Page,
						Fields:        nil,
					},
				},
			})
		}
	}

	return rootLinks
}

func (m *markdown) convertTextToPageLink(block *model.Block, isPageLinked map[string]bool) (*model.Block, map[string]bool) {

	targetId := strings.Replace(block.GetText().Marks.Marks[0].Param, "%20", " ", -1)
	blockOut := &model.Block{
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: targetId,
				Style:         model.BlockContentLink_Page,
			},
		},
	}

	isPageLinked[targetId] = true
	return blockOut, isPageLinked
}

func (m *markdown) MarkdownToBlocks(markdownSource []byte) ([]*model.Block, error) {
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	bWriter := blocksUtil.NewRWriter(writer)

	err := m.ConvertBlocks(markdownSource, bWriter)
	if err != nil {
		return nil, err
	}

	return bWriter.GetBlocks(), nil
}

func (m *markdown) HTMLToBlocks(source []byte) (error, []*model.Block) {
	preprocessedSource := string(source)

	// special wiki spaces
	preprocessedSource = strings.ReplaceAll(preprocessedSource, "<span> </span>", " ")

	// Pattern: <pre> <span>\n console \n</span> <span>\n . \n</span> <span>\n log \n</span>
	reWikiCode := regexp.MustCompile(`<span[\s\S]*?>([\s\S]*?)</span>`)
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

	italic := htmlConverter.Rule{
		Filter: []string{"i"},
		Replacement: func(content string, selec *goquery.Selection, opt *htmlConverter.Options) *string {
			content = strings.TrimSpace(content)
			return htmlConverter.String(" *" + content + "* ")
		},
	}

	br := htmlConverter.Rule{
		Filter: []string{"br"},
		Replacement: func(content string, selec *goquery.Selection, opt *htmlConverter.Options) *string {
			content = strings.TrimSpace(content)
			return htmlConverter.String("\n" + content)
		},
	}

	converter := htmlConverter.NewConverter("", true, nil)
	converter.AddRules(strikethrough, italic, br)

	md, _ := converter.ConvertString(preprocessedSource)

	//md := html2md.Convert(preprocessedSource)
	md = spaceReplace.WhitespaceNormalizeString(md)
	//md = strings.ReplaceAll(md, "*  *", "* *")

	reCode := regexp.MustCompile(`[ ]+`)
	md = reCode.ReplaceAllString(md, ` `)

	reEmptyLinkText := regexp.MustCompile(`\[[\s]*?\]\(([\s\S]*?)\)`)
	md = reEmptyLinkText.ReplaceAllString(md, `[$1]($1)`)

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	bWriter := blocksUtil.NewRWriter(writer)

	err := m.ConvertBlocks([]byte(md), bWriter)
	if err != nil {
		return err, nil
	}

	return nil, bWriter.GetBlocks()
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
