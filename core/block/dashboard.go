package block

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
)

func newDashboard(s *service, block anytype.Block) (smartBlock, error) {
	p := &dashboard{&commonSmart{s: s}}
	return p, nil
}

type dashboard struct {
	*commonSmart
}

func (p *dashboard) Type() smartBlockType {
	return smartBlockTypeDashboard
}
