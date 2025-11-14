package widgetmigration

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
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
	DeleteGarbage(spaceId string) error
}

type deleter interface {
	DeleteObjectByFullID(id domain.FullID) error
}

type service struct {
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	objectGetter  cache.ObjectGetter
	objectCreator objectcreator.Service
	spaceService  space.Service
	objectStore   objectstore.ObjectStore
	deleter       deleter
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
	s.deleter = app.MustComponent[deleter](a)
	return nil
}

func (s *service) DeleteGarbage(spaceId string) error {
	spc, err := s.spaceService.Get(s.componentCtx, spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}

	for _, cs := range []struct {
		uk domain.UniqueKey
	}{
		{
			uk: domain.MustUniqueKey(coresb.SmartBlockTypePage, internalKeyOldPinned),
		},
		{
			uk: domain.MustUniqueKey(coresb.SmartBlockTypePage, internalKeyRecentlyOpened),
		},
		{
			uk: domain.MustUniqueKey(coresb.SmartBlockTypePage, internalKeyRecentlyEdited),
		},
	} {
		go func() {
			id, err := spc.DeriveObjectID(s.componentCtx, cs.uk)
			if err != nil {
				fmt.Println("DERIVE OBJECT ID", err)
				return
			}

			err = s.deleter.DeleteObjectByFullID(domain.FullID{
				ObjectID: id,
				SpaceID:  spaceId,
			})
			if err != nil {
				fmt.Println("DELETE OBJECT", err)
				return
			}
			fmt.Println("DELETED", id, cs.uk.InternalKey(), id)
		}()
	}
	return nil
}

func (s *service) MigrateWidgets(objectId string) error {
	return nil
}

func (s *service) AddToOldPinnedCollection(space smartblock.Space, favoriteIds []string) error {
	return nil
}

func (s *service) addToMigratedCollection(space smartblock.Space, collId string, favoriteIds []string) error {
	return nil
}
