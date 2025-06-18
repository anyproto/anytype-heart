package anymark

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	html2md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/wikilink"
	"golang.org/x/net/html"

	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark/whitespace"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	reEmptyLinkText = regexp.MustCompile(`\[[\s]*?\]\(([\s\S]*?)\)`)
	reWikiCode      = regexp.MustCompile(`<span\s*?>(\s*?)</span>`)
	reNotionTable   = regexp.MustCompile(`(?s)(<table>.*?</table>)`)

	reWikiWbr = regexp.MustCompile(`<wbr[^>]*>`)
)

func convertBlocks(source []byte, r ...renderer.NodeRenderer) error {
	nodeRenderers := make([]util.PrioritizedValue, 0, len(r))
	for _, nodeRenderer := range r {
		nodeRenderers = append(nodeRenderers, util.Prioritized(nodeRenderer, 100))
	}
	gm := goldmark.New(goldmark.WithRenderer(
		renderer.NewRenderer(renderer.WithNodeRenderers(nodeRenderers...)),
	), goldmark.WithExtensions(extension.Table), goldmark.WithExtensions(extension.Strikethrough), goldmark.WithExtensions(&wikilink.Extender{}))
	return gm.Convert(source, &bytes.Buffer{})
}

func MarkdownToBlocks(markdownSource []byte,
	baseFilepath string,
	allFileShortPaths []string) (blocks []*model.Block, rootBlockIDs []string, err error) {
	br := newBlocksRenderer(baseFilepath, allFileShortPaths, false)

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

func escapeAll(n *html.Node) {
	if n.Type == html.TextNode {
		n.Data = Escape(n.Data)
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		escapeAll(c)
	}
}

// escapeRecursively mutates every text-node under sel (including sel itself)
func escapeRecursively(sel *goquery.Selection) {
	// operate on the direct text children of this element
	sel.Contents().Each(func(_ int, n *goquery.Selection) {
		if n.Nodes[0].Type == html.TextNode {
			text := n.Text()
			n.SetText(Escape(text))
		}
	})

	// recurse into element children
	sel.Children().Each(func(_ int, child *goquery.Selection) {
		escapeRecursively(child)
	})
}

func HTMLToBlocks(source []byte, url string) (blocks []*model.Block, rootBlockIDs []string, err error) {
	preprocessedSource := string(source)

	preprocessedSource = transformCSSUnderscore(preprocessedSource)
	// special wiki spaces
	preprocessedSource = strings.ReplaceAll(preprocessedSource, "<span> </span>", " ")
	preprocessedSource = reWikiWbr.ReplaceAllString(preprocessedSource, ``)

	// Pattern: <pre> <span>\n console \n</span> <span>\n . \n</span> <span>\n log \n</span>
	preprocessedSource = reWikiCode.ReplaceAllString(preprocessedSource, `$1`)
	preprocessedSource = reNotionTable.ReplaceAllStringFunc(preprocessedSource, func(match string) string {
		return strings.ReplaceAll(match, "\n", "")
	})

	converter := html2md.NewConverter("", true, &html2md.Options{
		DisableEscaping:  true,
		AllowHeaderBreak: true,
		EmDelimiter:      "*",
		GetAbsoluteURL: func(selec *goquery.Selection, src string, domain string) string {
			return getAbsolutePath(url, src)
		},
	})
	converter.Before(func(selec *goquery.Selection) {
		for _, n := range selec.Nodes { // the hook can hand you several roots
			escapeAll(n)
		}
	})
	converter.Use(plugin.GitHubFlavored())
	converter.AddRules(getCustomHTMLRules()...)
	md, err := converter.ConvertString(preprocessedSource)
	if err != nil {
		return nil, nil, err
	}

	md = whitespace.WhitespaceNormalizeString(md)

	md = reEmptyLinkText.ReplaceAllString(md, `[$1]($1)`)

	blRenderer := newBlocksRenderer("", nil, false)
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
			var src, title string
			if src = extractImageSource(selec); src == "" {
				return nil
			}

			title, _ = selec.Attr("alt")
			if title == "" {
				title = "image"
			}

			absolutePath := options.GetAbsoluteURL(selec, src, "")
			// if we simply return link, BlockPaste command will not recognize it as image
			return html2md.String(fmt.Sprintf("![%s](%s)", title, absolutePath))
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

func extractImageSource(selec *goquery.Selection) string {
	var (
		src string
		ok  bool
	)
	if src, ok = selec.Attr("src"); !ok || src == "" {
		if src, ok = selec.Attr("data-src"); !ok || src == "" {
			return ""
		}
	}
	return src
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

func getAbsolutePath(rawUrl, relativeSrc string) string {
	parsedUrl, err := url.Parse(rawUrl)
	if err != nil {
		return relativeSrc
	}

	// cases like //upload.com/picture.png where we should add scheme
	if strings.HasPrefix(relativeSrc, "//") {
		if parsedUrl.Scheme == "" {
			parsedUrl.Scheme = "http"
		}
		return strings.Join([]string{parsedUrl.Scheme, relativeSrc}, ":")
	}

	// cases like /static/example.png where we should add root path
	if strings.HasPrefix(relativeSrc, "/") {
		if parsedUrl.Host != "" {
			parsedUrl.Path = relativeSrc
			return parsedUrl.String()
		}
	}

	// link to section of html page
	if strings.HasPrefix(relativeSrc, "#") {
		parsedUrl.Fragment = strings.TrimLeft(relativeSrc, "#")
		return parsedUrl.String()
	}
	return relativeSrc
}
