package simple

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/google/uuid"
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
	Model() *model.Block
	Diff(block Block) (msgs []*pb.EventMessage, err error)
	Copy() Block
}

func New(block *model.Block) (b Block) {
	if block.Id == "" {
		block.Id = uuid.New().String()
	}
	for _, c := range registry {
		if b = c(block); b != nil {
			return
		}
	}
	return fallback(block)
}
