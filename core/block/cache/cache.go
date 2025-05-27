package cache

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
)

type ObjectGetterComponent interface {
	app.Component
	ObjectGetter
}

type ObjectGetter interface {
	GetObject(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error)
	GetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error)
}

type ObjectWaitGetterComponent interface {
	app.Component
	ObjectWaitGetter
}

type ObjectWaitGetter interface {
	WaitAndGetObject(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error)
	WaitAndGetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error)
}

type CachedObjectGetter interface {
	ObjectGetter
	TryRemoveFromCache(ctx context.Context, objectId string) (res bool, err error)
}

func Do[t any](p ObjectGetter, objectID string, apply func(sb t) error) error {
	ctx := context.Background()
	sb, err := p.GetObject(ctx, objectID)
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

func DoWait[t any](p ObjectWaitGetter, ctx context.Context, objectID string, apply func(sb t) error) error {
	sb, err := p.WaitAndGetObject(ctx, objectID)
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

func DoContext[t any](p ObjectGetter, ctx context.Context, objectID string, apply func(sb t) error) error {
	sb, err := p.GetObject(ctx, objectID)
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

func DoContextFullID[t any](p ObjectGetter, ctx context.Context, id domain.FullID, apply func(sb t) error) error {
	sb, err := p.GetObjectByFullID(ctx, id)
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

// DoState2 picks two blocks and perform an action on them. The order of locks is always the same for two ids.
// It correctly handles the case when two ids are the same.
func DoState2[t1, t2 any](s ObjectGetter, firstID, secondID string, f func(*state.State, *state.State, t1, t2) error) error {
	if firstID == secondID {
		return DoState(s, firstID, func(st *state.State, b t1) error {
			// Check that b satisfies t2
			b2, ok := any(b).(t2)
			if !ok {
				var dummy t2
				return fmt.Errorf("block %s is not of type %T", firstID, dummy)
			}
			return f(st, st, b, b2)
		})
	}
	if firstID < secondID {
		return DoState(s, firstID, func(firstState *state.State, firstBlock t1) error {
			return DoState(s, secondID, func(secondState *state.State, secondBlock t2) error {
				return f(firstState, secondState, firstBlock, secondBlock)
			})
		})
	}
	return DoState(s, secondID, func(secondState *state.State, secondBlock t2) error {
		return DoState(s, firstID, func(firstState *state.State, firstBlock t1) error {
			return f(firstState, secondState, firstBlock, secondBlock)
		})
	})
}

func DoState[t any](p ObjectGetter, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
	ctx := context.Background()
	sb, err := p.GetObject(ctx, id)
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

	st := sb.NewState()
	err = apply(st, bb)
	if err != nil {
		return fmt.Errorf("apply func: %w", err)
	}

	return sb.Apply(st, flags...)
}

// TODO rename to something more meaningful
func DoStateCtx[t any](p ObjectGetter, ctx session.Context, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
	sb, err := p.GetObject(context.Background(), id)
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

	st := sb.NewStateCtx(ctx)
	err = apply(st, bb)
	if err != nil {
		return fmt.Errorf("apply func: %w", err)
	}

	return sb.Apply(st, flags...)
}
