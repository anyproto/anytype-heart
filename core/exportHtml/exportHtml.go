package exportHtml

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/yosssi/gohtml"
	"strings"
)

/*oneof content {
	Content.Dashboard dashboard = 11;
	Content.Page page = 12;
	Content.Dataview dataview = 13;

	Content.Text text = 14;
	Content.File file = 15;
	Content.Layout layout = 16;
	Content.Div div = 17;
	Content.Bookmark bookmark = 18;
	Content.Icon icon = 19;
	Content.Link link = 20;
}*/

/*enum Style {
	Paragraph = 0;
	Header1 = 1;
	Header2 = 2;
	Header3 = 3;
	Header4 = 4;
	Quote = 5;
	Code = 6;
	Title = 7;
	Checkbox = 8;
	Marked = 9;
	Numbered = 10;
	Toggle = 11;
}*/

/*message Mark {
	Range range = 1; // range of symbols to apply this mark. From(symbol) To(symbol)
	Type type = 2;
	string param = 3; // link, color, etc

	enum Type {
		Strikethrough = 0;
		Keyboard = 1;
		Italic = 2;
		Bold = 3;
		Underscored = 4;
		Link = 5;
		TextColor = 6;
		BackgroundColor = 7;
	}
}*/


func BlocksToHtml (blocks []*model.Block) string {
	var htmlArr []string
	for _, b := range blocks {
		htmlArr = append(htmlArr, blockToHtml(b.GetContent(), b))
	}

	output := strings.Join(htmlArr, "\n")
	return gohtml.Format(wrapHtml(output))
}

func blockToHtml (content interface{}, b *model.Block) string {
	output := ""

	switch content.(type) {
		case model.BlockContentOfText: output = textBlockToHtml(b)
		case model.BlockContentOfFile: output = fileToHtml(b)
		case model.BlockContentOfBookmark: output = bookmarkToHtml(b)
		case model.BlockContentOfDiv: output = divToHtml(b)
		case model.BlockContentOfIcon: output = iconToHtml(b)
		case model.BlockContentOfLayout: output = layoutToHtml(b)
		case model.BlockContentOfDashboard: break // Impossible
		case model.BlockContentOfPage: break // Impossible
		case model.BlockContentOfDataview: break // Not implemented yet
		case model.BlockContentOfLink: break // TODO: export linked page too?
		default: break
	}

	return output
}

func wrapHtml (innerHtml string) string {
	title := "" // TODO: add title
	styles := "" // TODO: add styles
	output := `
		<!DOCTYPE html>
		<html>
			<head>
				<meta http-equiv="content-type" content="text/html; charset=utf-8" />
				<title>` + title + `</title>
				<style type="text/css">` + styles + `</style>
				<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/9.15.6/styles/github.min.css">
				<script src="https://code.jquery.com/jquery-3.4.1.min.js"></script>
				<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/9.15.6/highlight.min.js"></script>
			</head>
			<body>
				<div class="container">` + innerHtml + `</div>
			</body>
		</html>`

	return output
}

func textBlockToHtml (block *model.Block) string {
	return ""
}

func fileToHtml (block *model.Block) string {
	output := ""

	if block.GetFile().Type == model.BlockContentFile_Image {
		 // TODO: how to export an image from the IPFS? We need an export service, I guess
	}

	return output
}

func bookmarkToHtml (block *model.Block) string {
	return `
		<div class="bookmark" ${attrJoin(attr)}>
			<a href="${bookMark.url}" ${attrJoin(attr)}>
				${bookMark.title}
			</a>
		</div>`

}

func divToHtml (block *model.Block) string {
	return "<hr>"
}

func iconToHtml (block *model.Block) string {
	icon := "" // TODO: get icon
	return `<div class="smile">` + icon + `</div>`
}

func layoutToHtml (block *model.Block) string {
	return `` // TODO
}
