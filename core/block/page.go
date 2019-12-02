package block

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
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
	var b = base.NewVirtual(&model.Block{
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
	})

	p.versions[b.Model().Id] = b
	p.root().ChildrenIds = append([]string{b.Model().Id}, p.root().ChildrenIds...)
}

func (p *page) addIcon(icon string) {

	var b = base.NewVirtual(&model.Block{
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
	})

	p.versions[b.Model().Id] = b
	p.root().ChildrenIds = append([]string{b.Model().Id}, p.root().ChildrenIds...)
}

func (p *page) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	return p.commonSmart.Create(req)
}

func (p *page) Type() smartBlockType {
	return smartBlockTypePage
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
	fields := b.page.versions[b.page.GetId()].Model().Fields
	fields.Fields["name"] = testStringValue(text)
	return b.page.setFields(b.page.GetId(), fields)
}

func (b *pageTitleBlock) Copy() simple.Block {
	return &pageTitleBlock{
		Block: b.Block.Copy().(text.Block),
		page:  b.page,
	}
}

func (b *pageTitleBlock) SetStyle(style model.BlockContentTextStyle) {}
func (b *pageTitleBlock) SetChecked(v bool)                          {}
