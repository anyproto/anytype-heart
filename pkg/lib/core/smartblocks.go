package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/textileio/go-threads/core/thread"
)

var ErrBlockSnapshotNotFound = fmt.Errorf("block snapshot not found")

func (a *Anytype) GetBlock(id string) (SmartBlock, error) {
	parts := strings.Split(id, "/")

	_, err := thread.Decode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("incorrect block id: %w", err)
	}
	smartBlock, err := a.GetSmartBlock(parts[0])
	if err != nil {
		return nil, err
	}

	return smartBlock, nil
}

func (a *Anytype) DeleteBlock(id string) error {
	err := a.threadService.DeleteThread(id)
	if err != nil {
		return err
	}

	if err = a.localStore.Objects.DeletePage(id); err != nil {
		return err
	}

	return nil
}

func (a *Anytype) GetSmartBlock(id string) (*smartBlock, error) {
	tid, err := thread.Decode(id)
	if err != nil {
		return nil, err
	}

	thrd, err := a.t.GetThread(context.TODO(), tid)
	if err != nil {
		return nil, err
	}

	return &smartBlock{thread: thrd, node: a}, nil
}
