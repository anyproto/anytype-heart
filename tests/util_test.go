package tests

import (
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type options struct {
	children     []*Block
	color        string
	restrictions *model.BlockRestrictions
	textStyle    model.BlockContentTextStyle
	marks        *model.BlockContentTextMarks
	fields       *types.Struct
}

type Option func(*options)

func Children(v ...*Block) Option {
	return func(o *options) {
		o.children = v
	}
}

func Restrictions(r model.BlockRestrictions) Option {
	return func(o *options) {
		o.restrictions = &r
	}
}

func Fields(v *types.Struct) Option {
	return func(o *options) {
		o.fields = v
	}
}

func Color(v string) Option {
	return func(o *options) {
		o.color = v
	}
}

func TextStyle(s model.BlockContentTextStyle) Option {
	return func(o *options) {
		o.textStyle = s
	}
}

func TextMarks(m model.BlockContentTextMarks) Option {
	return func(o *options) {
		o.marks = &m
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

func mkBlock(b *model.Block, opts ...Option) *Block {
	o := options{
		// Init children for easier equality check in tests
		children:     []*Block{},
		restrictions: &model.BlockRestrictions{},
	}
	for _, apply := range opts {
		apply(&o)
	}
	b.Restrictions = o.restrictions
	b.Fields = o.fields
	return &Block{
		block:    b,
		children: o.children,
	}
}

func Root(opts ...Option) *Block {
	return mkBlock(&model.Block{
		Content: &model.BlockContentOfSmartblock{
			Smartblock: &model.BlockContentSmartblock{},
		},
	}, opts...)
}

func Layout(style model.BlockContentLayoutStyle, opts ...Option) *Block {
	return mkBlock(&model.Block{
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{Style: style},
		},
	}, opts...)
}

func Header(opts ...Option) *Block {
	return Layout(model.BlockContentLayout_Header, append(opts, Restrictions(
		model.BlockRestrictions{
			Edit:   true,
			Remove: true,
			Drag:   true,
			DropOn: true,
		}))...)
}

func FeaturedRelations(opts ...Option) *Block {
	return mkBlock(&model.Block{
		Content: &model.BlockContentOfFeaturedRelations{
			FeaturedRelations: &model.BlockContentFeaturedRelations{},
		},
	}, append(opts, Restrictions(model.BlockRestrictions{
		Remove: true,
		Drag:   true,
		DropOn: true,
	}))...)
}

func Text(s string, opts ...Option) *Block {
	o := options{
		marks: &model.BlockContentTextMarks{},
	}
	for _, apply := range opts {
		apply(&o)
	}

	return mkBlock(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  s,
				Style: o.textStyle,
				Color: o.color,
				Marks: o.marks,
			},
		},
	}, opts...)
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
