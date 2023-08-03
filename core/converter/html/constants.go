package html

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

const (
	wrapCopyStart = `<html>
		<head>
			<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
			<meta http-equiv="Content-Style-Type" content="text/css">
			<title></title>
			<meta name="Generator" content="Cocoa HTML Writer">
			<meta name="CocoaVersion" content="1894.1">
			<style type="text/css">
				.row > * { display: flex; }
				.header1 { padding: 23px 0px 1px 0px; font-size: 28px; line-height: 32px; letter-spacing: -0.36px; font-weight: 600; }
				.header2 { padding: 15px 0px 1px 0px; font-size: 22px; line-height: 28px; letter-spacing: -0.16px; font-weight: 600; }
				.header3 { padding: 15px 0px 1px 0px; font-size: 17px; line-height: 24px; font-weight: 600; }
				.quote { padding: 7px 0px 7px 0px; font-size: 18px; line-height: 26px; font-style: italic; }
				.paragraph { font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word; }
				.callout-image { width: 20px; height: 20px; font-size: 16px; line-height: 20px; margin-right: 6px; display: inline-block; }
				.callout-image img { width: 100%; object-fit: cover; }
				a { cursor: pointer; }
				kbd { display: inline; font-family: 'Mono'; line-height: 1.71; background: rgba(247,245,240,0.5); padding: 0px 4px; border-radius: 2px; }
				ul { margin: 0px; }
			</style>
		</head>
		<body>`
	wrapCopyEnd = `</body>
	</html>`
	wrapExportStart = `
	<!DOCTYPE html>
		<html>
			<head>
				<meta http-equiv="content-type" content="text/html; charset=utf-8" />
				<title></title>
				<style type="text/css"></style>
				<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/9.15.6/styles/github.min.css">
				<script src="https://code.jquery.com/jquery-3.4.1.min.js"></script>
				<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/9.15.6/highlight.min.js"></script>
			</head>
			<body>
				<div class="anytype-container">`
	wrapExportEnd = `</div>
			</body>
		</html>`

	styleParagraph = "font-size: 15px; line-height: 24px; letter-spacing: -0.08px; font-weight: 400; word-wrap: break-word;"
	styleHeader1   = "padding: 23px 0px 1px 0px; font-size: 28px; line-height: 32px; letter-spacing: -0.36px; font-weight: 600;"
	styleHeader2   = "padding: 15px 0px 1px 0px; font-size: 22px; line-height: 28px; letter-spacing: -0.16px; font-weight: 600;"
	styleHeader3   = "padding: 15px 0px 1px 0px; font-size: 17px; line-height: 24px; font-weight: 600;"
	styleHeader4   = ""
	styleQuote     = "padding: 7px 0px 7px 0px; font-size: 18px; line-height: 26px; font-style: italic;"
	styleCode      = "font-size:15px; font-family: monospace;"
	styleTitle     = ""
	styleCheckbox  = "font-size:15px;"
	styleToggle    = "font-size:15px;"
	styleKbd       = "display: inline; font-family: 'Mono'; line-height: 1.71; background: rgba(247,245,240,0.5); padding: 0px 4px; border-radius: 2px;"
	styleCallout   = "background: #f3f2ec; border-radius: 6px; padding: 16px; margin: 6px 0px;"

	defaultStyle = -1
)

type styleTag struct {
	OpenTag, CloseTag string
}

var styleTags = map[model.BlockContentTextStyle]styleTag{
	model.BlockContentText_Header1:  {OpenTag: `<h1 style="` + styleHeader1 + `">`, CloseTag: `</h1>`},
	model.BlockContentText_Header2:  {OpenTag: `<h2 style="` + styleHeader2 + `">`, CloseTag: `</h2>`},
	model.BlockContentText_Header3:  {OpenTag: `<h3 style="` + styleHeader3 + `">`, CloseTag: `</h3>`},
	model.BlockContentText_Header4:  {OpenTag: `<h4 style="` + styleHeader4 + `">`, CloseTag: `</h4>`},
	model.BlockContentText_Quote:    {OpenTag: `<quote style="` + styleQuote + `">`, CloseTag: `</quote>`},
	model.BlockContentText_Code:     {OpenTag: `<code style="` + styleCode + `"><pre>`, CloseTag: `</pre></code>`},
	model.BlockContentText_Title:    {OpenTag: `<h1 style="` + styleTitle + `">`, CloseTag: `</h1>`},
	model.BlockContentText_Checkbox: {OpenTag: `<div style="` + styleCheckbox + `" class="check"><input type="checkbox"/>`, CloseTag: `</div>`},
	model.BlockContentText_Toggle:   {OpenTag: `<div style="` + styleToggle + `" class="toggle">`, CloseTag: `</div>`},
	defaultStyle:                    {OpenTag: `<div style="` + styleParagraph + `" class="paragraph" style="` + styleParagraph + `">`, CloseTag: `</div>`},
}

func textColor(color string) string {
	switch color {
	case "grey":
		return "#aca996"
	case "yellow":
		return "#ecd91b"
	case "orange":
		return "#ffb522"
	case "red":
		return "#f55522"
	case "pink":
		return "#e51ca0"
	case "purple":
		return "#ab50cc"
	case "blue":
		return "#3e58"
	case "ice":
		return "#2aa7ee"
	case "teal":
		return "#0fc8ba"
	case "lime":
		return "#5dd400"
	case "black":
		return "#2c2b27"
	default:
		return color
	}
}

func backgroundColor(color string) string {
	switch color {
	case "grey":
		return "#f3f2ec"
	case "yellow":
		return "#fef9cc"
	case "orange":
		return "#fef3c5"
	case "red":
		return "#ffebe5"
	case "pink":
		return "#fee3f5"
	case "purple":
		return "#f4e3fa"
	case "blue":
		return "#f4e3fa"
	case "ice":
		return "#d6effd"
	case "teal":
		return "#d6f5f3"
	case "lime":
		return "#e3f7d0"
	default:
		return color
	}
}
