package recovery

import (
	"context"
	"errors"
	"fmt"
	gonet "net"
	"strings"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/net"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/secureservice/handshake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassify(t *testing.T) {
	type want struct {
		class     pb.EventAccountRecoveryErrorClass
		retryable bool
	}
	tests := []struct {
		name string
		err  error
		want want
	}{
		{"incompatible version", handshake.ErrIncompatibleVersion, want{pb.EventAccountRecovery_IncompatibleVersion, false}},
		{"incompatible proto", handshake.ErrIncompatibleProto, want{pb.EventAccountRecovery_IncompatibleVersion, false}},
		{"remote incompatible proto", handshake.ErrRemoteIncompatibleProto, want{pb.EventAccountRecovery_IncompatibleVersion, false}},
		{"invalid credentials", handshake.ErrInvalidCredentials, want{pb.EventAccountRecovery_NotAuthorized, false}},
		{"peer declined credentials", handshake.ErrPeerDeclinedCredentials, want{pb.EventAccountRecovery_NotAuthorized, false}},
		{"skip verify not allowed", handshake.ErrSkipVerifyNotAllowed, want{pb.EventAccountRecovery_NotAuthorized, false}},
		{"coordinator forbidden", coordinatorproto.ErrForbidden, want{pb.EventAccountRecovery_NotAuthorized, false}},
		{"account deleted", coordinatorproto.ErrAccountIsDeleted, want{pb.EventAccountRecovery_AccountDeleted, false}},
		{"space deleted (spacesync)", spacesyncproto.ErrSpaceIsDeleted, want{pb.EventAccountRecovery_SpaceDeleted, false}},
		{"space deleted (coordinator)", coordinatorproto.ErrSpaceIsDeleted, want{pb.EventAccountRecovery_SpaceDeleted, false}},
		{"space deletion pending", coordinatorproto.ErrSpaceDeletionPending, want{pb.EventAccountRecovery_SpaceDeleted, false}},
		{"too many requests", spacesyncproto.ErrTooManyRequestsFromPeer, want{pb.EventAccountRecovery_RateLimited, true}},
		{"duplicate request", spacesyncproto.ErrDuplicateRequest, want{pb.EventAccountRecovery_RateLimited, true}},
		{"unable to connect", net.ErrUnableToConnect, want{pb.EventAccountRecovery_PeerUnreachable, true}},
		{"addrs not found", peerservice.ErrAddrsNotFound, want{pb.EventAccountRecovery_PeerUnreachable, true}},
		{"handshake deadline", handshake.ErrDeadlineExceeded, want{pb.EventAccountRecovery_PeerUnreachable, true}},
		{"context deadline", context.DeadlineExceeded, want{pb.EventAccountRecovery_PeerUnreachable, true}},
		{"peer not responsible", spacesyncproto.ErrPeerIsNotResponsible, want{pb.EventAccountRecovery_PeerUnreachable, true}},
		{"net.OpError", &gonet.OpError{Op: "dial", Err: errors.New("connection refused")}, want{pb.EventAccountRecovery_PeerUnreachable, true}},
		{"net.Error timeout", timeoutErr{}, want{pb.EventAccountRecovery_PeerUnreachable, true}},
		{"invalid changes", objecttree.ErrHasInvalidChanges, want{pb.EventAccountRecovery_Unexpected, false}},
		{"collection not found", anystore.ErrCollectionNotFound, want{pb.EventAccountRecovery_Unexpected, false}},
		{"space missing outside the account context", spacesyncproto.ErrSpaceMissing, want{pb.EventAccountRecovery_Unexpected, true}},
		{"unknown", errors.New("something"), want{pb.EventAccountRecovery_Unexpected, true}},
		{"wrapped", fmt.Errorf("init tech space: %w", coordinatorproto.ErrAccountIsDeleted), want{pb.EventAccountRecovery_AccountDeleted, false}},
		{"joined dial errors: best member wins", errors.Join(timeoutErr{}, handshake.ErrIncompatibleVersion, &gonet.OpError{Op: "dial", Err: errors.New("refused")}), want{pb.EventAccountRecovery_IncompatibleVersion, false}},
		{"joined dial errors: all transport", errors.Join(timeoutErr{}, &gonet.OpError{Op: "dial", Err: errors.New("refused")}), want{pb.EventAccountRecovery_PeerUnreachable, true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// when
			got := classify(tc.err)

			// then
			require.NotNil(t, got)
			assert.Equal(t, tc.want.class, got.class)
			assert.Equal(t, tc.want.retryable, got.retryable)
			assert.Equal(t, tc.err.Error(), got.debug)
		})
	}

	t.Run("nil and cancellation classify to nothing", func(t *testing.T) {
		assert.Nil(t, classify(nil))
		assert.Nil(t, classify(context.Canceled))
		assert.Nil(t, classify(fmt.Errorf("start: %w", context.Canceled)))
	})

	t.Run("debug message is bounded", func(t *testing.T) {
		got := classify(errors.New(strings.Repeat("x", 1000)))
		assert.Len(t, got.debug, debugMessageMax)
	})
}

func TestClassifyAccount(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want pb.EventAccountRecoveryErrorClass
	}{
		{"space deleted is account deleted", fmt.Errorf("init tech space: %w", spacesyncproto.ErrSpaceIsDeleted), pb.EventAccountRecovery_AccountDeleted},
		{"space missing is account not found", fmt.Errorf("init tech space: %w", spacesyncproto.ErrSpaceMissing), pb.EventAccountRecovery_AccountNotFound},
		{"incompatible version keeps its class", handshake.ErrIncompatibleVersion, pb.EventAccountRecovery_IncompatibleVersion},
		{"unknown stays unexpected", errors.New("boom"), pb.EventAccountRecovery_Unexpected},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyAccount(tc.err)
			require.NotNil(t, got)
			assert.Equal(t, tc.want, got.class)
			if tc.want == pb.EventAccountRecovery_AccountNotFound {
				assert.False(t, got.retryable)
			}
		})
	}
	assert.Nil(t, classifyAccount(context.Canceled))
}
