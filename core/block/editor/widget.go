package editor

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type WidgetObject struct {
	smartblock.SmartBlock
	basic.IHistory
	basic.Movable
	basic.Unlinkable
	basic.Updatable
	widget.Widget
	basic.DetailsSettable

	spaceIndex    spaceindex.Store
	objectCreator objectcreator.Service
}

func (f *ObjectFactory) newWidgetObject(
	sb smartblock.SmartBlock,
	objectStore spaceindex.Store,
) *WidgetObject {
	bs := basic.NewBasic(sb, objectStore, f.layoutConverter, nil)
	return &WidgetObject{
		SmartBlock:      sb,
		Movable:         bs,
		Updatable:       bs,
		DetailsSettable: bs,
		IHistory:        basic.NewHistory(sb),
		Widget:          widget.NewWidget(sb),
		spaceIndex:      objectStore,
		objectCreator:   f.objectCreator,
	}
}

func (w *WidgetObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = w.SmartBlock.Init(ctx); err != nil {
		return
	}

	var migrateBlockRecentlyEdited string
	var migrateBlockRecentlyOpened string
	var migrateBlockFavorites string

	// cleanup broken
	var removeIds []string
	_ = ctx.State.Iterate(func(b simple.Block) (isContinue bool) {
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

			if wc.Link.TargetBlockId == addr.MissingObject {
				removeIds = append(removeIds, b.Model().Id)
				return true
			}
		}
		return true
	})

	if len(removeIds) > 0 {
		// we need to avoid these situations, so lets log it
		log.Warnf("widget: removing %d broken links", len(removeIds))
	}
	for _, id := range removeIds {
		ctx.State.Unlink(id)
	}
	// now remove empty widget wrappers
	removeIds = removeIds[:0]
	_ = ctx.State.Iterate(func(b simple.Block) (isContinue bool) {
		if _, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
			if len(b.Model().GetChildrenIds()) == 0 {
				removeIds = append(removeIds, b.Model().Id)
				return true
			}
		}
		return true
	})
	if len(removeIds) > 0 {
		log.Warnf("widget: removing %d empty wrappers", len(removeIds))
	}
	for _, id := range removeIds {
		ctx.State.Unlink(id)
	}

	go w.migrateWidgets(migrateBlockRecentlyEdited, migrateBlockRecentlyOpened, migrateBlockFavorites)

	return nil
}

func (w *WidgetObject) migrateWidgets(migrateBlockRecentlyEdited string, migrateBlockRecentlyOpened string, migrateBlockFavorites string) {
	ctx := context.Background()

	if migrateBlockRecentlyEdited != "" {
		id, err := w.migrationCreateQuery(ctx, "Recently edited", "recently_edited", bundle.RelationKeyLastModifiedDate)
		if err != nil {
			log.Errorf("widget migration: failed to create Recently edited object: %v", err)
			return
		}

		w.Lock()
		st := w.NewState()
		block := st.Get(migrateBlockRecentlyEdited)
		block.Model().Content.(*model.BlockContentOfLink).Link.TargetBlockId = id
		err = w.Apply(st)
		if err != nil {
			log.Errorf("widget migration: failed to update Recently edited link: %v", err)
		}
		w.Unlock()
	}

	if migrateBlockRecentlyOpened != "" {
		id, err := w.migrationCreateQuery(ctx, "Recently opened", "recently_opened", bundle.RelationKeyLastOpenedDate)
		// TODO Fix according to
		/*
					case J.Constant.widgetId.recentEdit: {
				filters.push({ relationKey: 'lastModifiedDate', condition: I.FilterCondition.Greater, value: space.createdDate + 3 });
				break;
			};

			case J.Constant.widgetId.recentOpen: {
				filters.push({ relationKey: 'lastOpenedDate', condition: I.FilterCondition.Greater, value: 0 });
				sorts.push({ relationKey: 'lastOpenedDate', type: I.SortType.Desc });
				break;
			};
		*/
		if err != nil {
			log.Errorf("widget migration: failed to create Recently opened object: %v", err)
			return
		}

		w.Lock()
		st := w.NewState()
		block := st.Get(migrateBlockRecentlyOpened)
		block.Model().Content.(*model.BlockContentOfLink).Link.TargetBlockId = id
		err = w.Apply(st)
		if err != nil {
			log.Errorf("widget migration: failed to update Recently opened link: %v", err)
		}
		w.Unlock()
	}

	if migrateBlockFavorites != "" {
		id, err := w.migrateToCollection()
		if err != nil {
			log.Errorf("widget migration: failed to create Favorites object: %v", err)
			return
		}

		w.Lock()
		st := w.NewState()
		block := st.Get(migrateBlockFavorites)
		block.Model().Content.(*model.BlockContentOfLink).Link.TargetBlockId = id
		err = w.Apply(st)
		if err != nil {
			log.Errorf("widget migration: failed to update Recently opened link: %v", err)
		}
		w.Unlock()
	}
}

func (w *WidgetObject) migrationCreateQuery(ctx context.Context, name string, uniqueKeyInternal string, key domain.RelationKey) (string, error) {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypePage, uniqueKeyInternal)
	if err != nil {
		return "", fmt.Errorf("new unique key: %w", err)
	}

	relId, err := w.Space().DeriveObjectID(ctx, domain.MustUniqueKey(coresb.SmartBlockTypeRelation, key.String()))
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
	blockContent.Dataview.Views[0].Sorts = []*model.BlockContentDataviewSort{
		{
			Id:          bson.NewObjectId().Hex(),
			RelationKey: key.String(),
			Type:        model.BlockContentDataviewSort_Desc,
		},
	}

	template.InitTemplate(st, template.WithDataview(blockContent, false))

	id, _, err := w.objectCreator.CreateSmartBlockFromState(ctx, w.SpaceID(), []domain.TypeKey{bundle.TypeKeySet}, st)
	if errors.Is(err, treestorage.ErrTreeExists) {
		id, err = w.Space().DeriveObjectID(ctx, uk)
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

func (p *WidgetObject) migrateToCollection() (string, error) {
	var favoriteIds []string
	derivedIds := p.Space().DerivedIDs()
	err := p.Space().Do(derivedIds.Home, func(sb smartblock.SmartBlock) error {
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

	uk := domain.MustUniqueKey(coresb.SmartBlockTypePage, "old_pinned")

	id, err := p.Space().DeriveObjectID(context.Background(), uk)
	if err != nil {
		return "", fmt.Errorf("derive object id: %w", err)
	}

	err = p.addToMigratedCollection(id, favoriteIds)
	if err == nil {
		return id, nil
	} else if !errors.Is(err, treestorage.ErrUnknownTreeId) {
		return "", fmt.Errorf("add to collection: %w", err)
	}

	st := state.NewDocWithUniqueKey("", nil, uk).(*state.State)
	st.SetDetailAndBundledRelation(bundle.RelationKeyName, domain.String("Old Pinned"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
	blockContent := template.MakeDataviewContent(true, nil, nil, "")
	template.InitTemplate(st, template.WithDataview(blockContent, false))

	_, _, err = p.objectCreator.CreateSmartBlockFromState(context.Background(), p.SpaceID(), []domain.TypeKey{bundle.TypeKeyCollection}, st)
	if err != nil {
		return "", fmt.Errorf("create pinned collection: %w", err)
	}
	err = p.addToMigratedCollection(id, favoriteIds)
	if err != nil {
		return "", fmt.Errorf("add to collection after creating: %w", err)
	}
	return id, nil
}

func (p *WidgetObject) addToMigratedCollection(collId string, favoriteIds []string) error {
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

func (w *WidgetObject) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 3,
		Proc: func(st *state.State) {
			// we purposefully do not add the ALl Objects widget here(as in migration3), because for new users we don't want to auto-create it
			template.InitTemplate(st,
				template.WithEmpty,
				template.WithObjectTypes([]domain.TypeKey{bundle.TypeKeyDashboard}),
				template.WithLayout(model.ObjectType_dashboard),
				template.WithDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
			)
		},
	}
}

func replaceWidgetTarget(st *state.State, targetFrom string, targetTo string, viewId string, layout model.BlockContentWidgetLayout) {
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if wc, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
			// get child
			if len(b.Model().GetChildrenIds()) > 0 {
				child := st.Get(b.Model().GetChildrenIds()[0])
				childBlock := st.Get(child.Model().Id)
				if linkBlock, ok := childBlock.Model().Content.(*model.BlockContentOfLink); ok {
					if linkBlock.Link.TargetBlockId == targetFrom {
						targets := st.Details().Get(bundle.RelationKeyAutoWidgetTargets).StringList()
						if slices.Contains(targets, targetTo) {
							return false
						}
						targets = append(targets, targetTo)
						st.SetDetail(bundle.RelationKeyAutoWidgetTargets, domain.StringList(targets))

						linkBlock.Link.TargetBlockId = targetTo
						wc.Widget.ViewId = viewId
						wc.Widget.Layout = layout
						return false
					}
				}
			}
		}
		return true
	})
}
func (w *WidgetObject) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{
		{
			Version: 2,
			Proc: func(s *state.State) {
				spc := w.Space()
				setTypeId, err := spc.GetTypeIdByKey(context.Background(), bundle.TypeKeySet)
				if err != nil {
					return
				}
				collectionTypeId, err := spc.GetTypeIdByKey(context.Background(), bundle.TypeKeyCollection)
				if err != nil {
					return
				}
				replaceWidgetTarget(s, widget.DefaultWidgetCollection, collectionTypeId, addr.ObjectTypeAllViewId, model.BlockContentWidget_View)
				replaceWidgetTarget(s, widget.DefaultWidgetSet, setTypeId, addr.ObjectTypeAllViewId, model.BlockContentWidget_View)

			},
		},
		{
			Version: 3,
			Proc: func(s *state.State) {
				// add All Objects widget for existing spaces
				_, err := w.CreateBlock(s, &pb.RpcBlockCreateWidgetRequest{
					ContextId:    s.RootId(),
					WidgetLayout: model.BlockContentWidget_Link,
					Position:     model.Block_InnerFirst,
					TargetId:     s.RootId(),
					ViewId:       "",
					Block: &model.Block{
						Id: widget.DefaultWidgetAll, // this is correct, to avoid collisions when applied on many devices
						Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
							TargetBlockId: widget.DefaultWidgetAll,
						}},
					},
				})
				if errors.Is(err, widget.ErrWidgetAlreadyExists) {
					return
				}
				if err != nil {
					log.Warnf("all objects migration failed: %s", err.Error())
				}
			},
		},
	},
	)
}

func (w *WidgetObject) Unlink(ctx session.Context, ids ...string) (err error) {
	st := w.NewStateCtx(ctx)
	for _, id := range ids {
		if p := st.PickParentOf(id); p != nil && p.Model().GetWidget() != nil {
			st.Unlink(p.Model().Id)
		}
		st.Unlink(id)
	}
	return w.Apply(st)
}
