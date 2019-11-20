package block

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

const (
	pageTitleSuffix = "/title"
	pageIconSuffix  = "/icon"
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
	if icon, ok := fieldsGetString(root.Fields, "icon"); ok {
		p.addIcon(icon)
	}
	if title, ok := fieldsGetString(root.Fields, "title"); ok {
		p.addTitle(title)
	}
	p.showFullscreen()
}

func (p *page) addTitle(title string) {
	var b = virtualBlock{
		&model.Block{
			Id: p.block.GetId() + pageTitleSuffix,
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  title,
					Style: model.BlockContentText_Title,
				},
			},
		},
	}
	p.versions[b.Model().Id] = b
	p.root().ChildrenIds = append([]string{b.Model().Id}, p.root().ChildrenIds...)
}

func (p *page) addIcon(icon string) {
	var b = virtualBlock{
		&model.Block{
			Id: p.block.GetId() + pageIconSuffix,
			Content: &model.BlockContentOfIcon{
				Icon: &model.BlockContentIcon{
					Name: icon,
				},
			},
		},
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
