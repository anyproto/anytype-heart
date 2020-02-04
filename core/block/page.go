package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
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

var (
	_ text.Block     = (*pageTitleBlock)(nil)
	_ base.IconBlock = (*pageIconBlock)(nil)
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
	p.history = history.NewHistory(0)
	p.init()
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

func (p *page) SetFields(fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	for _, bf := range fields {
		if err = p.setFields(s, bf.BlockId, bf.Fields); err != nil {
			return
		}
		if bf.BlockId == p.GetId() {
			// apply changes to virtual blocks
			name, _ := fieldsGetString(p.versions[bf.BlockId].Model().Fields, "name")
			nameId := p.block.GetId() + pageTitleSuffix
			nameBlock := s.get(nameId).(*pageTitleBlock)
			nameBlock.Block.SetText(name, nil)

			icon, _ := fieldsGetString(p.versions[bf.BlockId].Model().Fields, "icon")
			iconId := p.block.GetId() + pageIconSuffix
			iconBlock := s.get(iconId).(*pageIconBlock)
			iconBlock.IconBlock.SetIconName(icon)
		}
	}
	return p.applyAndSendEvent(s)
}

func (p *page) UpdateTextBlocks(ids []string, event bool, apply func(t text.Block) error) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	var (
		tb      text.Block
		titleId = p.block.GetId() + pageTitleSuffix
	)
	for _, id := range ids {
		if tb, err = s.getText(id); err != nil {
			return
		}
		if err = apply(tb); err != nil {
			return
		}
		if id == titleId {
			p.updateTitle(s, tb.GetText())
		}
	}
	return p.applyAndSendEventHist(s, true, event)
}

func (p *page) UpdateIconBlock(id string, apply func(t base.IconBlock) error) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	icon, err := s.getIcon(id)
	if err != nil {
		return
	}
	if err = apply(icon); err != nil {
		return
	}
	if id == p.block.GetId()+pageIconSuffix {
		p.updateIcon(s, icon.Model().GetIcon().Name)
	}
	return p.applyAndSendEvent(s)
}

func (p *page) updateTitle(s *state, title string) {
	fields := s.get(p.GetId()).Model().Fields
	if fields.Fields == nil {
		fields.Fields = make(map[string]*types.Value)
	}
	fields.Fields["name"] = testStringValue(title)
}

func (p *page) updateIcon(s *state, icon string) {
	fields := s.get(p.GetId()).Model().Fields
	if fields.Fields == nil {
		fields.Fields = make(map[string]*types.Value)
	}
	fields.Fields["icon"] = testStringValue(icon)
}

type pageTitleBlock struct {
	text.Block
	page *page
}

func (b *pageTitleBlock) Virtual() bool {
	return true
}

func (b *pageTitleBlock) SetText(text string, marks *model.BlockContentTextMarks) (err error) {
	return b.Block.SetText(text, nil)
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

func (b *pageTitleBlock) Split(_ int32) (simple.Block, error) {
	return nil, fmt.Errorf("page title can't be splitted")
}

func (b *pageTitleBlock) Merge(_ simple.Block) error {
	return fmt.Errorf("page title can't be merged ")
}

func (b *pageTitleBlock) SetStyle(style model.BlockContentTextStyle) {}
func (b *pageTitleBlock) SetChecked(v bool)                          {}
func (b *pageTitleBlock) SetTextBackgroundColor(_ string)            {}
func (b *pageTitleBlock) SetTextColor(_ string)                      {}

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

func (b *pageIconBlock) SetIconName(name string) (err error) {
	return b.IconBlock.SetIconName(name)
}

func (b *pageIconBlock) Diff(block simple.Block) ([]*pb.EventMessage, error) {
	return b.IconBlock.Diff(block.(*pageIconBlock).IconBlock)
}
