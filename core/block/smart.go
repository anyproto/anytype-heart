package block

import (
	"errors"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
)

var (
	ErrUnexpectedSmartBlockType = errors.New("unexpected smartBlock type")
)

type smartBlock interface {
	Open(b anytype.Block) error
	GetId() string
	Type() smartBlockType
	Close() error
}

type smartBlockType int

const (
	smartBlockTypeDashboard smartBlockType = iota
	smartBlockTypePage
)

func openSmartBlock(s *service, id string) (sb smartBlock, err error) {
	b, err := s.anytype.GetBlock(id)
	if err != nil {
		return
	}
	ver, err := b.GetCurrentVersion()
	if err != nil {
		return
	}

	switch ver.(type) {
	case *core.DashboardVersion:
		sb, err = newDashboard(s, b)
	case *core.PageVersion:
		sb, err = newPage(s, b)
	default:
		return nil, ErrUnexpectedSmartBlockType
	}
	if err = sb.Open(b); err != nil {
		return
	}
	return
}
