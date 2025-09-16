package editor

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

// required relations for archive beside the bundle.RequiredInternalRelations
var dashboardRequiredRelations = []domain.RelationKey{}

type Dashboard struct {
	smartblock.SmartBlock
	basic.AllOperations
	blockcollection.Collection

	objectCreator objectcreator.Service

	objectStore spaceindex.Store
}

func (f *ObjectFactory) newDashboard(sb smartblock.SmartBlock, objectStore spaceindex.Store) *Dashboard {
	return &Dashboard{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, objectStore, f.layoutConverter, nil),
		Collection:    blockcollection.NewCollection(sb, objectStore),
		objectStore:   objectStore,
		objectCreator: f.objectCreator,
	}
}

func (p *Dashboard) Init(ctx *smartblock.InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, dashboardRequiredRelations...)
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.AddHook(p.updateObjects, smartblock.HookAfterApply)
	return p.updateObjects(smartblock.ApplyInfo{})

}

func (p *Dashboard) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 2,
		Proc: func(st *state.State) {
			template.InitTemplate(st,
				template.WithObjectTypes([]domain.TypeKey{bundle.TypeKeyDashboard}),
				template.WithLayout(model.ObjectType_dashboard),
				template.WithEmpty,
				template.WithDetailName("Home"),
				template.WithDetailIconEmoji("🏠"),
				template.WithNoDuplicateLinks(),
				template.WithForcedDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
			)
		},
	}
}

func (p *Dashboard) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{{
		Version: 2,
		Proc:    template.WithForcedDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
	}})
}

func (p *Dashboard) updateObjects(info smartblock.ApplyInfo) (err error) {
	favoritedIds, err := p.GetIds()
	if err != nil {
		return
	}

	go func() {
		mErr := p.migrateToCollection(favoritedIds)
		if mErr != nil {
			log.Errorf("favorite: can't migrate to collection: %v", mErr)
		}

		uErr := p.updateInStore(favoritedIds)
		if uErr != nil {
			log.Errorf("favorite: can't update in store: %v", uErr)
		}
	}()

	return nil
}

func (p *Dashboard) migrateToCollection(favoriteIds []string) error {
	uk := domain.MustUniqueKey(coresb.SmartBlockTypePage, "old_pinned")

	id, err := p.Space().DeriveObjectID(context.Background(), uk)
	if err != nil {
		return fmt.Errorf("derive object id: %w", err)
	}

	err = p.addToMigratedCollection(id, favoriteIds)
	if err == nil {
		return nil
	} else if !errors.Is(err, treestorage.ErrUnknownTreeId) {
		return fmt.Errorf("add to collection: %w", err)
	}

	st := state.NewDocWithUniqueKey("", nil, uk).(*state.State)
	st.SetDetailAndBundledRelation(bundle.RelationKeyName, domain.String("Old Pinned"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
	blockContent := template.MakeDataviewContent(true, nil, nil, "")
	template.InitTemplate(st, template.WithDataview(blockContent, false))

	_, _, err = p.objectCreator.CreateSmartBlockFromState(context.Background(), p.SpaceID(), []domain.TypeKey{bundle.TypeKeyCollection}, st)
	if err != nil {
		return fmt.Errorf("create pinned collection: %w", err)
	}
	err = p.addToMigratedCollection(id, favoriteIds)
	if err != nil {
		return fmt.Errorf("add to collection after creating: %w", err)
	}
	return nil
}

func (p *Dashboard) addToMigratedCollection(collId string, favoriteIds []string) error {
	return p.Space().Do(collId, func(sb smartblock.SmartBlock) error {
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

func (p *Dashboard) updateInStore(favoritedIds []string) error {
	records, err := p.objectStore.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyIsFavorite,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
		},
	})
	if err != nil {
		return err
	}
	var storeFavoritedIds = make([]string, 0, len(records))
	for _, rec := range records {
		storeFavoritedIds = append(storeFavoritedIds, rec.Details.GetString(bundle.RelationKeyId))
	}

	removedIds, addedIds := slice.DifferenceRemovedAdded(storeFavoritedIds, favoritedIds)
	for _, removedId := range removedIds {
		go func(id string) {
			if err := p.ModifyLocalDetails(id, func(current *domain.Details) (*domain.Details, error) {
				if current == nil {
					current = domain.NewDetails()
				}
				current.SetBool(bundle.RelationKeyIsFavorite, false)
				return current, nil
			}); err != nil {
				logFavoriteError(err)
			}
		}(removedId)
	}
	for _, addedId := range addedIds {
		go func(id string) {
			if err := p.ModifyLocalDetails(id, func(current *domain.Details) (*domain.Details, error) {
				if current == nil {
					current = domain.NewDetails()
				}
				current.SetBool(bundle.RelationKeyIsFavorite, true)
				return current, nil
			}); err != nil {
				logFavoriteError(err)
			}
		}(addedId)
	}
	return nil
}

func logFavoriteError(err error) {
	if errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) {
		return
	}
	if errors.Is(err, treestorage.ErrUnknownTreeId) {
		return
	}
	log.Errorf("favorite: can't set detail to object: %v", err)
}
