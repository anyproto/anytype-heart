package block

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func newPage(s *service, block anytype.Block) (smartBlock, error) {
	p := &page{&commonSmart{s: s}}
	return p, nil
}

type page struct {
	*commonSmart
}

func (p *page) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	return p.commonSmart.Create(req)
}

func (p *page) Type() smartBlockType {
	return smartBlockTypePage
}
