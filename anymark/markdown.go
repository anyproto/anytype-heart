// Package goldmark implements functions to convert markdown text to a desired format.
package anymark

import (
	"bytes"
	"regexp"
	"strings"

	htmlConverter "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/anytypeio/go-anytype-middleware/anymark/renderer/html"
	"github.com/anytypeio/go-anytype-middleware/anymark/spaceReplace"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var (
	linkRegexp      = regexp.MustCompile(`\[([\s\S]*?)\]\((.*?)\)`)
	markRightEdge   = regexp.MustCompile(`([^\*\~\_\s])([\*\~\_]+)(\S)`)
	linkLeftEdge    = regexp.MustCompile(`(\S)\[`)
	reEmptyLinkText = regexp.MustCompile(`\[[\s]*?\]\(([\s\S]*?)\)`)
	reWikiCode      = regexp.MustCompile(`<span[\s\S]*?>([\s\S]*?)</span>`)

	reWikiWbr = regexp.MustCompile(`<wbr[^>]*>`)
)

// A Markdown interface offers functions to convert Markdown text to
// a desired format.
type Markdown interface {
	HTMLToBlocks(source []byte) (err error, blocks []*model.Block, rootBlockIDs []string)
	MarkdownToBlocks(markdownSource []byte, baseFilepath string, allFileShortPaths []string) (blocks []*model.Block, rootBlockIDs []string, err error)
}

type markdown struct {
}

// New returns a new Markdown with given options.
func New() Markdown {
	return &markdown{}
}

// func (m *markdown) Convert(source []byte, w io.Writer, opts ...parser.ParseOption) error {
// 	reader := text.NewReader(source)
// 	doc := m.parser.Parse(reader, opts...)
//
// 	writer := bufio.NewWriter(w)
// 	bWriter := blocksUtil.NewRWriter(writer, "", []string{})
// 	// bWriter := blocksUtil.ExtendWriter(writer, &rState)
//
// 	return m.renderer.Render(bWriter, source, doc)
// }

func (m *markdown) ConvertBlocks(source []byte, r renderer.NodeRenderer) error {
	gm := goldmark.New(goldmark.WithRenderer(
		renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(r, 1000))),
	))
	return gm.Convert(source, &bytes.Buffer{})
}

func (m *markdown) MarkdownToBlocks(markdownSource []byte, baseFilepath string, allFileShortPaths []string) (blocks []*model.Block, rootBlockIDs []string, err error) {
	w := html.NewRWriter(baseFilepath, allFileShortPaths)
	r := html.NewRenderer(w)

	// allFileShortPaths,
	err = m.ConvertBlocks(markdownSource, r)
	if err != nil {
		return nil, nil, err
	}

	return r.GetBlocks(), r.GetRootBlockIDs(), nil
}

func (m *markdown) HTMLToBlocks(source []byte) (err error, blocks []*model.Block, rootBlockIDs []string) {
	preprocessedSource := string(source)

	preprocessedSource = transformCSSUnderscore(preprocessedSource)
	// special wiki spaces
	preprocessedSource = strings.ReplaceAll(preprocessedSource, "<span> </span>", " ")
	preprocessedSource = reWikiWbr.ReplaceAllString(preprocessedSource, ``)

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
	underscore := htmlConverter.Rule{
		Filter: []string{"u", "ins"},
		Replacement: func(content string, selec *goquery.Selection, opt *htmlConverter.Options) *string {
			content = strings.TrimSpace(content)
			return htmlConverter.String("<u>" + content + "</u>")
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
	converter.AddRules(strikethrough, br, underscore)

	md, _ := converter.ConvertString(preprocessedSource)

	// md := html2md.Convert(preprocessedSource)
	md = spaceReplace.WhitespaceNormalizeString(md)
	// md = strings.ReplaceAll(md, "*  *", "* *")

	md = reEmptyLinkText.ReplaceAllString(md, `[$1]($1)`)

	w := html.NewRWriter("", nil)
	r := html.NewRenderer(w)

	err = m.ConvertBlocks([]byte(md), r)
	if err != nil {
		return err, nil, nil
	}
	return nil, r.GetBlocks(), r.GetRootBlockIDs()
}
