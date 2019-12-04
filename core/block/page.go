package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
)

const (
	pageTitleSuffix = "-title"
	pageIconSuffix  = "-icon"
)

func newPage(s *service, block anytype.Block) (smartBlock, error) {
	p := &page{&commonSmart{s: s}}
	return p, nil
}

type page struct {
	*commonSmart
}

func (p *page) Init() {
	p.m.Lock()
	defer p.m.Unlock()
	root := p.root()
	if name, ok := fieldsGetString(root.Fields, "name"); ok {
		p.addName(name)
	}
	if icon, ok := fieldsGetString(root.Fields, "icon"); ok {
		p.addIcon(icon)
	}
	p.show()
}

func (p *page) addName(title string) {
	var b = &pageTitleBlock{
		Block: simple.New(&model.Block{
			Id: p.block.GetId() + pageTitleSuffix,
			Restrictions: &model.BlockRestrictions{
				Read:   false,
				Edit:   false,
				Remove: true,
				Drag:   true,
				DropOn: false,
			}, Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  title,
					Style: model.BlockContentText_Title,
				},
			},
		}).(text.Block),
		page: p,
	}

	p.versions[b.Model().Id] = b
	p.root().ChildrenIds = append([]string{b.Model().Id}, p.root().ChildrenIds...)
}

func (p *page) addIcon(icon string) {
	var b = &pageIconBlock{
		IconBlock: simple.New(&model.Block{
			Id: p.block.GetId() + pageIconSuffix,
			Restrictions: &model.BlockRestrictions{
				Read:   false,
				Edit:   false,
				Remove: true,
				Drag:   true,
				DropOn: true,
			}, Content: &model.BlockContentOfIcon{
				Icon: &model.BlockContentIcon{
					Name: icon,
				},
			},
		}).(base.IconBlock),
		page: p,
	}

	p.versions[b.Model().Id] = b
	p.root().ChildrenIds = append([]string{b.Model().Id}, p.root().ChildrenIds...)
}

func (p *page) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	return p.commonSmart.Create(req)
}

func (p *page) Type() smartBlockType {
	return smartBlockTypePage
}

func (p *page) SetFields(id string, fields *types.Struct) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	if err = p.setFields(id, fields); err != nil {
		return
	}
	if id == p.GetId() {
		// apply changes to virtual blocks
		name, _ := fieldsGetString(p.versions[id].Model().Fields, "name")
		nameId := p.block.GetId() + pageTitleSuffix
		nameBlock := p.versions[nameId].Copy().(*pageTitleBlock)
		nameBlock.Block.SetText(name, nil)
		diff, _ := p.versions[nameId].Diff(nameBlock)
		if len(diff) > 0 {
			p.versions[nameId] = nameBlock
		}
		msgs := diff

		icon, _ := fieldsGetString(p.versions[id].Model().Fields, "icon")
		iconId := p.block.GetId() + pageIconSuffix
		iconBlock := p.versions[iconId].Copy().(*pageIconBlock)
		iconBlock.IconBlock.SetIconName(icon)
		diff, _ = p.versions[iconId].Diff(iconBlock)
		if len(diff) > 0 {
			p.versions[iconId] = iconBlock
			msgs = append(msgs, diff...)
		}
		if len(msgs) > 0 {
			p.s.sendEvent(&pb.Event{
				Messages:  msgs,
				ContextId: p.GetId(),
			})
		}
	}
	return
}

type pageTitleBlock struct {
	text.Block
	page *page
}

func (b *pageTitleBlock) Virtual() bool {
	return true
}

func (b *pageTitleBlock) SetText(text string, marks *model.BlockContentTextMarks) (err error) {
	if err = b.Block.SetText(text, nil); err != nil {
		return
	}
	fields := b.page.versions[b.page.GetId()].Copy().Model().Fields
	fields.Fields["name"] = testStringValue(text)
	return b.page.setFields(b.page.GetId(), fields)
}

func (b *pageTitleBlock) Copy() simple.Block {
	return &pageTitleBlock{
		Block: b.Block.Copy().(text.Block),
		page:  b.page,
	}
}

func (b *pageTitleBlock) Diff(block simple.Block) ([]*pb.EventMessage, error) {
	return b.Block.Diff(block.(*pageTitleBlock).Block)
}

func (b *pageTitleBlock) Split(pos int64) (simple.Block, error) {
	return nil, fmt.Errorf("page title can't be splitted")
}

func (b *pageTitleBlock) SetStyle(style model.BlockContentTextStyle) {}
func (b *pageTitleBlock) SetChecked(v bool)                          {}

type pageIconBlock struct {
	base.IconBlock
	page *page
}

func (b *pageIconBlock) Virtual() bool {
	return true
}

func (b *pageIconBlock) Copy() simple.Block {
	return &pageIconBlock{
		IconBlock: b.IconBlock.Copy().(base.IconBlock),
		page:      b.page,
	}
}

func (b *pageIconBlock) SetIconName(name string) error {
	if err := b.IconBlock.SetIconName(name); err != nil {
		return err
	}
	fields := b.page.versions[b.page.GetId()].Copy().Model().Fields
	fields.Fields["icon"] = testStringValue(name)
	return b.page.setFields(b.page.GetId(), fields)
}

func (b *pageIconBlock) Diff(block simple.Block) ([]*pb.EventMessage, error) {
	return b.IconBlock.Diff(block.(*pageIconBlock).IconBlock)
}
