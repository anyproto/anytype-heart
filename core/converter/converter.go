package converter

import (
	"fmt"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/types"
	"github.com/yosssi/gohtml"
)


type Node struct {
	id     string
	model  *model.Block
	children []*Node
}

type converter struct {
	nodeTable map[string]*Node
	rootNode *Node
	remainBlocks []*model.Block
}

type Converter interface {
	CreateTree (blocks []*model.Block) Node
	ProcessTree (node *Node) (out string)
	PrintNode (node *Node) (out string)
	Convert (blocks []*model.Block) (out string)

	nextTreeLayer (node *Node) (processedNode *Node)
	filterById (blocks []*model.Block, id string) (out []*model.Block)
}


func New() Converter {
	c := &converter{
		rootNode: &Node{id: "root"},
		nodeTable: map[string]*Node{},
		remainBlocks: []*model.Block{},
	}

	return c
}


func (c *converter) filterById (blocks []*model.Block, id string) (out []*model.Block) {
	for _, b := range blocks {
		if b.Id != id {
			out = append(out, b)
		}
	}
	return out
}

func (c *converter) CreateTree (blocks []*model.Block) Node {

	// 1. Create map
	for _, b := range blocks {
		c.nodeTable[b.Id] = &Node{b.Id, b,[]*Node{} }
	}

	// 2. Fill children field
	for _, b := range blocks {
		for _, child := range b.ChildrenIds {
			c.nodeTable[b.Id].children = append(c.nodeTable[b.Id].children, c.nodeTable[child])
		}
	}

	// 3. get root level blocks
	blocksRootLvl := blocks
	for _, b := range blocks {

		for _, child := range b.ChildrenIds {
			blocksRootLvl = c.filterById(blocksRootLvl, child)
		}
	}

	fmt.Println("ROOT LEVEL:", blocksRootLvl)

	// 4. Create root
	c.rootNode.model = &model.Block{ChildrenIds:[]string{}}

	c.remainBlocks = blocks

	// 5. Set top level blocks to root model
	for _, br := range blocksRootLvl {
		c.rootNode.model.ChildrenIds = append(c.rootNode.model.ChildrenIds, br.Id)
		c.remainBlocks = c.filterById(c.remainBlocks, br.Id)
	}

	fmt.Println("ROOT NODE BEFORE:", c.rootNode)
	c.rootNode = c.nextTreeLayer(c.rootNode)

	fmt.Println("ROOT NODE AFTER:", c.rootNode)

	return *c.rootNode
}

func contains(s []*Node, e *Node) bool {
	for _, a := range s {
		if a.id == e.id {
			return true
		}
	}

	return false
}

func (c *converter) nextTreeLayer (node *Node) *Node {
	c.remainBlocks = c.filterById(c.remainBlocks, node.id)

	for _, childId := range node.model.ChildrenIds {
		c.remainBlocks = c.filterById(c.remainBlocks, childId)

		if  c.nodeTable[childId] != nil &&
			c.nodeTable[childId].model !=nil &&
			c.nodeTable[childId].model.ChildrenIds != nil &&
			len(c.nodeTable[childId].model.ChildrenIds) > 0 {
			c.nodeTable[childId] = c.nextTreeLayer(c.nodeTable[childId])
		}

		if !contains(node.children, c.nodeTable[childId]) {
			node.children = append(node.children, c.nodeTable[childId])
		}
	}

	return node
}

// For test purposes
func (c *converter) PrintNode (node *Node) (out string) {
	for _, child := range node.children {
		out += "<node>" + child.id
		if len(child.children) > 0 {
			out += c.PrintNode(child)
		}

		out += "</node>"
	}

	return "\n" + gohtml.Format(out)
}

func (c *converter) ProcessTree (node *Node) (out string) {
	for _, child := range node.children {

		switch  cont := child.model.Content.(type) {
		case *model.BlockContentOfText:     out += renderText(true, cont)
		case *model.BlockContentOfFile:     out += renderFile(true, cont)
		case *model.BlockContentOfBookmark: out += renderBookmark(true, cont)
		case *model.BlockContentOfDiv:      out += renderDiv(true, cont)
		case *model.BlockContentOfIcon:     out += renderIcon(true, cont)
		case *model.BlockContentOfLayout:   out += renderLayout(true, cont, child.model)
		case *model.BlockContentOfDashboard: break;
		case *model.BlockContentOfPage: break;
		case *model.BlockContentOfDataview: break;
		case *model.BlockContentOfLink: break;
		}

		if len(child.children) > 0 {
			out += c.ProcessTree(child)
		}

		switch cont := child.model.Content.(type) {
		case *model.BlockContentOfText:     out += renderText(false, cont)
		case *model.BlockContentOfFile:     out += renderFile(false, cont)
		case *model.BlockContentOfBookmark: out += renderBookmark(false, cont)
		case *model.BlockContentOfDiv:      out += renderDiv(false, cont)
		case *model.BlockContentOfIcon:     out += renderIcon(false, cont)
		case *model.BlockContentOfLayout:   out += renderLayout(false, cont, child.model)
		case *model.BlockContentOfDashboard: break;
		case *model.BlockContentOfPage: break;
		case *model.BlockContentOfDataview: break;
		case *model.BlockContentOfLink: break;
		}
	}

	return out
}

func colorMapping (color string, isText bool) (out string) {
	if isText {
		switch color {
		case "grey":  out = "#aca996"
		case "yellow":  out = "#ecd91b"
		case "orange":  out = "#ffb522"
		case "red":  out = "#f55522"
		case "pink":  out = "#e51ca0"
		case "purple":  out = "#ab50cc"
		case "blue":  out = "#3e58"
		case "ice":  out = "#2aa7ee"
		case "teal":  out = "#0fc8ba"
		case "lime":  out = "#5dd400"
		case "black": out = "#2c2b27"
		default: out = color
		}
	} else {
		switch color {
		case "grey":  out = "#f3f2ec"
		case "yellow":  out = "#fef9cc"
		case "orange":  out = "#fef3c5"
		case "red":  out = "#ffebe5"
		case "pink":  out = "#fee3f5"
		case "purple":  out = "#f4e3fa"
		case "blue":  out = "#f4e3fa"
		case "ice":  out = "#d6effd"
		case "teal":  out = "#d6f5f3"
		case "lime":  out = "#e3f7d0"
		default: out = color
		}
	}

	return out
}

func applyMarks (text string, marks *model.BlockContentTextMarks) (out string) {
	if len(text) == 0 ||
		marks == nil ||
		marks.Marks == nil ||
		len(marks.Marks) == 0 {
		return text
	}

	var symbols []string

	r := []rune(text)
	if len(r) != len(text) {
		return text
	}

	for i := 0; i < len(text); i++ {
		symbols = append(symbols, string(r[i]))
	}

	for i := 0; i < len(marks.Marks); i++ {
		if len(symbols) > int(marks.Marks[i].Range.From) && len(symbols) > int(marks.Marks[i].Range.To - 1) {

			switch marks.Marks[i].Type {
			case model.BlockContentTextMark_Strikethrough:
				symbols[marks.Marks[i].Range.From] = "<s>" + symbols[marks.Marks[i].Range.From]
				symbols[marks.Marks[i].Range.To-1] = symbols[marks.Marks[i].Range.To-1] + "</s>"
			case model.BlockContentTextMark_Keyboard:
				symbols[marks.Marks[i].Range.From] = "<kbd>" + symbols[marks.Marks[i].Range.From]
				symbols[marks.Marks[i].Range.To-1] = symbols[marks.Marks[i].Range.To-1] + "</kbd>"
			case model.BlockContentTextMark_Italic:
				symbols[marks.Marks[i].Range.From] = "<i>" + symbols[marks.Marks[i].Range.From]
				symbols[marks.Marks[i].Range.To-1] = symbols[marks.Marks[i].Range.To-1] + "</i>"
			case model.BlockContentTextMark_Bold:
				symbols[marks.Marks[i].Range.From] = "<b>" + symbols[marks.Marks[i].Range.From]
				symbols[marks.Marks[i].Range.To-1] = symbols[marks.Marks[i].Range.To-1] + "</b>"
			case model.BlockContentTextMark_Link:
				symbols[marks.Marks[i].Range.From] = `<a href="` + marks.Marks[i].Param + `">` + symbols[marks.Marks[i].Range.From]
				symbols[marks.Marks[i].Range.To-1] = symbols[marks.Marks[i].Range.To-1] + "</a>"
			case model.BlockContentTextMark_TextColor:
				symbols[marks.Marks[i].Range.From] = `<span style="color:` + colorMapping(marks.Marks[i].Param, true) + `">` + symbols[marks.Marks[i].Range.From]
				symbols[marks.Marks[i].Range.To-1] = symbols[marks.Marks[i].Range.To-1] + "</span>"
			case model.BlockContentTextMark_BackgroundColor:
				symbols[marks.Marks[i].Range.From] = `<span style="background-color:` + colorMapping(marks.Marks[i].Param, false) + `">` + symbols[marks.Marks[i].Range.From]
				symbols[marks.Marks[i].Range.To-1] = symbols[marks.Marks[i].Range.To-1] + "</span>"
			}
		}
	}

	for i := 0; i < len(symbols); i++ {
		out = out + symbols[i]
	}

	return out
}

func renderText(isOpened bool, child *model.BlockContentOfText) (out string) {

	styleParagraph := "font-size:15px;"
	styleHeader1 := ""
	styleHeader2 := ""
	styleHeader3 := ""
	styleHeader4 := ""
	styleQuote := "font-size:15px; font-style: italic;"
	styleCode := "font-size:15px; font-family: monospace;"
	styleTitle := ""
	styleCheckbox := "font-size:15px;"
	styleMarked := "font-size:15px;"
	styleNumbered := "font-size:15px;"
	styleToggle := "font-size:15px;"

	if isOpened {
		switch child.Text.Style {
		// TODO: renderText -> c.renderText; ul li, ul li -> ul li li
		case model.BlockContentText_Paragraph: out += `<div style="` + styleParagraph + `" class="paragraph" style="` + styleParagraph + `">` + applyMarks(child.Text.Text, child.Text.Marks)
		case model.BlockContentText_Header1:   out += `<h1 style="` + styleHeader1 + `">` + child.Text.Text
		case model.BlockContentText_Header2:   out += `<h2 style="` + styleHeader2 + `">` + child.Text.Text
		case model.BlockContentText_Header3:   out += `<h3 style="` + styleHeader3 + `">` + child.Text.Text
		case model.BlockContentText_Header4:   out += `<h4 style="` + styleHeader4 + `">` + child.Text.Text
		case model.BlockContentText_Quote:     out += `<quote style="` + styleQuote + `">` + applyMarks(child.Text.Text, child.Text.Marks)
		case model.BlockContentText_Code:      out += `<code style="` + styleCode + `">` + applyMarks(child.Text.Text, child.Text.Marks)
		case model.BlockContentText_Title:     out += `<h1 style="` + styleTitle + `">` + applyMarks(child.Text.Text, child.Text.Marks)
		case model.BlockContentText_Checkbox:  out += `<div style="` + styleCheckbox + `" class="check"><input type="checkbox"/>` + applyMarks(child.Text.Text, child.Text.Marks)
		case model.BlockContentText_Marked:    out += `<ul style="` + styleMarked + `"><li>` + applyMarks(child.Text.Text, child.Text.Marks)
		case model.BlockContentText_Numbered:  out += `<ol style="` + styleNumbered + `"><li>` + applyMarks(child.Text.Text, child.Text.Marks)
		case model.BlockContentText_Toggle:    out += `<div style="` + styleToggle + `" class="toggle">` + applyMarks(child.Text.Text, child.Text.Marks)
		}
	} else {
		switch child.Text.Style {
		case model.BlockContentText_Paragraph: out += "</div>"
		case model.BlockContentText_Header1:   out += "</h1>"
		case model.BlockContentText_Header2:   out += "</h2>"
		case model.BlockContentText_Header3:   out += "</h3>"
		case model.BlockContentText_Header4:   out += "</h4>"
		case model.BlockContentText_Quote:     out += "</quote>"
		case model.BlockContentText_Code:      out += "</code>"
		case model.BlockContentText_Title:     out += "</h1>"
		case model.BlockContentText_Checkbox:  out += `</div>`
		case model.BlockContentText_Marked:    out += "</li></ul>"
		case model.BlockContentText_Numbered:  out += "<li></ol>"
		case model.BlockContentText_Toggle:    out += `</div>`
		}
	}

	return out
}

func renderFile(isOpened bool, child *model.BlockContentOfFile) (out string) {
	if isOpened {
		switch child.File.Type {
		case model.BlockContentFile_File: break // TODO
		case model.BlockContentFile_Image: break // TODO
		case model.BlockContentFile_Video: break // TODO
		}
	} else {
		switch child.File.Type {
		case model.BlockContentFile_File: break
		case model.BlockContentFile_Image: break
		case model.BlockContentFile_Video: break
		}
	}
	return out
}

func renderBookmark(isOpened bool, child *model.BlockContentOfBookmark) (out string) {
	if isOpened {
		href := "" // TODO
		title := "" // TODO
		out = `<div class="bookmark"><a href="` + href + `">` + title
	} else {
		out = "</a></div>"
	}

	return out
}

func renderDiv(isOpened bool, child *model.BlockContentOfDiv) (out string) {
	if isOpened {
		switch child.Div.Style {
		case model.BlockContentDiv_Dots: out = `<hr class="dots">`
		case model.BlockContentDiv_Line: out = `<hr class="line">`
		}
	}

	return out
}

func renderIcon(isOpened bool, child *model.BlockContentOfIcon) (out string) {
	if isOpened {
		out = `<div class="icon ` + child.Icon.Name + `">`
	} else {
		out = "</div>"
	}

	return out
}

func fieldsGetFloat(field *types.Struct, key string) (value float64, ok bool) {
	if field != nil && field.Fields != nil {
		if value, ok := field.Fields[key]; ok {
			if s, ok := value.Kind.(*types.Value_NumberValue); ok {
				return s.NumberValue, true
			}
		}
	}
	return
}

func renderLayout(isOpened bool, child *model.BlockContentOfLayout, block *model.Block) (out string) {
	if isOpened {
		switch child.Layout.Style {
		case model.BlockContentLayout_Column:
			style := ""
			fields := block.Fields
			if fields != nil && fields.Fields != nil && fields.Fields["width"] != nil {
				width, _ := fieldsGetFloat(fields, "width")
				if width > 0 {
					style = `style="width: ` + string(int64(width * 100)) + `%">`
				}
			}
			out = `<div class="column" ` + style + `>`

		case model.BlockContentLayout_Row: out = `<div class="row">`
		}
	} else {
		out = "</div>"
	}

	return out
}

func wrapExportHtml (innerHtml string) string {
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
				<div class="anytype-container">` + innerHtml + `</div>
			</body>
		</html>`

	return output
}

func wrapCopyHtml (innerHtml string) string {
	output := `<meta charset='utf-8'>` + innerHtml + `</meta>`
	return output
}


func (c *converter) Convert (blocks []*model.Block) (out string) {
	tree := c.CreateTree(blocks)
	html := c.ProcessTree(&tree)

	fmt.Println("req.Blocks:", blocks)
	fmt.Println("tree:", c.ProcessTree(&tree))
	return wrapCopyHtml(html) //  gohtml.Format

}