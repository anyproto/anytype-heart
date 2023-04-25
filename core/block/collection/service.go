package collection

import (
	"fmt"
	"sync"

	"github.com/anytypeio/any-sync/app"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

type Service struct {
	picker block.Picker

	lock        *sync.RWMutex
	collections map[string]chan []slice.Change[string]
}

func New() *Service {
	return &Service{
		lock:        &sync.RWMutex{},
		collections: map[string]chan []slice.Change[string]{},
	}
}

func (s *Service) Init(a *app.App) (err error) {
	s.picker = app.MustComponent[block.Picker](a)
	return nil
}

func (s *Service) Name() string {
	return "collection"
}

const storeKey = "objects"

func (s *Service) Add(ctx *session.Context, req *pb.RpcObjectCollectionAddRequest) error {
	return s.updateCollection(ctx, req.ContextId, func(col []string) []string {
		toAdd := slice.Difference(req.ObjectIds, col)
		pos := slice.FindPos(col, req.AfterId)
		if pos >= 0 {
			col = slice.Insert(col, pos+1, toAdd...)
		} else {
			col = append(toAdd, col...)
		}
		return col
	})
}

func (s *Service) Remove(ctx *session.Context, req *pb.RpcObjectCollectionRemoveRequest) error {
	return s.updateCollection(ctx, req.ContextId, func(col []string) []string {
		col = slice.Filter(col, func(id string) bool {
			return slice.FindPos(req.ObjectIds, id) == -1
		})
		return col
	})
}

func (s *Service) Sort(ctx *session.Context, req *pb.RpcObjectCollectionSortRequest) error {
	return s.updateCollection(ctx, req.ContextId, func(col []string) []string {
		exist := map[string]struct{}{}
		for _, id := range col {
			exist[id] = struct{}{}
		}
		col = col[:0]
		for _, id := range req.ObjectIds {
			// Reorder only existing objects
			if _, ok := exist[id]; ok {
				col = append(col, id)
			}
		}
		return col
	})
}

func (s *Service) updateCollection(ctx *session.Context, contextID string, modifier func(src []string) []string) error {
	return block.DoStateCtx(s.picker, ctx, contextID, func(s *state.State, sb smartblock.SmartBlock) error {
		lst := pbtypes.GetStringList(s.Store(), storeKey)
		lst = modifier(lst)
		s.StoreSlice(storeKey, lst)
		return nil
	})
}

func (s *Service) RegisterCollection(sb smartblock.SmartBlock) {
	s.lock.Lock()
	changesCh, ok := s.collections[sb.Id()]
	if !ok {
		changesCh = make(chan []slice.Change[string])
		s.collections[sb.Id()] = changesCh
	}
	s.lock.Unlock()

	sb.AddHook(func(info smartblock.ApplyInfo) (err error) {
		// TODO: I don't like that changes converted to marshalling format and then back again
		var changes []slice.Change[string]
		for _, ch := range info.Changes {
			upd := ch.GetStoreSliceUpdate()
			if upd == nil {
				continue
			}
			if v := upd.GetAdd(); v != nil {
				changes = append(changes, slice.MakeChangeAdd(v.Ids, v.AfterId))
			} else if v := upd.GetRemove(); v != nil {
				changes = append(changes, slice.MakeChangeRemove[string](v.Ids))
			} else if v := upd.GetMove(); v != nil {
				changes = append(changes, slice.MakeChangeMove[string](v.Ids, v.AfterId))
			}
		}

		go func() {
			changesCh <- changes
		}()
		return nil
	}, smartblock.HookAfterApply)
}

func (s *Service) SubscribeForCollection(contextID string) ([]string, <-chan []slice.Change[string], error) {
	var initialObjectIDs []string
	// Waking up of collection smart block will automatically add hook used in RegisterCollection
	err := block.DoState(s.picker, contextID, func(s *state.State, sb smartblock.SmartBlock) error {
		initialObjectIDs = pbtypes.GetStringList(s.Store(), storeKey)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	s.lock.RLock()
	ch, ok := s.collections[contextID]
	s.lock.RUnlock()
	if !ok {
		return nil, nil, fmt.Errorf("collection is not registered")
	}
	return initialObjectIDs, ch, err
}
