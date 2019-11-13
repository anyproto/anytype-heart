package block

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
)

func newPage(s *service, block anytype.Block) (smartBlock, error) {
	p := &page{&commonSmart{s: s}}
	return p, nil
}

type page struct {
	*commonSmart
}

func (p *page) Type() smartBlockType {
	return smartBlockTypePage
}
