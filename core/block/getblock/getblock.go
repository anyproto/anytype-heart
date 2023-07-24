package getblock

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
)

type Picker interface {
	PickBlock(ctx context.Context, id string) (sb smartblock.SmartBlock, err error)
}

func Do[t any](p Picker, objectID string, apply func(sb t) error) error {
	ctx := context.Background()
	sb, err := p.PickBlock(ctx, objectID)
	if err != nil {
		return err
	}

	bb, ok := sb.(t)
	if !ok {
		var dummy = new(t)
		return fmt.Errorf("the interface %T is not implemented in %T", dummy, sb)
	}

	sb.Lock()
	defer sb.Unlock()
	return apply(bb)
}
