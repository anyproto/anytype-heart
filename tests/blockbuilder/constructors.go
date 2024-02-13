package blockbuilder

import (
	"fmt"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Block struct {
	block    *model.Block
	children []*Block
}

func (b *Block) Block() *model.Block {
	return b.block
}

func (b *Block) String() string {
	return strings.TrimPrefix(fmt.Sprintf("%T %s", b.block.Content, b.block.Content), "*model.BlockContentOf")
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
	b.BackgroundColor = o.backgroundColor
	b.Fields = o.fields
	b.Id = o.id
	return &Block{
		block:    b,
		children: o.children,
	}
}

type options struct {
	children        []*Block
	color           string
	restrictions    *model.BlockRestrictions
	textStyle       model.BlockContentTextStyle
	textIconImage   string
	marks           *model.BlockContentTextMarks
	fields          *types.Struct
	id              string
	backgroundColor string
	fileHash        string
}

type Option func(*options)

func ID(id string) Option {
	return func(o *options) {
		o.id = id
	}
}

func BackgroundColor(color string) Option {
	return func(o *options) {
		o.backgroundColor = color
	}
}

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

func TextIconImage(id string) Option {
	return func(o *options) {
		o.textIconImage = id
	}
}

func TextMarks(m model.BlockContentTextMarks) Option {
	return func(o *options) {
		o.marks = &m
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
				Text:      s,
				Style:     o.textStyle,
				Color:     o.color,
				Marks:     o.marks,
				IconImage: o.textIconImage,
			},
		},
	}, opts...)
}

func Row(opts ...Option) *Block {
	return Layout(model.BlockContentLayout_Row, opts...)
}

func Column(opts ...Option) *Block {
	return Layout(model.BlockContentLayout_Column, opts...)
}

func FileHash(hash string) Option {
	return func(o *options) {
		o.fileHash = hash
	}
}

func File(targetObjectId string, opts ...Option) *Block {
	var o options
	for _, apply := range opts {
		apply(&o)
	}

	return mkBlock(&model.Block{
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Hash:           o.fileHash,
				TargetObjectId: targetObjectId,
			},
		},
	}, opts...)
}
