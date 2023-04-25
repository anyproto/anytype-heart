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
	collections map[string]map[string]chan []string
}

func New() *Service {
	return &Service{
		lock:        &sync.RWMutex{},
		collections: map[string]map[string]chan []string{},
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
	col, ok := s.collections[sb.Id()]
	if !ok {
		col = map[string]chan []string{}
		s.collections[sb.Id()] = col
	}
	s.lock.Unlock()

	sb.AddHook(func(info smartblock.ApplyInfo) (err error) {
		go func() {
			s.broadcast(sb.Id(), pbtypes.GetStringList(info.State.Store(), storeKey))
		}()
		return nil
	}, smartblock.HookAfterApply)
}

func (s *Service) broadcast(collectionID string, objectIDs []string) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	for _, ch := range s.collections[collectionID] {
		ch <- objectIDs
	}
}

type Subscription struct {
	objectsCh chan []string
	closeCh   chan struct{}
}

func (s *Subscription) Chan() <-chan []string {
	return s.objectsCh
}

func (s *Subscription) Close() {
	close(s.closeCh)
}

func (s *Service) SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error) {
	var initialObjectIDs []string
	// Waking up of collection smart block will automatically add hook used in RegisterCollection
	err := block.DoState(s.picker, collectionID, func(s *state.State, sb smartblock.SmartBlock) error {
		initialObjectIDs = pbtypes.GetStringList(s.Store(), storeKey)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	s.lock.Lock()
	col, ok := s.collections[collectionID]
	if !ok {
		return nil, nil, fmt.Errorf("collection is not registered")
	}

	ch, ok := col[subscriptionID]
	if !ok {
		ch = make(chan []string)
		col[subscriptionID] = ch
	}
	s.lock.Unlock()

	return initialObjectIDs, ch, err
}

func (s *Service) UnsubscribeFromCollection(collectionID string, subscriptionID string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	col, ok := s.collections[collectionID]
	if !ok {
		return
	}

	ch := col[subscriptionID]
	close(ch)
	delete(col, subscriptionID)
}
