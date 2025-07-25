package treemanager

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
)

var log = logging.Logger("anytype-mw-tree-manager")

type treeManager struct {
	eventSender  event.Sender
	spaceService space.Service

	beforeDelete func(id domain.FullID) error
}

func New() treemanager.TreeManager {
	return newTreeManager(nil)
}

func newTreeManager(beforeDelete func(id domain.FullID) error) *treeManager {
	return &treeManager{
		beforeDelete: beforeDelete,
	}
}

func (m *treeManager) Name() string {
	return treemanager.CName
}

type beforeDeleteProvider interface {
	BeforeDelete(id domain.FullID, workspaceRemove func() error) error
}

func (m *treeManager) Init(a *app.App) error {
	m.eventSender = app.MustComponent[event.Sender](a)
	m.spaceService = app.MustComponent[space.Service](a)

	beforeDelete := app.MustComponent[beforeDeleteProvider](a).BeforeDelete
	m.beforeDelete = func(id domain.FullID) error {
		return beforeDelete(id, nil)
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
	spc, err := m.spaceService.Get(ctx, spaceId)
	if err != nil {
		return
	}
	v, err := spc.GetObject(ctx, id)
	if err != nil {
		return
	}

	sb := v.(smartblock.SmartBlock)
	return sb.Tree(), nil
}

func (m *treeManager) ValidateAndPutTree(ctx context.Context, spaceId string, payload treestorage.TreeStorageCreatePayload) error {
	// TODO: this should be better done inside cache
	spc, err := m.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	_, err = spc.TreeBuilder().PutTree(ctx, payload, nil)
	return err
}

func (m *treeManager) MarkTreeDeleted(ctx context.Context, spaceId, treeId string) error {
	err := m.beforeDelete(domain.FullID{
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
	spc, err := m.spaceService.Get(ctx, spaceId)
	if err != nil {
		return
	}
	obj, err := spc.GetObject(ctx, treeId)
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

	m.sendOnRemoveEvent(spaceId, []string{treeId})
	err = spc.Remove(ctx, treeId)
	return
}

func (m *treeManager) sendOnRemoveEvent(spaceId string, ids []string) {
	m.eventSender.Broadcast(event.NewEventSingleMessage(spaceId, &pb.EventMessageValueOfObjectRemove{
		ObjectRemove: &pb.EventObjectRemove{
			Ids: ids,
		},
	}))
}
