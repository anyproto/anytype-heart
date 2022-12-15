package anymark

import (
	"bytes"
	"regexp"
	"strings"

	htmlconverter "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/anytypeio/go-anytype-middleware/anymark/whitespace"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var (
	reEmptyLinkText = regexp.MustCompile(`\[[\s]*?\]\(([\s\S]*?)\)`)
	reWikiCode      = regexp.MustCompile(`<span[\s\S]*?>([\s\S]*?)</span>`)

	reWikiWbr = regexp.MustCompile(`<wbr[^>]*>`)
)

// A Markdown interface offers functions to convert Markdown text to
// a desired format.
type Markdown interface {
	HTMLToBlocks(source []byte) (blocks []*model.Block, rootBlockIDs []string, err error)
	MarkdownToBlocks(markdownSource []byte, baseFilepath string, allFileShortPaths []string) (blocks []*model.Block, rootBlockIDs []string, err error)
}

type markdown struct {
}

// New returns a new Markdown with given options.
func New() Markdown {
	return &markdown{}
}

func (m *markdown) convertBlocks(source []byte, r renderer.NodeRenderer) error {
	gm := goldmark.New(goldmark.WithRenderer(
		renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(r, 1000))),
	))
	return gm.Convert(source, &bytes.Buffer{})
}

func (m *markdown) MarkdownToBlocks(markdownSource []byte, baseFilepath string, allFileShortPaths []string) (blocks []*model.Block, rootBlockIDs []string, err error) {
	r := NewRenderer(baseFilepath, allFileShortPaths)

	// allFileShortPaths,
	err = m.convertBlocks(markdownSource, r)
	if err != nil {
		return nil, nil, err
	}

	return r.GetBlocks(), r.GetRootBlockIDs(), nil
}

func (m *markdown) HTMLToBlocks(source []byte) (blocks []*model.Block, rootBlockIDs []string, err error) {
	preprocessedSource := string(source)

	preprocessedSource = transformCSSUnderscore(preprocessedSource)
	// special wiki spaces
	preprocessedSource = strings.ReplaceAll(preprocessedSource, "<span> </span>", " ")
	preprocessedSource = reWikiWbr.ReplaceAllString(preprocessedSource, ``)

	// Pattern: <pre> <span>\n console \n</span> <span>\n . \n</span> <span>\n log \n</span>
	preprocessedSource = reWikiCode.ReplaceAllString(preprocessedSource, `$1`)

	strikethrough := htmlconverter.Rule{
		Filter: []string{"span"},
		Replacement: func(content string, selec *goquery.Selection, opt *htmlconverter.Options) *string {
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
			return htmlconverter.String("~" + content + "~")
		},
	}
	underscore := htmlconverter.Rule{
		Filter: []string{"u", "ins"},
		Replacement: func(content string, selec *goquery.Selection, opt *htmlconverter.Options) *string {
			content = strings.TrimSpace(content)
			return htmlconverter.String("<u>" + content + "</u>")
		},
	}

	br := htmlconverter.Rule{
		Filter: []string{"br"},
		Replacement: func(content string, selec *goquery.Selection, opt *htmlconverter.Options) *string {
			content = strings.TrimSpace(content)
			return htmlconverter.String("\n" + content)
		},
	}

	converter := htmlconverter.NewConverter("", true, &htmlconverter.Options{
		DisableEscaping:  true,
		AllowHeaderBreak: true,
		EmDelimiter:      "*",
	})
	converter.AddRules(strikethrough, br, underscore)

	md, _ := converter.ConvertString(preprocessedSource)

	// md := html2md.Convert(preprocessedSource)
	md = whitespace.WhitespaceNormalizeString(md)
	// md = strings.ReplaceAll(md, "*  *", "* *")

	md = reEmptyLinkText.ReplaceAllString(md, `[$1]($1)`)

	r := NewRenderer("", nil)
	err = m.convertBlocks([]byte(md), r)
	if err != nil {
		return nil, nil, err
	}
	return r.GetBlocks(), r.GetRootBlockIDs(), nil
}
