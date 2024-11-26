package collection

import (
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/backlinks"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var log = logging.Logger("collection-service")

type Service struct {
	lock             *sync.RWMutex
	collections      map[string]map[string]chan []string
	picker           cache.ObjectGetter
	objectStore      objectstore.ObjectStore
	backlinksUpdater backlinks.UpdateWatcher
}

func New() *Service {
	return &Service{
		lock:        &sync.RWMutex{},
		collections: map[string]map[string]chan []string{},
	}
}

func (s *Service) Init(a *app.App) (err error) {
	s.picker = app.MustComponent[cache.ObjectGetter](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.backlinksUpdater = app.MustComponent[backlinks.UpdateWatcher](a)
	return nil
}

func (s *Service) Name() string {
	return "collection"
}

func (s *Service) CollectionType() string {
	return "collection"
}

func (s *Service) Add(ctx session.Context, req *pb.RpcObjectCollectionAddRequest) error {
	var toAdd []string
	err := s.updateCollection(ctx, req.ContextId, func(col []string) []string {
		toAdd = slice.Difference(req.ObjectIds, col)
		pos := slice.FindPos(col, req.AfterId)
		if pos >= 0 {
			col = slice.Insert(col, pos+1, toAdd...)
		} else {
			col = append(toAdd, col...)
		}
		return col
	})
	if err != nil {
		return err
	}

	// we update backlinks of objects added to collection synchronously to avoid object rerender after backlinks accumulation interval
	if len(toAdd) != 0 {
		s.backlinksUpdater.FlushUpdates()
	}

	return nil
}

func (s *Service) Remove(ctx session.Context, req *pb.RpcObjectCollectionRemoveRequest) error {
	return s.updateCollection(ctx, req.ContextId, func(col []string) []string {
		col = slice.Filter(col, func(id string) bool {
			return slice.FindPos(req.ObjectIds, id) == -1
		})
		return col
	})
}

func (s *Service) Sort(ctx session.Context, req *pb.RpcObjectCollectionSortRequest) error {
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

func (s *Service) updateCollection(ctx session.Context, contextID string, modifier func(src []string) []string) error {
	return cache.DoStateCtx(s.picker, ctx, contextID, func(s *state.State, sb smartblock.SmartBlock) error {
		lst := s.GetStoreSlice(template.CollectionStoreKey)
		lst = modifier(lst)
		s.UpdateStoreSlice(template.CollectionStoreKey, lst)
		internalflag.Set{}.AddToState(s)
		return nil
	}, smartblock.KeepInternalFlags)
}

func (s *Service) collectionAddHookOnce(sb smartblock.SmartBlock) {
	sb.AddHookOnce("collection", func(info smartblock.ApplyInfo) (err error) {
		for _, ch := range info.Changes {
			if upd := ch.GetStoreSliceUpdate(); upd != nil && upd.Key == template.CollectionStoreKey {
				s.broadcast(sb.Id(), info.State.GetStoreSlice(template.CollectionStoreKey))
				return nil
			}
		}
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

	err := cache.DoState(s.picker, collectionID, func(st *state.State, sb smartblock.SmartBlock) error {
		s.collectionAddHookOnce(sb)

		initialObjectIDs = st.GetStoreSlice(template.CollectionStoreKey)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	col, ok := s.collections[collectionID]
	if !ok {
		col = map[string]chan []string{}
		s.collections[collectionID] = col
	}

	ch, ok := col[subscriptionID]
	if !ok {
		ch = make(chan []string)
		col[subscriptionID] = ch
	}

	return initialObjectIDs, ch, err
}

func (s *Service) UnsubscribeFromCollection(collectionID string, subscriptionID string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	col, ok := s.collections[collectionID]
	if !ok {
		return
	}

	ch, ok := col[subscriptionID]
	if ok {
		close(ch)
		delete(col, subscriptionID)
	}
}

func (s *Service) CreateCollection(details *types.Struct, flags []*model.InternalFlag) (coresb.SmartBlockType, *types.Struct, *state.State, error) {
	details = internalflag.PutToDetails(details, flags)

	newState := state.NewDoc("", nil).NewState().SetDetails(details)

	tmpls := []template.StateTransformer{}

	blockContent := template.MakeDataviewContent(true, nil, nil)
	tmpls = append(tmpls,
		template.WithDataview(blockContent, false),
	)
	template.InitTemplate(newState, tmpls...)

	return coresb.SmartBlockTypePage, newState.CombinedDetails(), newState, nil
}

func (s *Service) ObjectToCollection(id string) error {
	return cache.DoState(s.picker, id, func(st *state.State, b basic.CommonOperations) error {
		sb := b.(smartblock.SmartBlock)
		s.setDefaultObjectTypeToViews(sb.SpaceID(), st)
		return b.SetObjectTypesInState(st, []domain.TypeKey{bundle.TypeKeyCollection}, true)
	})
}

func (s *Service) setDefaultObjectTypeToViews(spaceId string, st *state.State) {
	if !lo.Contains(st.ParentState().ObjectTypeKeys(), bundle.TypeKeySet) {
		return
	}

	setOfValue := pbtypes.GetStringList(st.ParentState().Details(), bundle.RelationKeySetOf.String())
	if len(setOfValue) == 0 {
		return
	}

	if s.isNotCreatableType(spaceId, setOfValue[0]) {
		return
	}

	dataviewBlock := st.Get(state.DataviewBlockID)
	if dataviewBlock == nil {
		return
	}
	content, ok := dataviewBlock.Model().Content.(*model.BlockContentOfDataview)
	if !ok {
		return
	}

	for _, view := range content.Dataview.Views {
		view.DefaultObjectTypeId = setOfValue[0]
	}
}

func (s *Service) isNotCreatableType(spaceId string, id string) bool {
	uk, err := s.objectStore.SpaceIndex(spaceId).GetUniqueKeyById(id)
	if err != nil {
		return true
	}
	if uk.SmartblockType() != coresb.SmartBlockTypeObjectType {
		return true
	}
	return lo.Contains(append(bundle.InternalTypes, bundle.TypeKeyObjectType), domain.TypeKey(uk.InternalKey()))
}
