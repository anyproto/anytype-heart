package spacecore

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/commonspace"

	"github.com/anyproto/anytype-heart/space/spacecore/keyvalueobserver"
)

func newAnySpace(cc commonspace.Space, kvObserver keyvalueobserver.Observer) (*AnySpace, error) {
	return &AnySpace{Space: cc, keyValueObserver: kvObserver}, nil
}

type AnySpace struct {
	commonspace.Space

	keyValueObserver keyvalueobserver.Observer
}

func (s *AnySpace) KeyValueObserver() keyvalueobserver.Observer {
	return s.keyValueObserver
}

func (s *AnySpace) Init(ctx context.Context) (err error) {
	err = s.Space.Init(ctx)
	if err != nil {
		return
	}
	return
}

func (s *AnySpace) TryClose(objectTTL time.Duration) (close bool, err error) {
	return false, nil
}

func (s *AnySpace) Close() (err error) {
	return s.Space.Close()
}
