package treemanager

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-tree-manager")

var errAppIsNotRunning = errors.New("app is not running")

type treeManager struct {
	coreService core.Service
	objectCache objectcache.Cache
	eventSender event.Sender

	onDelete func(id domain.FullID) error

	syncStarted bool
	syncerLock  sync.Mutex
}

func New() treemanager.TreeManager {
	return newTreeManager(nil)
}

func newTreeManager(onDelete func(id domain.FullID) error) *treeManager {
	return &treeManager{
		onDelete: onDelete,
	}
}

func (m *treeManager) Name() string {
	return treemanager.CName
}

type onDeleteProvider interface {
	OnDelete(id domain.FullID, workspaceRemove func() error) error
}

func (m *treeManager) Init(a *app.App) error {
	m.coreService = app.MustComponent[core.Service](a)
	m.eventSender = app.MustComponent[event.Sender](a)
	m.objectCache = app.MustComponent[objectcache.Cache](a)

	onDelete := app.MustComponent[onDeleteProvider](a).OnDelete
	m.onDelete = func(id domain.FullID) error {
		return onDelete(id, nil)
	}

	return nil
}

func (m *treeManager) Run(ctx context.Context) error {
	return nil
}

func (m *treeManager) Close(ctx context.Context) error {
	return nil
}

// GetTree should only be called by either space services or debug apis, not the client code
func (m *treeManager) GetTree(ctx context.Context, spaceId, id string) (tr objecttree.ObjectTree, err error) {
	if !m.coreService.IsStarted() {
		err = errAppIsNotRunning
		return
	}

	v, err := m.objectCache.GetObject(ctx, domain.FullID{
		SpaceID:  spaceId,
		ObjectID: id,
	})
	if err != nil {
		return
	}

	sb := v.(smartblock.SmartBlock)
	return sb.Tree(), nil
}

func (m *treeManager) MarkTreeDeleted(ctx context.Context, spaceId, treeId string) error {
	err := m.onDelete(domain.FullID{
		SpaceID:  spaceId,
		ObjectID: treeId,
	})
	if err != nil {
		log.Error("failed to execute on delete for tree", zap.Error(err))
	}
	return err
}

// DeleteTree should only be called by space services
func (m *treeManager) DeleteTree(ctx context.Context, spaceId, treeId string) (err error) {
	if !m.coreService.IsStarted() {
		return errAppIsNotRunning
	}

	obj, err := m.objectCache.GetObject(ctx, domain.FullID{
		SpaceID:  spaceId,
		ObjectID: treeId,
	})
	if err != nil {
		return
	}
	m.MarkTreeDeleted(ctx, spaceId, treeId)
	// this should be done not inside lock
	sb := obj.(smartblock.SmartBlock)
	err = sb.(source.ObjectTreeProvider).Tree().Delete()
	if err != nil {
		return
	}

	// TODO: move to block service
	m.sendOnRemoveEvent(treeId)
	err = m.objectCache.Remove(ctx, treeId)
	return
}

func (m *treeManager) sendOnRemoveEvent(ids ...string) {
	m.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfObjectRemove{
					ObjectRemove: &pb.EventObjectRemove{
						Ids: ids,
					},
				},
			},
		},
	})
}
