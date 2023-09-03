package block

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app/ocache"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
)

type ctxKey int

var errAppIsNotRunning = errors.New("app is not running")

const (
	optsKey                  ctxKey = iota
	derivedObjectLoadTimeout        = time.Minute * 30
	objectLoadTimeout               = time.Minute * 3
	concurrentTrees                 = 10
)

type treeCreateCache struct {
	initFunc InitFunc
}

type cacheOpts struct {
	spaceId      string
	createOption *treeCreateCache
	buildOption  source.BuildOptions
	putObject    smartblock.SmartBlock
}

type InitFunc = func(id string) *smartblock.InitContext

func (s *Service) createCache() ocache.OCache {
	return ocache.New(
		s.cacheLoad,
		// ocache.WithLogger(log.Desugar()),
		ocache.WithGCPeriod(time.Minute),
		// TODO: [MR] Get ttl from config
		ocache.WithTTL(time.Duration(60)*time.Second),
	)
}

func (s *Service) cacheLoad(ctx context.Context, id string) (value ocache.Object, err error) {
	// TODO Pass options as parameter?
	opts := ctx.Value(optsKey).(cacheOpts)

	buildObject := func(id string) (sb smartblock.SmartBlock, err error) {
		return s.objectFactory.InitObject(id, &smartblock.InitContext{Ctx: ctx, BuildOpts: opts.buildOption, SpaceID: opts.spaceId})
	}
	createObject := func() (sb smartblock.SmartBlock, err error) {
		initCtx := opts.createOption.initFunc(id)
		initCtx.IsNewObject = true
		initCtx.Ctx = ctx
		initCtx.SpaceID = opts.spaceId
		initCtx.BuildOpts = opts.buildOption
		return s.objectFactory.InitObject(id, initCtx)
	}

	switch {
	case opts.createOption != nil:
		return createObject()
	case opts.putObject != nil:
		// putting object through cache
		return opts.putObject, nil
	default:
		break
	}

	sbt, _ := s.sbtProvider.Type(opts.spaceId, id)
	switch sbt {
	default:
		return buildObject(id)
	}
}

func (s *Service) getObject(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	ctx = updateCacheOpts(ctx, func(opts cacheOpts) cacheOpts {
		if opts.spaceId == "" {
			opts.spaceId = id.SpaceID
		}
		return opts
	})
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		done    = make(chan struct{})
		closing bool
	)
	var start time.Time
	go func() {
		select {
		case <-done:
			cancel()
		case <-s.closing:
			start = time.Now()
			cancel()
			closing = true
		}
	}()
	v, err := s.cache.Get(ctx, id.ObjectID)
	close(done)
	if closing && errors.Is(err, context.Canceled) {
		log.With("close_delay", time.Since(start).Milliseconds()).With("objectID", id).Warnf("object was loading during closing")
	}
	if err != nil {
		return
	}
	if v == nil {
		fmt.Println()
	}
	return v.(smartblock.SmartBlock), nil
}

func (s *Service) getObjectWithTimeout(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, objectLoadTimeout)
		defer cancel()
	}
	return s.getObject(ctx, id)
}

// PickBlock returns opened smartBlock or opens smartBlock in silent mode
func (s *Service) PickBlock(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error) {
	spaceID, err := s.spaceService.ResolveSpaceID(objectID)
	if err != nil {
		// Object not loaded yet
		return nil, source.ErrObjectNotFound
	}
	return s.getObjectWithTimeout(ctx, domain.FullID{
		SpaceID:  spaceID,
		ObjectID: objectID,
	})
}

func CacheOptsWithRemoteLoadDisabled(ctx context.Context) context.Context {
	return updateCacheOpts(ctx, func(opts cacheOpts) cacheOpts {
		opts.buildOption.DisableRemoteLoad = true
		return opts
	})
}

func updateCacheOpts(ctx context.Context, update func(opts cacheOpts) cacheOpts) context.Context {
	opts, ok := ctx.Value(optsKey).(cacheOpts)
	if !ok {
		opts = cacheOpts{}
	}
	return context.WithValue(ctx, optsKey, update(opts))
}

type Picker interface {
	PickBlock(ctx context.Context, objectID string) (sb smartblock.SmartBlock, err error)
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

func DoContext[t any](p Picker, ctx context.Context, objectID string, apply func(sb t) error) error {
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

// DoState2 picks two blocks and perform an action on them. The order of locks is always the same for two ids.
// It correctly handles the case when two ids are the same.
func DoState2[t1, t2 any](s Picker, firstID, secondID string, f func(*state.State, *state.State, t1, t2) error) error {
	if firstID == secondID {
		return DoStateAsync(s, firstID, func(st *state.State, b t1) error {
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
		return DoStateAsync(s, firstID, func(firstState *state.State, firstBlock t1) error {
			return DoStateAsync(s, secondID, func(secondState *state.State, secondBlock t2) error {
				return f(firstState, secondState, firstBlock, secondBlock)
			})
		})
	}
	return DoStateAsync(s, secondID, func(secondState *state.State, secondBlock t2) error {
		return DoStateAsync(s, firstID, func(firstState *state.State, firstBlock t1) error {
			return f(firstState, secondState, firstBlock, secondBlock)
		})
	})
}

func DoStateAsync[t any](p Picker, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
	ctx := context.Background()
	sb, err := p.PickBlock(ctx, id)
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
func DoStateCtx[t any](p Picker, ctx session.Context, id string, apply func(s *state.State, sb t) error, flags ...smartblock.ApplyFlag) error {
	sb, err := p.PickBlock(context.Background(), id)
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
