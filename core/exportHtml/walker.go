package converter

import (
	"fmt"
	"github.com/anytypeio/go-anytype-library/pb/model"
)

type Node struct {
	id     string
	model  *model.Block
	children []*Node
}

type walker struct {
	nodeTable map[string]*Node
	rootNode *Node
}

type Walker interface {
	CreateTree (blocks []*model.Block) Node

	nextTreeLayer (node *Node, remainBlocks []*model.Block) (processedNode *Node, newRemain []*model.Block)
	filterById (blocks []*model.Block, id string) (out []*model.Block)
}


func New(node *Node) Walker {
	w := &walker{
		rootNode: node,
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
			fmt.Println("child", child, blocksRootLvl)
		}
	}

	// 4. Create root
	rootNode := Node{id:"root"}
	rootNode.model = &model.Block{ChildrenIds:[]string{}}
	remainBLocks := blocks

	// 5. Set top level blocks to root model
	for _, br := range blocksRootLvl {
		rootNode.model.ChildrenIds = append(rootNode.model.ChildrenIds, br.Id)
		remainBLocks = w.filterById(remainBLocks, br.Id)
	}

	w.nextTreeLayer(&rootNode, remainBLocks)

	fmt.Println("TREE:", rootNode, remainBLocks) //rootNode, "BLOCKS:", blocks, "ROOTLVL:", blocksRootLvl,
	return rootNode
}

func (w *walker) nextTreeLayer (node *Node, remainBlocks []*model.Block) (processedNode *Node, newRemain []*model.Block) {
	processedNode = node
	newRemain = remainBlocks
	fmt.Println("processedNode.model", processedNode.model)
	fmt.Println("remain", remainBlocks)
	if len(newRemain) > 0 && len(processedNode.model.ChildrenIds) > 0 {

		for _, childId := range processedNode.model.ChildrenIds {
			if len(w.nodeTable[childId].model.ChildrenIds) > 0 {
				w.nodeTable[childId], newRemain = w.nextTreeLayer(w.nodeTable[childId], newRemain)
			}

			processedNode.children = append(processedNode.children, w.nodeTable[childId])
			newRemain = w.filterById(newRemain, childId)
		}
	}

	fmt.Println("return processedNode", processedNode)
	return processedNode, newRemain
}

