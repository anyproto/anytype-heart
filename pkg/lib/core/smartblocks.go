package core

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"strings"

	"github.com/textileio/go-threads/core/thread"
)

var ErrBlockSnapshotNotFound = fmt.Errorf("block snapshot not found")

func (a *Anytype) GetBlock(id string) (SmartBlock, error) {
	return a.GetBlockCtx(context.Background(), id)
}

func (a *Anytype) GetBlockCtx(ctx context.Context, id string) (SmartBlock, error) {
	parts := strings.Split(id, "/")

	_, err := thread.Decode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("incorrect block id: %w", err)
	}
	smartBlock, err := a.GetSmartBlockCtx(ctx, parts[0])
	if err != nil {
		return nil, err
	}

	return smartBlock, nil
}

func (a *Anytype) DeleteBlock(id string) error {
	var workspaceId string
	// todo: probably need to have more reliable way to get the workspace where this object is stored
	if d, err := a.ObjectStore().GetDetails(id); err != nil {
		workspaceId = pbtypes.GetString(d.Details, bundle.RelationKeyWorkspaceId.String())
	}
	err := a.threadService.DeleteThread(id, workspaceId)
	if err != nil {
		return err
	}

	if err = a.objectStore.DeleteObject(id); err != nil {
		return err
	}

	return nil
}

func (a *Anytype) GetSmartBlock(id string) (*smartBlock, error) {
	return a.GetSmartBlockCtx(context.Background(), id)
}

func (a *Anytype) GetSmartBlockCtx(ctx context.Context, id string) (*smartBlock, error) {
	tid, err := thread.Decode(id)
	if err != nil {
		return nil, err
	}

	thrd, err := a.threadService.Threads().GetThread(ctx, tid)
	if err != nil {
		return nil, err
	}

	return &smartBlock{thread: thrd, node: a}, nil
}
