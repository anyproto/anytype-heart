package anymark

import (
	"bytes"
	"regexp"
	"strings"

	html2md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark/whitespace"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	reEmptyLinkText = regexp.MustCompile(`\[[\s]*?\]\(([\s\S]*?)\)`)
	reWikiCode      = regexp.MustCompile(`<span\s*?>(\s*?)</span>`)

	reWikiWbr = regexp.MustCompile(`<wbr[^>]*>`)
)

func convertBlocks(source []byte, r ...renderer.NodeRenderer) error {
	nodeRenderers := make([]util.PrioritizedValue, 0, len(r))
	for _, nodeRenderer := range r {
		nodeRenderers = append(nodeRenderers, util.Prioritized(nodeRenderer, 100))
	}
	gm := goldmark.New(goldmark.WithRenderer(
		renderer.NewRenderer(renderer.WithNodeRenderers(nodeRenderers...)),
	), goldmark.WithExtensions(extension.Table), goldmark.WithExtensions(extension.Strikethrough))
	return gm.Convert(source, &bytes.Buffer{})
}

func MarkdownToBlocks(markdownSource []byte,
	baseFilepath string,
	allFileShortPaths []string) (blocks []*model.Block, rootBlockIDs []string, err error) {
	br := newBlocksRenderer(baseFilepath, allFileShortPaths)

	r := NewRenderer(br)

	te := table.NewEditor(nil)
	tr := NewTableRenderer(br, te)
	// allFileShortPaths,
	err = convertBlocks(markdownSource, r, tr)
	if err != nil {
		return nil, nil, err
	}

	return r.GetBlocks(), r.GetRootBlockIDs(), nil
}

func HTMLToBlocks(source []byte) (blocks []*model.Block, rootBlockIDs []string, err error) {
	preprocessedSource := string(source)

	preprocessedSource = transformCSSUnderscore(preprocessedSource)
	// special wiki spaces
	preprocessedSource = strings.ReplaceAll(preprocessedSource, "<span> </span>", " ")
	preprocessedSource = reWikiWbr.ReplaceAllString(preprocessedSource, ``)

	// Pattern: <pre> <span>\n console \n</span> <span>\n . \n</span> <span>\n log \n</span>
	preprocessedSource = reWikiCode.ReplaceAllString(preprocessedSource, `$1`)

	converter := html2md.NewConverter("", true, &html2md.Options{
		DisableEscaping:  true,
		AllowHeaderBreak: true,
		EmDelimiter:      "*",
	})
	converter.Use(plugin.GitHubFlavored())
	converter.AddRules(getCustomHTMLRules()...)
	md, err := converter.ConvertString(preprocessedSource)
	if err != nil {
		return nil, nil, err
	}

	md = whitespace.WhitespaceNormalizeString(md)

	md = reEmptyLinkText.ReplaceAllString(md, `[$1]($1)`)

	blRenderer := newBlocksRenderer("", nil)
	r := NewRenderer(blRenderer)
	tr := NewTableRenderer(blRenderer, table.NewEditor(nil))
	err = convertBlocks([]byte(md), r, tr)
	if err != nil {
		return nil, nil, err
	}
	return r.GetBlocks(), r.GetRootBlockIDs(), nil
}

func getCustomHTMLRules() []html2md.Rule {
	span := html2md.Rule{
		Filter: []string{"span"},
		Replacement: func(content string, selec *goquery.Selection, opt *html2md.Options) *string {
			return html2md.String(content)
		},
	}

	del := html2md.Rule{
		Filter: []string{"del"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			content = strings.TrimSpace(content)
			return html2md.String("~~" + content + "~~")
		},
	}

	underscore := html2md.Rule{
		Filter: []string{"u", "ins", "abbr"},
		Replacement: func(content string, selec *goquery.Selection, opt *html2md.Options) *string {
			content = strings.TrimSpace(content)
			return html2md.String("<u>" + content + "</u>")
		},
	}

	br := html2md.Rule{
		Filter: []string{"br"},
		Replacement: func(content string, selec *goquery.Selection, opt *html2md.Options) *string {
			content = strings.TrimSpace(content)
			return html2md.String("\n" + content)
		},
	}

	anohref := html2md.Rule{
		Filter: []string{"a"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			content = strings.ReplaceAll(content, `\`, ``)
			if _, exists := selec.Attr("href"); exists {
				return nil
			}
			return html2md.String(content)
		},
	}

	simpleText := html2md.Rule{
		Filter: []string{"small", "sub", "sup", "caption"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			return html2md.String(content)
		},
	}

	blockquote := html2md.Rule{
		Filter: []string{"blockquote", "q"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			return html2md.String("> " + strings.TrimSpace(content))
		},
	}

	italic := html2md.Rule{
		Filter: []string{"cite", "dfn", "address"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			return html2md.String("*" + strings.TrimSpace(content) + "*")
		},
	}

	code := html2md.Rule{
		Filter: []string{"samp", "var"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			return html2md.String("`" + content + "`")
		},
	}

	bdo := html2md.Rule{
		Filter: []string{"bdo"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			runes := []rune(content)
			for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
				runes[i], runes[j] = runes[j], runes[i]
			}
			return html2md.String(string(runes))
		},
	}

	div := html2md.Rule{
		Filter: []string{"hr"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			return html2md.String("___")
		},
	}

	img := html2md.Rule{
		Filter: []string{"img"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			var (
				src, title string
				ok         bool
			)
			if src, ok = selec.Attr("src"); !ok {
				return nil
			}

			title, _ = selec.Attr("alt")

			if title != "" {
				return html2md.String("![" + title + "]" + "(" + src + ")")
			}
			return html2md.String(src)
		},
	}

	// Add header row to table to support tables without headers, because markdown doesn't parse tables without headers
	table := html2md.Rule{
		Filter: []string{"table"},
		Replacement: func(content string, selec *goquery.Selection, options *html2md.Options) *string {
			node := selec.Children()
			hasHeader, numberOfRows, numberOfCells := calculateTotalCellsAndRows(node)
			if hasHeader {
				return html2md.String(content)
			}

			if numberOfRows == 0 {
				return nil
			}
			headerRow := addHeaderRow(content, numberOfCells, numberOfRows)
			return html2md.String(headerRow)
		},
	}

	return []html2md.Rule{span, del, underscore, br, anohref,
		simpleText, blockquote, italic, code, bdo, div, img, table}
}

func addHeaderRow(content string, numberOfCells int, numberOfRows int) string {
	numberOfColumns := numberOfCells / numberOfRows

	headerRow := "|"
	for i := 0; i < numberOfColumns; i++ {
		headerRow += " |"
	}
	headerRow += "\n|"
	for i := 0; i < numberOfColumns; i++ {
		headerRow += " --- |"
	}
	headerRow += content
	return headerRow
}

func calculateTotalCellsAndRows(node *goquery.Selection) (bool, int, int) {
	var (
		isContinue                  = true
		hasHeader                   = false
		numberOfRows, numberOfCells int
	)
	for {
		if isContinue {
			if hasHeader, isContinue = isHeadingRow(node); hasHeader {
				break
			}
		}
		if len(node.Nodes) == 0 {
			break
		}
		node.Each(func(i int, s *goquery.Selection) {
			nodeName := goquery.NodeName(s)
			if nodeName == "tr" {
				numberOfRows++
			}
			if nodeName == "td" || nodeName == "th" {
				numberOfCells++
			}
		})
		node = node.Children()
	}
	return hasHeader, numberOfRows, numberOfCells
}

func isHeadingRow(s *goquery.Selection) (bool, bool) {
	parent := s.Parent()

	if goquery.NodeName(parent) == "thead" {
		return true, false
	}

	var (
		everyTH    = false
		isContinue = true
	)

	s.Children().Each(func(i int, s *goquery.Selection) {
		if isContinue {
			if goquery.NodeName(s) == "th" && goquery.NodeName(s.Next()) == "th" {
				everyTH = true
				isContinue = false
				return
			}
			if goquery.NodeName(s) != "th" {
				everyTH = false
			}
		}
	})

	if parent.Children().First().IsSelection(s) && everyTH {
		return true, false
	}

	return false, isContinue
}
