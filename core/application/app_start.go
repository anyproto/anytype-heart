package application

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/anytype-heart/core/recovery"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
)

// startNewApp is the single place an account app is started. It brackets
// anytype.StartNewApp with the recovery tracker's run lifecycle: Begin before
// the start, so the tracker is registered ahead of every bootstrap component
// and its Init runs before any dial or space load; Fail on error, so the
// status stream ends with an account-level verdict. The tracker lives for the
// process and is reused across starts — each Begin opens a new run — which is
// what lets AccountRecoveryState serve it without touching s.lock.
func (s *Service) startNewApp(ctx context.Context, mode pb.EventAccountRecoveryMode, comps ...app.Component) (*app.App, error) {
	if s.recovery != nil {
		s.recovery.Begin(recovery.Run{Mode: mode, Sender: s.eventSender})
		comps = append(comps, s.recovery)
	}
	a, err := anytype.StartNewApp(ctx, s.clientWithVersion, comps...)
	if err != nil {
		if s.recovery != nil {
			s.recovery.Fail(startFailure(err))
		}
		return nil, err
	}
	return a, nil
}

// startFailure prepares a start error for the tracker's classification:
// sentinels defined above core/recovery that replace the any-sync error chain
// are joined with the tracker's own, so it can name the failure without
// importing the space tree. space.ErrSpaceNotExists is what createAccount
// reports when the network has no space for this account — the
// wrong-mnemonic / wrong-network case, which must not read as Unexpected.
func startFailure(err error) error {
	if errors.Is(err, space.ErrSpaceNotExists) {
		return errors.Join(recovery.ErrAccountNotFound, err)
	}
	return err
}
