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

func (w *walker) nextTreeLayer (node *Node) ( *Node) {
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
			case *model.BlockContentOfText: out += "<TEXT> " + cont.Text.Text +  " "
			case *model.BlockContentOfFile: break
			case *model.BlockContentOfBookmark: break
			case *model.BlockContentOfDiv: out += "<DIV> "
			case *model.BlockContentOfIcon: break
			case *model.BlockContentOfLayout: break
			case *model.BlockContentOfDashboard: break
			case *model.BlockContentOfPage: break
			case *model.BlockContentOfDataview: break
			case *model.BlockContentOfLink: break
		}

		if len(child.children) > 0 {
			out += w.ProcessTree(child)
		}

		switch child.model.Content.(type) {
		case *model.BlockContentOfText: out += "</TEXT>\n"
		case *model.BlockContentOfFile: break
		case *model.BlockContentOfBookmark: break
		case *model.BlockContentOfDiv: out += "</DIV>\n"
		case *model.BlockContentOfIcon: break
		case *model.BlockContentOfLayout: break
		case *model.BlockContentOfDashboard: break
		case *model.BlockContentOfPage: break
		case *model.BlockContentOfDataview: break
		case *model.BlockContentOfLink: break
		}
	}

	return gohtml.Format(out)
}
