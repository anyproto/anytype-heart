package blockbuilder

import (
	"fmt"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

// BuildAST builds tree structure from flat list of blocks
func BuildAST(raw []*model.Block) *Block {
	blocks := lo.SliceToMap(raw, func(b *model.Block) (string, *Block) {
		return b.Id, &Block{block: b}
	})

	isChildOf := map[string]string{}
	for _, b := range raw {
		children := make([]*Block, 0, len(b.ChildrenIds))
		for _, id := range b.ChildrenIds {
			isChildOf[id] = b.Id

			if v, ok := blocks[id]; ok {
				children = append(children, v)
			}
		}

		blocks[b.Id].children = children
	}

	return findRootBlock(blocks, isChildOf)
}

func findRootBlock(blocks map[string]*Block, isChildOf map[string]string) *Block {
	for id, b := range blocks {
		// Root block has no parent
		if _, ok := isChildOf[id]; !ok {
			return b
		}
	}
	return nil
}

func dropBlockIDs(b *Block) {
	b.block.Id = ""
	for i := range b.block.ChildrenIds {
		b.block.ChildrenIds[i] = ""
	}

	if b.block.Restrictions == nil {
		b.block.Restrictions = &model.BlockRestrictions{}
	}

	for _, c := range b.children {
		dropBlockIDs(c)
	}
}

func isMarkContainsLink(m *model.BlockContentTextMark) bool {
	return m.Type == model.BlockContentTextMark_Mention ||
		m.Type == model.BlockContentTextMark_Object
}

func remapLinksInDataview(b *model.BlockContentDataview, newIDtoOldID map[string]string) {
	b.TargetObjectId = newIDtoOldID[b.TargetObjectId]

	for _, view := range b.Views {
		for _, filter := range view.Filters {
			newID := filter.Value.GetStringValue()
			if newID != "" {
				oldID := newIDtoOldID[newID]
				if oldID != "" {
					filter.Value = pbtypes.String(oldID)
				}
			}
		}
	}
}

func remapLinks(root *Block, idsMap map[string]string) {
	if b := root.block.GetLink(); b != nil {
		b.TargetBlockId = idsMap[b.TargetBlockId]
	}
	if b := root.block.GetText(); b != nil {
		for _, m := range b.Marks.Marks {
			if isMarkContainsLink(m) {
				m.Param = idsMap[m.Param]
			}
		}
	}
	if b := root.block.GetBookmark(); b != nil {
		if b.Type == model.LinkPreview_Page {
			b.TargetObjectId = idsMap[b.TargetObjectId]
		}
	}
	if b := root.block.GetDataview(); b != nil {
		remapLinksInDataview(b, idsMap)
	}

	for _, c := range root.children {
		remapLinks(c, idsMap)
	}
}

func AssertPagesEqual(t *testing.T, want, got []*model.Block) bool {
	wantTree := BuildAST(want)
	gotTree := BuildAST(got)

	dropBlockIDs(wantTree)
	dropBlockIDs(gotTree)

	ok := assert.Equal(t, wantTree, gotTree)
	if !ok {
		fmt.Println("Want tree:")
		printTree(wantTree)
		fmt.Println("Got tree:")
		printTree(gotTree)
	}
	return ok
}

func AssertPagesEqualWithLinks(t *testing.T, want, got []*model.Block, idsMap map[string]string) bool {
	wantTree := BuildAST(want)
	gotTree := BuildAST(got)

	dropBlockIDs(wantTree)
	dropBlockIDs(gotTree)

	remapLinks(gotTree, idsMap)
	return assert.Equal(t, wantTree, gotTree)
}

func printTree(root *Block) {
	b := &strings.Builder{}
	renderNode(b, root, 0)
	fmt.Println(b.String())
}

func renderNode(b *strings.Builder, node *Block, lvl int) {
	b.WriteString(strings.Repeat("  ", lvl))
	b.WriteString(node.String())
	b.WriteString("\n")
	for _, c := range node.children {
		renderNode(b, c, lvl+1)
	}
}
