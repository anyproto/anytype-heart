package spacecore

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/commonspace"

	"github.com/anyproto/anytype-heart/core/syncstatus/objectsyncstatus"
)

func newAnySpace(cc commonspace.Space, status syncStatusService, statusWatcher objectsyncstatus.StatusWatcher) (*AnySpace, error) {
	return &AnySpace{
		Space:         cc,
		status:        status,
		statusWatcher: statusWatcher,
	}, nil
}

type AnySpace struct {
	commonspace.Space
	status        syncStatusService
	statusWatcher objectsyncstatus.StatusWatcher
}

func (s *AnySpace) Init(ctx context.Context) (err error) {
	err = s.Space.Init(ctx)
	if err != nil {
		return
	}
	s.status.RegisterSpace(s, s.statusWatcher)
	return
}

func (s *AnySpace) TryClose(objectTTL time.Duration) (close bool, err error) {
	return false, nil
}

func (s *AnySpace) Close() (err error) {
	s.status.UnregisterSpace(s)
	return s.Space.Close()
}
