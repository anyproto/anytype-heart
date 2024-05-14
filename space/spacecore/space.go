package spacecore

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/commonspace"
)

func newAnySpace(cc commonspace.Space) (*AnySpace, error) {
	return &AnySpace{
		Space: cc,
	}, nil
}

type AnySpace struct {
	commonspace.Space
}

func (s *AnySpace) Init(ctx context.Context) (err error) {
	return s.Space.Init(ctx)
}

func (s *AnySpace) TryClose(objectTTL time.Duration) (close bool, err error) {
	return false, nil
}

func (s *AnySpace) Close() (err error) {
	return s.Space.Close()
}
