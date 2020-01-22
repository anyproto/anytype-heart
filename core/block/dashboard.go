package block

import (
	"fmt"
	"os"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
)

func newDashboard(s *service, block anytype.Block) (smartBlock, error) {
	p := &dashboard{&commonSmart{s: s}}
	return p, nil
}

type dashboard struct {
	*commonSmart
}

func (p *dashboard) Init() {
	p.m.Lock()
	defer p.m.Unlock()
	if p.block.GetId() == p.s.anytype.PredefinedBlockIds().Home {
		// virtually add testpage to home screen
		p.addTestPage()
	}
	p.migratePageToLinks()
	p.init()
}

func (p *dashboard) migratePageToLinks() {
	s := p.newState()
	for id, v := range p.versions {
		if v.Model().GetPage() != nil {
			link := s.createLink(v.Model())
			if err := p.replace(s, id, link); err != nil {
				fmt.Println("middle: can't wrap page to link:", err)
			}
		}
		if link := v.Model().GetLink(); link != nil && link.TargetBlockId == testPageId {
			if !v.Virtual() {
				s.removeFromChilds(id)
				s.remove(id)
			}
		}
	}
	if _, err := s.apply(); err != nil {
		fmt.Println("can't apply state for migrating page to link", err)
	}
}

func (p *dashboard) addTestPage() {
	if os.Getenv("NO_TESTPAGE") != "" && os.Getenv("NO_TESTPAGE") != "0" {
		return
	}
	p.versions[testPageId] = base.NewVirtual(&model.Block{
		Id: testPageId + "-link",
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				Style: model.BlockContentLink_Page,
				Fields: &types.Struct{
					Fields: map[string]*types.Value{
						"name": testStringValue("Test page"),
						"icon": testStringValue(":deciduous_tree:"),
					},
				},
				TargetBlockId: testPageId,
			},
		},
	})
	p.versions[p.block.GetId()].Model().ChildrenIds = append(p.versions[p.block.GetId()].Model().ChildrenIds, testPageId+"-link")
}

func (p *dashboard) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	// add empty text block on new page after create
	return p.commonSmart.Create(req)
}

func (p *dashboard) Type() smartBlockType {
	return smartBlockTypeDashboard
}
