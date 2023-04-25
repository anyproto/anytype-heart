package collection

import (
	"fmt"
	"sync"

	"github.com/anytypeio/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var log = logging.Logger("collection-service")

type Service struct {
	lock        *sync.RWMutex
	collections map[string]map[string]chan []string

	picker        block.Picker
	objectStore   objectstore.ObjectStore
	objectCreator ObjectCreator
	objectDeleter ObjectDeleter
}

type ObjectCreator interface {
	CreateObject(req block.DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error)
}

type ObjectDeleter interface {
	DeleteObject(id string) (err error)
}

func New(
	picker block.Picker,
	store objectstore.ObjectStore,
	objectCreator ObjectCreator,
	objectDeleter ObjectDeleter,
) *Service {
	return &Service{
		picker:        picker,
		objectStore:   store,
		objectCreator: objectCreator,
		objectDeleter: objectDeleter,
		lock:          &sync.RWMutex{},
		collections:   map[string]map[string]chan []string{},
	}
}

func (s *Service) Init(a *app.App) (err error) {
	return nil
}

func (s *Service) Name() string {
	return "collection"
}

const StoreKey = "objects"

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
		lst := pbtypes.GetStringList(s.Store(), StoreKey)
		lst = modifier(lst)
		s.StoreSlice(StoreKey, lst)
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
		for _, ch := range info.Changes {
			if upd := ch.GetStoreSliceUpdate(); upd != nil && upd.Key == StoreKey {
				s.broadcast(sb.Id(), pbtypes.GetStringList(info.State.Store(), StoreKey))
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
	// Waking up of collection smart block will automatically add hook used in RegisterCollection
	err := block.DoState(s.picker, collectionID, func(s *state.State, sb smartblock.SmartBlock) error {
		initialObjectIDs = pbtypes.GetStringList(s.Store(), StoreKey)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	col, ok := s.collections[collectionID]
	if !ok {
		return nil, nil, fmt.Errorf("collection is not registered")
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

	ch := col[subscriptionID]
	close(ch)
	delete(col, subscriptionID)
}

func (s *Service) CreateCollection(details *types.Struct, flags []*model.InternalFlag) (coresb.SmartBlockType, *types.Struct, *state.State, error) {
	details = internalflag.PutToDetails(details, flags)

	newState := state.NewDoc("", nil).NewState()

	tmpls := []template.StateTransformer{
		template.WithRequiredRelations(),
	}

	blockContent := MakeDataviewContent()
	tmpls = append(tmpls,
		template.WithDataview(*blockContent, false),
	)
	template.InitTemplate(newState, tmpls...)

	return coresb.SmartBlockTypeCollection, details, newState, nil
}

func MakeDataviewContent() *model.BlockContentOfDataview {
	relations := []*model.RelationLink{
		{
			Format: model.RelationFormat_shorttext,
			Key:    bundle.RelationKeyName.String(),
		},
	}
	viewRelations := []*model.BlockContentDataviewRelation{
		{
			Key:       bundle.RelationKeyName.String(),
			IsVisible: true,
		},
	}
	for _, relKey := range dataview.DefaultDataviewRelations {
		if pbtypes.HasRelationLink(relations, relKey.String()) {
			continue
		}
		rel := bundle.MustGetRelation(relKey)
		if rel.Hidden {
			continue
		}
		relations = append(relations, &model.RelationLink{
			Format: rel.Format,
			Key:    rel.Key,
		})
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: false})
	}

	blockContent := &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			RelationLinks: relations,
			Views: []*model.BlockContentDataviewView{
				{
					Id:   bson.NewObjectId().Hex(),
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Filters:   nil,
					Relations: viewRelations,
				},
			},
		},
	}
	return blockContent
}

func (s *Service) ObjectToCollection(id string) (string, error) {
	var (
		details      *types.Struct
		dvBlock      *model.Block
		typesFromSet []string
	)
	if err := block.Do(s.picker, id, func(sb smartblock.SmartBlock) error {
		details = pbtypes.CopyStruct(sb.Details())

		st := sb.NewState()
		if layout, ok := st.Layout(); ok && layout == model.ObjectType_note {
			textBlock, err := st.GetFirstTextBlock()
			if err != nil {
				return err
			}
			if textBlock != nil {
				details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(textBlock.Model().GetText().Text)
			}
		}

		b := st.Pick(template.DataviewBlockId)
		if b != nil {
			typesFromSet = pbtypes.GetStringList(details, bundle.RelationKeySetOf.String())
			delete(details.Fields, bundle.RelationKeySetOf.String())
			pbtypes.UpdateStringList(details, bundle.RelationKeyFeaturedRelations.String(), func(fr []string) []string {
				return slice.Remove(fr, bundle.RelationKeySetOf.String())
			})
			dvBlock = b.Model()
		}
		return nil
	}); err != nil {
		return "", err
	}
	// cleanup details
	delete(details.Fields, bundle.RelationKeyLayout.String())
	delete(details.Fields, bundle.RelationKeyType.String())

	newID, _, err := s.objectCreator.CreateObject(&pb.RpcObjectCreateRequest{
		Details: details,
	}, bundle.TypeKeyCollection)
	if err != nil {
		return "", err
	}

	if dvBlock != nil {

		err = block.DoState(s.picker, newID, func(st *state.State, sb smartblock.SmartBlock) error {
			dvBlock.Id = template.DataviewBlockId
			b := simple.New(dvBlock)
			st.Set(b)

			recs, _, qErr := s.objectStore.Query(nil, database.Query{
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyType.String(),
						Condition:   model.BlockContentDataviewFilter_In,
						Value:       pbtypes.StringList(typesFromSet),
					},
				},
			})
			if qErr != nil {
				return fmt.Errorf("can't get records for collection: %w", err)
			}
			ids := make([]string, 0, len(recs))
			for _, r := range recs {
				ids = append(ids, pbtypes.GetString(r.Details, bundle.RelationKeyId.String()))
			}
			st.StoreSlice(StoreKey, ids)
			return nil
		})
		if err != nil {
			return newID, fmt.Errorf("can't update dataview block: %w", err)
		}
	}

	res, err := s.objectStore.GetWithLinksInfoByID(id)
	if err != nil {
		return "", err
	}
	for _, il := range res.Links.Inbound {
		err = block.Do(s.picker, il.Id, func(b basic.CommonOperations) error {
			return b.ReplaceLink(id, newID)
		})
		if err != nil {
			return "", fmt.Errorf("replace link in %s: %w", il.Id, err)
		}
	}
	err = s.objectDeleter.DeleteObject(id)
	if err != nil {
		// intentionally do not return error here
		log.Errorf("failed to delete object after conversion to set: %s", err.Error())
	}

	return newID, nil
}
