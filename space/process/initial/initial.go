package initial

import (
	"context"

	"github.com/anyproto/anytype-heart/space/process/modechanger"
)

type initial struct {
}

func New() modechanger.Process {
	return &initial{}
}

func (i *initial) Start(ctx context.Context) error {
	return nil
}

func (i *initial) Close(ctx context.Context) error {
	return nil
}

func (i *initial) CanTransition(next modechanger.Mode) bool {
	return true
}
