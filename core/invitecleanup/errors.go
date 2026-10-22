package invitecleanup

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// errInviteLive: the acl still honours the invite, so its file must stay where it is.
	errInviteLive = errors.New("invite is still live")
	// errInviteUnknown: this device has never seen an acl record for the invite. Its record may have
	// been accepted by the node and not synced back yet, so the invite may well still be in use.
	errInviteUnknown = errors.New("invite is unknown to the acl")
	// errInviteDisputed: the acl says the invite is revoked, the workspace says it is current. One of
	// the two logs is behind and there is no way to tell which.
	errInviteDisputed = errors.New("acl and workspace disagree about the invite")
	// errGuestInvite: a guest invite is never revoked.
	errGuestInvite = errors.New("guest invite is never revoked")
)

// ErrAclHeadChanged means the acl moved between deciding a read-key rotation was needed and sending
// it. The rotation is abandoned rather than applied to state that has changed under it; the most
// likely cause is that another of the account's devices already did the rotation.
var ErrAclHeadChanged = errors.New("acl head changed since the rotation was decided")

// permanentError wraps an error that no amount of retrying will fix. Everything else — an unreachable
// node, a timeout, an unrecognised transport failure — is treated as transient, so an error we did
// not anticipate defers the space rather than being silently written off.
type permanentError struct {
	err error
}

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

func permanent(err error) error {
	return permanentError{err: err}
}

func isPermanent(err error) bool {
	var perm permanentError
	return errors.As(err, &perm)
}

// withRetry runs f until it succeeds, fails permanently, or runs out of attempts.
func withRetry(ctx context.Context, f func() error) error {
	var err error
	for attempt := 0; ; attempt++ {
		if err = f(); err == nil || isPermanent(err) {
			return err
		}
		if attempt >= len(retryDelays) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ctx.Err(), err)
		case <-time.After(retryDelays[attempt]):
		}
	}
}
