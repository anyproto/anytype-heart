package space

import (
	"context"

	"github.com/anyproto/any-sync/commonspace"
)

func newClientSpace(cc commonspace.Space, status syncStatusService) (commonspace.Space, error) {
	return &clientSpace{cc, status}, nil
}

type clientSpace struct {
	commonspace.Space
	status syncStatusService
}

func (s *clientSpace) Init(ctx context.Context) (err error) {
	err = s.Space.Init(ctx)
	if err != nil {
		return
	}
	s.status.RegisterSpace(s)
	return
}

func (s *clientSpace) Close() (err error) {
	s.status.UnregisterSpace(s)
	return s.Space.Close()
}
