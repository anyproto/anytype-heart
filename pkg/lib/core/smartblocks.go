package core

import (
	"context"
	"fmt"
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
