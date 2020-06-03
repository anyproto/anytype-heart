package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	htmlConverter "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

func main() {
	var buf bytes.Buffer
	markdown := goldmark.New(
		goldmark.WithExtensions(extension.Strikethrough),
		goldmark.WithRendererOptions(
			html.WithXHTML(),
			html.WithUnsafe(),
		))

	if err := markdown.Convert([]byte(`
## A Nested List

List can be nested (lists inside lists):

- Coffee
- Tea
     - Black tea
    - Green tea
- Milk
`), &buf); err != nil {
		panic(err)
	}

	fmt.Println(string(buf.Bytes()))
}

func main3() {
	preprocessedSource := `
<html><head>/head><body>

<h2>A Nested List</h2>
<p><g class="gr_ gr_10 gr-alert gr_gramm gr_inline_cards gr_disable_anim_appear Grammar only-ins replaceWithoutSep" id="10" data-gr-id="10">List</g> can be nested (lists inside lists):</p>

<ul>
  <li>Coffee</li>
  <li>Tea
	<ul>
      <li>Black tea</li>
      <li>Green tea</li>
    </ul>
  </li>
  <li>Milk</li>
</ul>

</body></html>
`
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
	//converter.AddRules(strikethrough, italic, br)
	_, _, _ = strikethrough, italic, br

	md, _ := converter.ConvertString(preprocessedSource)

	fmt.Println(string(md))
}
