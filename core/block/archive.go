package block

import (
	"errors"

	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	errNotPossibleForArchive = errors.New("not possible for archive")
)

func newArchive(s *service) (smartBlock, error) {
	p := &archive{&commonSmart{s: s}}
	return p, nil
}

type archive struct {
	*commonSmart
}

func (p *archive) Init() {
	p.m.Lock()
	defer p.m.Unlock()
	p.history = history.NewHistory(0)
	p.init()
}

func (p *archive) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	return "", errNotPossibleForArchive
}

func (p *archive) Type() smartBlockType {
	return smartBlockTypeDashboard
}
