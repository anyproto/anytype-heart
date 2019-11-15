package block

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func newDashboard(s *service, block anytype.Block) (smartBlock, error) {
	p := &dashboard{&commonSmart{s: s}}
	return p, nil
}

type dashboard struct {
	*commonSmart
}

func (p *dashboard) Create(req pb.RpcBlockCreateRequest)(id string, err error) {
	return p.commonSmart.Create(req)
}

func (p *dashboard) Type() smartBlockType {
	return smartBlockTypeDashboard
}
