package application

import (
	"context"
	"time"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// sessionAttachTimeout bounds how long AccountRecover waits for the caller's
// event stream. Bracketed on both sides: the client's stream reconnect backoff
// is 3s (dispatcher.ts), so this covers one full retry, and metrics' LONG_RPC
// threshold is 10s, past which every slow login would dump a goroutine profile
// to Sentry. On expiry we broadcast anyway, which is the pre-fix behaviour.
const sessionAttachTimeout = 5 * time.Second

// AccountRecover answers with nothing but an AccountShow event, and the client's
// login screen has no other trigger. token is the caller's session token (empty
// when the transport carries none, e.g. the mobile in-process library).
func (s *Service) AccountRecover(ctx context.Context, token string) error {
	// Snapshot rather than read through: WalletRecover writes derivedKeys under
	// this lock and is unauthenticated, so a second recovery landing during the
	// wait below would otherwise have us answer this request with that account.
	s.lock.RLock()
	derivedKeys := s.derivedKeys
	s.lock.RUnlock()

	if derivedKeys == nil {
		return ErrWalletNotInitialized
	}
	accountId := derivedKeys.Identity.GetPublic().Account()

	if token != "" {
		// The client opens ListenSessionEvents and calls AccountRecover as two
		// independent requests, in that order but with no ordering guarantee, and
		// the stream loses often enough to be the top login-hang report — plausibly
		// because the RPC reuses a warm connection while a new stream does not.
		// Broadcasting into an empty session map drops the event ("no servers to
		// broadcast event") with nothing to redeliver it, so wait for the caller's
		// own stream to attach first. Nothing here is slow enough to mask the race
		// any more: the key derivation that used to sit in front of this broadcast
		// moved to WalletRecover in GO-6131.
		if waiter, ok := s.eventSender.(event.SessionWaiter); ok {
			waiter.WaitForSession(ctx, token, sessionAttachTimeout)
		}
		// Checked here rather than on the wait's result: the stream can also attach
		// and then be torn down before we broadcast, and that drop is invisible in
		// the log otherwise — Broadcast only reports the case where no session at
		// all is listening.
		if !s.eventSender.IsActive(token) {
			log.Warnf("accountRecover: no event stream for the calling session, accountShow for %s may be lost", accountId)
		}
	}

	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountShow{
		AccountShow: &pb.EventAccountShow{
			Account: &model.Account{
				Id: accountId,
			},
		},
	}))

	return nil
}
