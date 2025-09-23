package widgetmigration

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "core.block.editor.widgetmigration"

const internalKeyRecentlyEdited = "recently_edited"
const internalKeyRecentlyOpened = "recently_opened"
const internalKeyOldPinned = "old_pinned"

var log = logging.Logger(CName)

type Service interface {
	app.ComponentRunnable

	MigrateWidgets(objectId string) error
	AddToOldPinnedCollection(space smartblock.Space, favoriteIds []string) error
}

type service struct {
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	objectGetter  cache.ObjectGetter
	objectCreator objectcreator.Service
	spaceService  space.Service
	objectStore   objectstore.ObjectStore
}

func New() Service {
	return &service{}

}

func (s *service) Name() string {
	return CName
}

func (s *service) Run(ctx context.Context) error {
	return nil
}

func (s *service) Close(ctx context.Context) error {
	if s.componentCtxCancel != nil {
		s.componentCtxCancel()
	}
	return nil
}

func (s *service) Init(a *app.App) error {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	s.objectCreator = app.MustComponent[objectcreator.Service](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (s *service) MigrateWidgets(objectId string) error {
	return nil

	return cache.Do(s.objectGetter, objectId, func(sb smartblock.SmartBlock) error {
		var migrateBlockRecentlyEdited string
		var migrateBlockRecentlyOpened string
		var migrateBlockFavorites string

		st := sb.NewState()
		_ = st.Iterate(func(b simple.Block) (isContinue bool) {
			if wc, ok := b.Model().Content.(*model.BlockContentOfLink); ok {
				if wc.Link.TargetBlockId == widget.DefaultWidgetRecentlyEdited {
					migrateBlockRecentlyEdited = b.Model().Id
				}
				if wc.Link.TargetBlockId == widget.DefaultWidgetRecentlyOpened {
					migrateBlockRecentlyOpened = b.Model().Id
				}
				if wc.Link.TargetBlockId == widget.DefaultWidgetFavorite {
					migrateBlockFavorites = b.Model().Id
				}
			}
			return true
		})
		s.migrateWidgets(sb, migrateBlockRecentlyEdited, migrateBlockRecentlyOpened, migrateBlockFavorites)
		return nil
	})
}

func (s *service) migrateWidgets(widget smartblock.SmartBlock, migrateBlockRecentlyEdited string, migrateBlockRecentlyOpened string, migrateBlockFavorites string) {
	st := widget.NewState()
	var needApply bool

	if migrateBlockRecentlyEdited != "" {
		id, err := s.migrationCreateQuery(s.componentCtx, widget, "Recently edited", internalKeyRecentlyEdited, bundle.RelationKeyLastModifiedDate, func(view *model.BlockContentDataviewView) {
			spaceViewId, err := s.spaceService.SpaceViewId(widget.SpaceID())
			if err != nil {
				log.Errorf("widget migration: failed to get space view id: %v", err)
				return
			}
			techSpaceId := s.spaceService.TechSpaceId()

			spaceViewDetails, err := s.objectStore.SpaceIndex(techSpaceId).GetDetails(spaceViewId)
			if err != nil {
				log.Errorf("widget migration: failed to get space view details: %v", err)
				return
			}

			view.Filters = []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLastModifiedDate.String(),
					Condition:   model.BlockContentDataviewFilter_Greater,
					Value:       pbtypes.Int64(spaceViewDetails.Get(bundle.RelationKeyCreatedDate).Int64() + 60),
				},
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_NotIn,
					Value:       pbtypes.IntList(int(model.ObjectType_relation), int(model.ObjectType_relationOption), int(model.ObjectType_dashboard), int(model.ObjectType_space), int(model.ObjectType_spaceView)),
				},
				{
					RelationKey: "type.uniqueKey",
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.String(bundle.TypeKeyTemplate.URL()),
				},
			}
		})
		if err != nil {
			log.Errorf("widget migration: failed to create Recently edited object: %v", err)
			return
		}

		block := st.Get(migrateBlockRecentlyEdited)
		block.Model().Content.(*model.BlockContentOfLink).Link.TargetBlockId = id
		needApply = true
	}

	if migrateBlockRecentlyOpened != "" {

		id, err := s.migrationCreateQuery(s.componentCtx, widget, "Recently opened", internalKeyRecentlyOpened, bundle.RelationKeyLastOpenedDate, func(view *model.BlockContentDataviewView) {
			view.Filters = []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLastOpenedDate.String(),
					Condition:   model.BlockContentDataviewFilter_Greater,
					Value:       pbtypes.Int64(0),
				},
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_NotIn,
					Value:       pbtypes.IntList(int(model.ObjectType_relation), int(model.ObjectType_relationOption), int(model.ObjectType_dashboard), int(model.ObjectType_space), int(model.ObjectType_spaceView)),
				},
				{
					RelationKey: "type.uniqueKey",
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.String(bundle.TypeKeyTemplate.URL()),
				},
			}

			var ignoreIds []string
			recentlyOpenedId, err := widget.Space().DeriveObjectID(s.componentCtx, domain.MustUniqueKey(coresb.SmartBlockTypePage, internalKeyRecentlyOpened))
			if err == nil {
				ignoreIds = append(ignoreIds, recentlyOpenedId)
			}
			recentlyEditedId, err := widget.Space().DeriveObjectID(s.componentCtx, domain.MustUniqueKey(coresb.SmartBlockTypePage, internalKeyRecentlyEdited))
			if err == nil {
				ignoreIds = append(ignoreIds, recentlyEditedId)
			}
			if len(ignoreIds) > 0 {
				view.Filters = append(view.Filters, &model.BlockContentDataviewFilter{
					RelationKey: bundle.RelationKeyId.String(),
					Condition:   model.BlockContentDataviewFilter_NotIn,
					Value:       pbtypes.StringList(ignoreIds),
				})
			}

		})
		if err != nil {
			log.Errorf("widget migration: failed to create Recently opened object: %v", err)
			return
		}

		block := st.Get(migrateBlockRecentlyOpened)
		block.Model().Content.(*model.BlockContentOfLink).Link.TargetBlockId = id
		needApply = true
	}

	if migrateBlockFavorites != "" {
		id, err := s.migrateToCollection(widget)
		if err != nil {
			log.Errorf("widget migration: failed to create Favorites object: %v", err)
			return
		}

		block := st.Get(migrateBlockFavorites)
		block.Model().Content.(*model.BlockContentOfLink).Link.TargetBlockId = id
		needApply = true
	}

	if needApply {
		err := widget.Apply(st)
		if err != nil {
			log.Errorf("widget migration: failed to update Recently opened link: %v", err)
		}
	}
}

func (s *service) migrationCreateQuery(ctx context.Context, widget smartblock.SmartBlock, name string, uniqueKeyInternal string, key domain.RelationKey, updateView func(view *model.BlockContentDataviewView)) (string, error) {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypePage, uniqueKeyInternal)
	if err != nil {
		return "", fmt.Errorf("new unique key: %w", err)
	}

	relId, err := widget.Space().DeriveObjectID(ctx, domain.MustUniqueKey(coresb.SmartBlockTypeRelation, key.String()))
	if err != nil {
		return "", fmt.Errorf("derive relation id: %w", err)
	}

	st := state.NewDocWithUniqueKey("", nil, uk).(*state.State)
	st.SetDetailAndBundledRelation(bundle.RelationKeyName, domain.String(name))
	st.SetDetailAndBundledRelation(bundle.RelationKeySetOf, domain.StringList([]string{relId}))
	st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_set)))
	blockContent := template.MakeDataviewContent(false, nil, []*model.RelationLink{
		{
			Key:    key.String(),
			Format: model.RelationFormat_date,
		},
	}, "")

	view := blockContent.Dataview.Views[0]
	view.Sorts = []*model.BlockContentDataviewSort{
		{
			Id:          bson.NewObjectId().Hex(),
			RelationKey: key.String(),
			Type:        model.BlockContentDataviewSort_Desc,
		},
	}
	updateView(view)

	template.InitTemplate(st, template.WithDataview(blockContent, false))

	id, _, err := s.objectCreator.CreateSmartBlockFromState(ctx, widget.SpaceID(), []domain.TypeKey{bundle.TypeKeySet}, st)
	if errors.Is(err, treestorage.ErrTreeExists) {
		id, err = widget.Space().DeriveObjectID(ctx, uk)
		if err != nil {
			return "", fmt.Errorf("derive object id: %w", err)
		}
		return id, err
	}

	if err != nil {
		return "", fmt.Errorf("create object: %w", err)
	}

	return id, err
}

func (s *service) migrateToCollection(widget smartblock.SmartBlock) (string, error) {
	var favoriteIds []string
	derivedIds := widget.Space().DerivedIDs()
	err := widget.Space().Do(derivedIds.Home, func(sb smartblock.SmartBlock) error {
		coll, ok := sb.(blockcollection.Collection)
		if !ok {
			return fmt.Errorf("object is not a block collection")
		}

		var err error
		favoriteIds, err = coll.GetIds()
		return err
	})
	if err != nil {
		return "", fmt.Errorf("get ids: %w", err)
	}

	uk := domain.MustUniqueKey(coresb.SmartBlockTypePage, internalKeyOldPinned)

	id, err := widget.Space().DeriveObjectID(context.Background(), uk)
	if err != nil {
		return "", fmt.Errorf("derive object id: %w", err)
	}

	err = s.addToMigratedCollection(widget.Space(), id, favoriteIds)
	if err == nil {
		return id, nil
	} else if !errors.Is(err, treestorage.ErrUnknownTreeId) {
		return "", fmt.Errorf("add to collection: %w", err)
	}

	st := state.NewDocWithUniqueKey("", nil, uk).(*state.State)
	st.SetDetailAndBundledRelation(bundle.RelationKeyName, domain.String("Old pinned"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
	blockContent := template.MakeDataviewContent(true, nil, nil, "")
	blockContent.Dataview.Views[0].Sorts = []*model.BlockContentDataviewSort{}
	template.InitTemplate(st, template.WithDataview(blockContent, false))

	_, _, err = s.objectCreator.CreateSmartBlockFromState(context.Background(), widget.SpaceID(), []domain.TypeKey{bundle.TypeKeyCollection}, st)
	if err != nil {
		return "", fmt.Errorf("create pinned collection: %w", err)
	}
	err = s.addToMigratedCollection(widget.Space(), id, favoriteIds)
	if err != nil {
		return "", fmt.Errorf("add to collection after creating: %w", err)
	}
	return id, nil
}

func (s *service) AddToOldPinnedCollection(space smartblock.Space, favoriteIds []string) error {
	return nil

	collId, err := space.DeriveObjectID(context.Background(), domain.MustUniqueKey(coresb.SmartBlockTypePage, internalKeyOldPinned))
	if err != nil {
		return fmt.Errorf("derive object id: %w", err)
	}
	return s.addToMigratedCollection(space, collId, favoriteIds)
}

func (s *service) addToMigratedCollection(space smartblock.Space, collId string, favoriteIds []string) error {
	return space.Do(collId, func(sb smartblock.SmartBlock) error {
		if sb.LocalDetails().GetBool(bundle.RelationKeyIsDeleted) {
			return nil
		}
		coll, ok := sb.(collection.Collection)
		if !ok {
			return fmt.Errorf("object is not a collection")
		}
		ids := coll.ListIdsFromCollection()
		var toAdd []string
		for _, id := range favoriteIds {
			if !slices.Contains(ids, id) {
				toAdd = append(toAdd, id)
			}
		}
		if len(toAdd) == 0 {
			return nil
		}
		return coll.AddToCollection(nil, &pb.RpcObjectCollectionAddRequest{
			AfterId:   "",
			ObjectIds: toAdd,
		})
	})
}
