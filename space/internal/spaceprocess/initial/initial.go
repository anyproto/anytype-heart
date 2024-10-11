package initial

import (
	"context"

	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
)

type initial struct {
}

func New() mode.Process {
	return &initial{}
}

func (i *initial) Start(ctx context.Context) error {
	return nil
}

func (i *initial) Close(ctx context.Context) error {
	return nil
}

func (i *initial) CanTransition(next mode.Mode) bool {
	return true
}
