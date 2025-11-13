package collection

import (
	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/collection"
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
)

var log = logging.Logger("collection-service")

type Service struct {
	picker      cache.ObjectGetter
	objectStore objectstore.ObjectStore
}

func New() *Service {
	return &Service{}
}

func (s *Service) Init(a *app.App) (err error) {
	s.picker = app.MustComponent[cache.ObjectGetter](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (s *Service) Name() string {
	return "collection"
}

func (s *Service) CollectionType() string {
	return "collection"
}

func (s *Service) Add(ctx session.Context, req *pb.RpcObjectCollectionAddRequest) error {
	return cache.Do[collection.Collection](s.picker, req.ContextId, func(coll collection.Collection) error {
		return coll.AddToCollection(ctx, req)
	})
}

func (s *Service) Remove(ctx session.Context, req *pb.RpcObjectCollectionRemoveRequest) error {
	return cache.Do[collection.Collection](s.picker, req.ContextId, func(coll collection.Collection) error {
		return coll.RemoveFromCollection(ctx, req)
	})
}

func (s *Service) Sort(ctx session.Context, req *pb.RpcObjectCollectionSortRequest) error {
	return cache.Do[collection.Collection](s.picker, req.ContextId, func(coll collection.Collection) error {
		return coll.ReorderCollection(ctx, req)
	})
}

func (s *Service) SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error) {
	var initialIds []string
	var ch <-chan []string

	err := cache.Do[collection.Collection](s.picker, collectionID, func(coll collection.Collection) error {
		var err error
		initialIds, ch, err = coll.SubscribeForCollection(subscriptionID)
		return err
	})
	return initialIds, ch, err
}

func (s *Service) UnsubscribeFromCollection(collectionID string, subscriptionID string) error {
	return cache.Do[collection.Collection](s.picker, collectionID, func(coll collection.Collection) error {
		coll.UnsubscribeFromCollection(subscriptionID)
		return nil
	})
}

func (s *Service) CreateCollection(details *domain.Details, flags []*model.InternalFlag) (coresb.SmartBlockType, *domain.Details, *state.State, error) {
	details = internalflag.PutToDetails(details, flags)

	newState := state.NewDoc("", nil).NewState().SetDetails(details)

	blockContent := template.MakeDataviewContent(true, nil, nil, nil)
	template.InitTemplate(newState, template.WithDataview(blockContent, false))

	return coresb.SmartBlockTypePage, newState.CombinedDetails(), newState, nil
}

func (s *Service) ObjectToCollection(id string) error {
	return cache.DoState(s.picker, id, func(st *state.State, b basic.CommonOperations) error {
		sb := b.(smartblock.SmartBlock)
		s.setDefaultObjectTypeToViews(sb.SpaceID(), st)
		return b.SetObjectTypesInState(st, []domain.TypeKey{bundle.TypeKeyCollection}, true)
	}, smartblock.KeepInternalFlags)
}

func (s *Service) setDefaultObjectTypeToViews(spaceId string, st *state.State) {
	if !lo.Contains(st.ParentState().ObjectTypeKeys(), bundle.TypeKeySet) {
		return
	}

	setOfValue := st.ParentState().Details().GetStringList(bundle.RelationKeySetOf)
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
