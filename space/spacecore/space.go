package spacecore

import (
	"context"
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

func (s *AnySpace) TryClose(objectTTL time.Duration) (close bool, err error) {
	return false, nil
}

func (s *AnySpace) Close() (err error) {
	s.status.UnregisterSpace(s)
	return s.Space.Close()
}
