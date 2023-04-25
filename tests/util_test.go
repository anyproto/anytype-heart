package tests

import (
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type options struct {
	children []*Block
	color    string
}

type Option func(*options)

func Children(v ...*Block) Option {
	return func(o *options) {
		o.children = v
	}
}

func Color(v string) Option {
	return func(o *options) {
		o.color = v
	}
}

type Block struct {
	block    *model.Block
	children []*Block
}

func (b *Block) Copy() *Block {
	children := make([]*Block, 0, len(b.children))
	for _, c := range b.children {
		children = append(children, c.Copy())
	}
	bc := Block{
		block:    pbtypes.CopyBlock(b.block),
		children: children,
	}
	return &bc
}

func (b *Block) Build() []*model.Block {
	if b.block.Id == "" {
		b.block.Id = bson.NewObjectId().Hex()
	}

	var descendants []*model.Block
	b.block.ChildrenIds = b.block.ChildrenIds[:0]
	for _, c := range b.children {
		descendants = append(descendants, c.Build()...)
		b.block.ChildrenIds = append(b.block.ChildrenIds, c.block.Id)
	}

	return append([]*model.Block{
		b.block,
	}, descendants...)
}

func Text(s string, opts ...Option) *Block {
	o := options{
		// Init children for easier equality check in tests
		children: []*Block{},
	}
	for _, apply := range opts {
		apply(&o)
	}

	return &Block{
		block: &model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: s,
				},
			},
		},
		children: o.children,
	}
}

func BuildAST(raw []*model.Block) *Block {
	rawMap := make(map[string]*model.Block, len(raw))
	for _, b := range raw {
		rawMap[b.Id] = b
	}

	blocks := make(map[string]*Block, len(raw))
	for _, b := range raw {
		blocks[b.Id] = &Block{
			block: b,
		}
	}

	isChildOf := map[string]string{}

	for _, b := range raw {
		children := make([]*Block, 0, len(b.ChildrenIds))
		for _, id := range b.ChildrenIds {
			isChildOf[id] = b.Id
			v, ok := blocks[id]
			if !ok {
				continue
			}
			children = append(children, v)
		}

		blocks[b.Id].children = children
	}

	for _, b := range raw {
		if _, ok := isChildOf[b.Id]; !ok {
			return blocks[b.Id]
		}
	}
	return nil
}

func dropBlockIDs(b *Block) {
	b.block.Id = ""
	for i := range b.block.ChildrenIds {
		b.block.ChildrenIds[i] = ""
	}

	for _, c := range b.children {
		dropBlockIDs(c)
	}
}

func AssertTreesEqual(t *testing.T, a, b *Block) bool {
	ac := a.Copy()
	bc := b.Copy()

	dropBlockIDs(ac)
	dropBlockIDs(bc)

	return assert.Equal(t, ac, bc)
}

func TestBuilder(t *testing.T) {
	makeTree := func() *Block {
		return Text("kek", Children(
			Text("level 2", Color("red")),
			Text("level 2.1", Children(
				Text("level 3.1"), Text("level 3.2"), Text("level 3.3"))),
		))
	}

	b := makeTree()
	blocks := b.Build()

	root := BuildAST(blocks)

	assert.Equal(t, b, root)

}

func TestTreesEquality(t *testing.T) {
	makeTree := func() *Block {
		root := Text("level 1", Children(
			Text("level 2", Color("red")),
			Text("level 2.1", Children(
				Text("level 3.1"), Text("level 3.2"), Text("level 3.3"))),
		))

		return BuildAST(root.Build())
	}
	a := makeTree()
	b := makeTree()

	AssertTreesEqual(t, a, b)
}
