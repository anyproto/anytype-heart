package backlinks

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/cheggaaa/mb"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const CName = "backlinks-update-watcher"

var log = logging.Logger(CName)

type backlinksUpdater interface {
	SubscribeBacklinksUpdate(callback func(info objectstore.BacklinksUpdateInfo))
}

type UpdateWatcher struct {
	app.ComponentRunnable

	updater      backlinksUpdater
	store        objectstore.ObjectStore
	resolver     idresolver.Resolver
	spaceService space.Service

	infoBatch *mb.MB
}

func New() app.Component {
	return &UpdateWatcher{}
}

func (uw *UpdateWatcher) Name() string {
	return CName
}

func (uw *UpdateWatcher) Init(a *app.App) error {
	uw.updater = app.MustComponent[backlinksUpdater](a)
	uw.store = app.MustComponent[objectstore.ObjectStore](a)
	uw.resolver = app.MustComponent[idresolver.Resolver](a)
	uw.spaceService = app.MustComponent[space.Service](a)
	uw.infoBatch = mb.New(0)
	return nil
}

func (uw *UpdateWatcher) Close(context.Context) error {
	if err := uw.infoBatch.Close(); err != nil {
		log.Errorf("failed to close message batch: %v", err)
	}
	return nil
}

func (uw *UpdateWatcher) Run(context.Context) error {
	uw.updater.SubscribeBacklinksUpdate(func(info objectstore.BacklinksUpdateInfo) {
		if err := uw.infoBatch.Add(info); err != nil {
			log.With("objectID", info.Id).Errorf("failed to add backlinks update info to message batch: %v", err)
		}
	})

	go uw.backlinksUpdateHandler()
	return nil
}

func (uw *UpdateWatcher) backlinksUpdateHandler() {
	for {
		msgs := uw.infoBatch.Wait()
		if len(msgs) == 0 {
			return
		}
		for _, msg := range msgs {
			info, ok := msg.(objectstore.BacklinksUpdateInfo)
			if !ok || hasSelfLinks(info) {
				continue
			}
			uw.updateBackLinksInObjects(info)
		}
	}
}

func (uw *UpdateWatcher) updateBackLinksInObjects(info objectstore.BacklinksUpdateInfo) {
	spaceId, err := uw.resolver.ResolveSpaceID(info.Id)
	if err != nil {
		log.With("objectID", info.Id).Errorf("failed to resolve space id for object %s: %v", info.Id, err)
		return
	}
	spc, err := uw.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		log.With("objectID", info.Id).Errorf("failed get space %s: %v", spaceId, err)
		return
	}

	addBacklink := func(current *types.Struct) (*types.Struct, error) {
		if current == nil || current.Fields == nil {
			return nil, objectstore.ErrDetailsNotChanged
		}
		backlinks := pbtypes.GetStringList(current, bundle.RelationKeyBacklinks.String())
		if lo.Contains(backlinks, info.Id) {
			return nil, objectstore.ErrDetailsNotChanged
		}
		backlinks = append(backlinks, info.Id)
		current.Fields[bundle.RelationKeyBacklinks.String()] = pbtypes.StringList(backlinks)
		return current, nil
	}

	removeBacklink := func(current *types.Struct) (*types.Struct, error) {
		if current == nil || current.Fields == nil {
			return nil, objectstore.ErrDetailsNotChanged
		}
		backlinks := pbtypes.GetStringList(current, bundle.RelationKeyBacklinks.String())
		newBacklinks := slice.Remove(backlinks, info.Id)
		if len(backlinks) == len(newBacklinks) {
			return nil, objectstore.ErrDetailsNotChanged
		}
		current.Fields[bundle.RelationKeyBacklinks.String()] = pbtypes.StringList(newBacklinks)
		return current, nil
	}

	for _, modification := range []struct {
		ids      []string
		modifier func(details *types.Struct) (*types.Struct, error)
	}{
		{info.Added, addBacklink},
		{info.Removed, removeBacklink},
	} {
		for _, id := range modification.ids {
			err = spc.DoLockedIfNotExists(id, func() error {
				return uw.store.ModifyObjectDetails(id, modification.modifier)
			})
			if err == nil {
				continue
			}
			if !errors.Is(err, ocache.ErrExists) {
				log.With("objectID", info.Id).Errorf("failed to update backlinks for not cached object %s: %v", id, err)
			}
			if err = spc.Do(id, func(b smartblock.SmartBlock) error {
				if cr, ok := b.(source.ChangeReceiver); ok {
					return cr.StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error) {
						return d.NewState(), nil, nil
					})
				}
				return b.Apply(b.NewState(), smartblock.KeepInternalFlags)
			}); err != nil {
				log.With("objectID", info.Id).Errorf("failed to update backlinks for object %s: %v", id, err)
			}
		}
	}
}

func hasSelfLinks(info objectstore.BacklinksUpdateInfo) bool {
	for _, link := range info.Added {
		if link == info.Id {
			return true
		}
	}
	for _, link := range info.Removed {
		if link == info.Id {
			return true
		}
	}
	return false
}
