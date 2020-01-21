package simple

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type BlockCreator = func(m *model.Block) Block

var (
	registry []BlockCreator
	fallback BlockCreator
)

func RegisterCreator(c BlockCreator) {
	registry = append(registry, c)
}

func RegisterFallback(c BlockCreator) {
	fallback = c
}

type Block interface {
	Virtual() bool
	Model() *model.Block
	Diff(block Block) (msgs []*pb.EventMessage, err error)
	Copy() Block
}

type Ctrl interface {
	Anytype() anytype.Anytype
	UpdateBlock(id string, apply func(b Block) error) error
}

type BlockInit interface {
	Block
	Init(ctrl Ctrl)
}

type BlockClose interface {
	Block
	Close()
}

func New(block *model.Block) (b Block) {
	for _, c := range registry {
		if b = c(block); b != nil {
			return
		}
	}
	return fallback(block)
}
