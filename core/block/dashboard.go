package block

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
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
	p.show()
}

func (p *dashboard) addTestPage() {
	p.versions[testPageId] = simple.NewVirtual(&model.Block{
		Id: testPageId,
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				"name": testStringValue("Test page"),
				"icon": testStringValue(":deciduous_tree:"),
			},
		},
		ChildrenIds: []string{},
		Content: &model.BlockContentOfPage{
			Page: &model.BlockContentPage{Style: model.BlockContentPage_Empty},
		},
	})
	p.versions[p.block.GetId()].Model().ChildrenIds = append(p.versions[p.block.GetId()].Model().ChildrenIds, testPageId)
}

func (p *dashboard) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	return p.commonSmart.Create(req)
}

func (p *dashboard) Type() smartBlockType {
	return smartBlockTypeDashboard
}
