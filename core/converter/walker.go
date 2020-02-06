package converter

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/yosssi/gohtml"
)

type Node struct {
	id     string
	model  *model.Block
	children []*Node
}

type walker struct {
	nodeTable map[string]*Node
	rootNode *Node
	remainBlocks []*model.Block
}

type Walker interface {
	CreateTree (blocks []*model.Block) Node
	ProcessTree (node *Node) (out string)
	PrintNode (node *Node) (out string)

	nextTreeLayer (node *Node) (processedNode *Node)
	filterById (blocks []*model.Block, id string) (out []*model.Block)
}


func New() Walker {
	w := &walker{
		rootNode: &Node{id: "root"},
		nodeTable: map[string]*Node{},
		remainBlocks: []*model.Block{},
	}

	return w
}


func (w *walker) filterById (blocks []*model.Block, id string) (out []*model.Block) {
	for _, b := range blocks {
		if b.Id != id {
			out = append(out, b)
		}
	}
	return out
}

func (w *walker) CreateTree (blocks []*model.Block) Node {
	//var htmlArr []string

	// 1. Create map
	for _, b := range blocks {
		w.nodeTable[b.Id] = &Node{b.Id, b,[]*Node{} }
	}

	// 2. Fill children field
	for _, b := range blocks {
		for _, child := range b.ChildrenIds {
			w.nodeTable[b.Id].children = append(w.nodeTable[b.Id].children, w.nodeTable[child])
		}
	}

	// 3. get root level blocks
	blocksRootLvl := blocks
	for _, b := range blocks {
		for _, child := range b.ChildrenIds {

			blocksRootLvl = w.filterById(blocksRootLvl, child)

		}
	}

	// 4. Create root
	//rootNode := Node{id:"root"}
	w.rootNode.model = &model.Block{ChildrenIds:[]string{}}

	w.remainBlocks = blocks

	// 5. Set top level blocks to root model
	for _, br := range blocksRootLvl {

		w.rootNode.model.ChildrenIds = append(w.rootNode.model.ChildrenIds, br.Id)
		w.remainBlocks = w.filterById(w.remainBlocks, br.Id)
		//fmt.Println("rootNode.model.ChildrenIds", w.rootNode.model.ChildrenIds)

	}

	//fmt.Println("BEFORE:", w.PrintNode(w.rootNode))
	w.rootNode = w.nextTreeLayer(w.rootNode)

	return *w.rootNode
}

func contains(s []*Node, e *Node) bool {
	for _, a := range s {
		if a.id == e.id {
			return true
		}
	}

	return false
}

func (w *walker) nextTreeLayer (node *Node) *Node {
	w.remainBlocks = w.filterById(w.remainBlocks, node.id)

	if len(w.remainBlocks) > 0 && len(node.model.ChildrenIds) > 0 {

		for _, childId := range node.model.ChildrenIds {
			w.remainBlocks = w.filterById(w.remainBlocks, childId)

			if len(w.nodeTable[childId].model.ChildrenIds) > 0 {
				w.nodeTable[childId] = w.nextTreeLayer(w.nodeTable[childId])
			}

			if !contains(node.children, w.nodeTable[childId]) {
				node.children = append(node.children, w.nodeTable[childId])
			}
			//fmt.Println("APPEND:", childId, "TO:", node.id, "CHILDREN:", node.children)
		}
	}

	//fmt.Println("NODE:", w.PrintNode(node))
	return node
}

func (w *walker) PrintNode (node *Node) (out string) {
	for _, child := range node.children {
		out += "<node>" + child.id
		if len(child.children) > 0 {
			out += w.PrintNode(child)
		}

		out += "</node>"
	}

	return "\n" + gohtml.Format(out)
}

func (w *walker) ProcessTree (node *Node) (out string) {
	for _, child := range node.children {

		switch  cont := child.model.Content.(type) {
			case *model.BlockContentOfText: out += renderText(true, cont)
			case *model.BlockContentOfFile: out += renderFile(true, cont)
			case *model.BlockContentOfBookmark: out += renderBookmark(true, cont)
			case *model.BlockContentOfDiv: out += renderDiv(true, cont)
			case *model.BlockContentOfIcon: out += renderIcon(true, cont)
			case *model.BlockContentOfLayout: out += renderLayout(true, cont)
			case *model.BlockContentOfDashboard: break;
			case *model.BlockContentOfPage: break;
			case *model.BlockContentOfDataview: break;
			case *model.BlockContentOfLink: break;
		}

		if len(child.children) > 0 {
			out += w.ProcessTree(child)
		}

		switch cont := child.model.Content.(type) {
			case *model.BlockContentOfText: out += renderText(false, cont)
			case *model.BlockContentOfFile: out += renderFile(false, cont)
			case *model.BlockContentOfBookmark: out += renderBookmark(false, cont)
			case *model.BlockContentOfDiv: out += renderDiv(false, cont)
			case *model.BlockContentOfIcon: out += renderIcon(false, cont)
			case *model.BlockContentOfLayout: out += renderLayout(false, cont)
			case *model.BlockContentOfDashboard: break;
			case *model.BlockContentOfPage: break;
			case *model.BlockContentOfDataview: break;
			case *model.BlockContentOfLink: break;
		}
	}

	return gohtml.Format(out)
}

func applyMarks (text string, marks *model.BlockContentTextMarks) (out string) {
	out = text
	// TODO
	return out
}

func renderText(isOpened bool, child *model.BlockContentOfText) (out string) {
	if isOpened {
		switch child.Text.Style {
			// TODO: renderText -> w.renderText; ul li, ul li -> ul li li
			case model.BlockContentText_Paragraph: out += "<p>" + applyMarks(child.Text.Text, child.Text.Marks)
			case model.BlockContentText_Header1: out += "<h1>" + child.Text.Text
			case model.BlockContentText_Header2: out += "<h2>" + child.Text.Text
			case model.BlockContentText_Header3: out += "<h3>" + child.Text.Text
			case model.BlockContentText_Header4: out += "<h4>" + child.Text.Text
			case model.BlockContentText_Quote: out += "<quote>" + applyMarks(child.Text.Text, child.Text.Marks)
			case model.BlockContentText_Code: out += "<code>" + applyMarks(child.Text.Text, child.Text.Marks)
			case model.BlockContentText_Title: out += "<h1>" + applyMarks(child.Text.Text, child.Text.Marks)
			case model.BlockContentText_Checkbox: out += `<div class="check"><input type="checkbox"/>` + applyMarks(child.Text.Text, child.Text.Marks)
			case model.BlockContentText_Marked: out += "<ul><li>" + applyMarks(child.Text.Text, child.Text.Marks)
			case model.BlockContentText_Numbered: out += "<ol><li>" + applyMarks(child.Text.Text, child.Text.Marks)
			case model.BlockContentText_Toggle: out += `<div class="toggle">` + applyMarks(child.Text.Text, child.Text.Marks)
		}

	} else {
		switch child.Text.Style {
			case model.BlockContentText_Paragraph: out += "</p>"
			case model.BlockContentText_Header1: out += "</h1>"
			case model.BlockContentText_Header2: out += "</h2>"
			case model.BlockContentText_Header3: out += "</h3>"
			case model.BlockContentText_Header4: out += "</h4>"
			case model.BlockContentText_Quote: out += "</quote>"
			case model.BlockContentText_Code: out += "</code>"
			case model.BlockContentText_Title: out += "</h1>"
			case model.BlockContentText_Checkbox: out += `</div>`
			case model.BlockContentText_Marked: out += "</li></ul>"
			case model.BlockContentText_Numbered: out += "<li></ol>"
			case model.BlockContentText_Toggle: out += `</div>`
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

func renderLayout(isOpened bool, child *model.BlockContentOfLayout) (out string) {
	if isOpened {
		switch child.Layout.Style {
			case model.BlockContentLayout_Column: out = `<div class="column">`
			case model.BlockContentLayout_Row: out = `<hr class="row">`
		}
	} else {
		out = "</div>"
	}

	return out
}
