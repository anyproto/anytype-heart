package personalspace

import (
	"context"

	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
)

type personalLoader struct {
	loader.Loader
	newLoader func() loader.Loader
}

func (p *personalLoader) Start(ctx context.Context) (err error) {
	p.Loader = p.newLoader()
	return p.Loader.Start(ctx)
}
