package spacecore

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace"
)

func newAnySpace(cc commonspace.Space, status syncStatusService) (*AnySpace, error) {
	return &AnySpace{
		Space:  cc,
		status: status,
	}, nil
}

type AnySpace struct {
	commonspace.Space
	status syncStatusService
}

func (s *AnySpace) Init(ctx context.Context) (err error) {
	err = s.Space.Init(ctx)
	if err != nil {
		return
	}
	s.status.RegisterSpace(s)
	return
}

type missingChecker interface {
	IsWasInMissing(ctx context.Context, treeId string) (ok bool, err error)
}

// IsObjectWasInMissing indicates if the given objectId was queued to sync in treeSyncer
func (s *AnySpace) IsObjectWasInMissing(ctx context.Context, objectId string) (ok bool, err error) {
	if mChecker, ok := s.Space.TreeSyncer().(missingChecker); ok {
		return mChecker.IsWasInMissing(ctx, objectId)
	}
	return false, fmt.Errorf("not implemented")
}

func (s *AnySpace) TryClose(objectTTL time.Duration) (close bool, err error) {
	return false, nil
}

func (s *AnySpace) Close() (err error) {
	s.status.UnregisterSpace(s)
	return s.Space.Close()
}
