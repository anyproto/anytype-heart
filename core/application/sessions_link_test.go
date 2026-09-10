package application

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// linkFixture is a Service wired just far enough to run the pairing handshake,
// with every broadcast event captured so tests can assert on what the client
// would actually receive.
type linkFixture struct {
	*Service
	events []*pb.Event
}

func newLinkFixture(t *testing.T) *linkFixture {
	t.Helper()
	fx := &linkFixture{Service: New()}
	fx.sessionSigningKey = []byte("test-signing-key-1234")
	fx.sessions = session.New()
	fx.eventSender = event.NewCallbackSender(func(e *pb.Event) {
		fx.events = append(fx.events, e)
	})

	walletMock := mock_wallet.NewMockWallet(t)
	walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
	walletMock.EXPECT().Init(nil).Return(nil).Maybe()
	walletMock.EXPECT().PersistAppLink(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ string, _ model.AccountAuthLocalApiScope, _ int64, grant *walletComp.AppLinkGrant) (*walletComp.AppLinkInfo, error) {
			return &walletComp.AppLinkInfo{AppHash: "hash", AppKey: "appKey", Grant: grant}, nil
		}).Maybe()

	a := new(app.App)
	a.Register(walletMock)
	fx.app = a
	return fx
}

func (fx *linkFixture) challenges() []*pb.EventAccountLinkApprovalRequest {
	var out []*pb.EventAccountLinkApprovalRequest
	for _, e := range fx.events {
		for _, m := range e.Messages {
			if v, ok := m.Value.(*pb.EventMessageValueOfAccountLinkApprovalRequest); ok {
				out = append(out, v.AccountLinkApprovalRequest)
			}
		}
	}
	return out
}

func (fx *linkFixture) hides() []*pb.EventAccountLinkApprovalHide {
	var out []*pb.EventAccountLinkApprovalHide
	for _, e := range fx.events {
		for _, m := range e.Messages {
			if v, ok := m.Value.(*pb.EventMessageValueOfAccountLinkApprovalHide); ok {
				out = append(out, v.AccountLinkApprovalHide)
			}
		}
	}
	return out
}

// TestLinkLocalStartNewChallenge_EventCarriesNoCode is the assertion the whole
// redesign exists for: the code must never reach the event bus, which every
// connected session receives.
func TestLinkLocalStartNewChallenge_EventCarriesNoCode(t *testing.T) {
	// given
	fx := newLinkFixture(t)
	clientInfo := &pb.EventAccountLinkApprovalRequestClientInfo{
		Name:        "Save to Anytype",
		ProcessPath: "/Applications/Google Chrome.app",
		Origin:      "chrome-extension://abc",
	}

	// when
	_, err := fx.LinkLocalStartNewChallenge(model.AccountAuth_JsonAPI, clientInfo, model.AccountAuthAppGrant_Read)

	// then the client is told who is asking. The event exists only to request
	// approval, so its arrival is the signal — there is no needApprove flag and
	// no code.
	require.NoError(t, err)
	broadcast := fx.challenges()
	require.Len(t, broadcast, 1)
	assert.Equal(t, clientInfo, broadcast[0].ClientInfo)
	assert.Equal(t, model.AccountAuth_JsonAPI, broadcast[0].Scope)

	// ...and no code travels with it, because none exists yet
	for _, e := range fx.events {
		assert.NotContains(t, e.String(), "challenge:", "no code may appear in any broadcast")
	}
}

func TestLinkLocalApproveChallenge(t *testing.T) {
	clientInfo := func() *pb.EventAccountLinkApprovalRequestClientInfo {
		return &pb.EventAccountLinkApprovalRequestClientInfo{
			// a name is required at challenge time: it becomes the key's
			// creation provenance (APIV2_OBJECT_DELETE.md §5/§11.7)
			Name:        "Clipper",
			ProcessPath: "/Applications/Google Chrome.app",
			Origin:      "chrome-extension://abc",
		}
	}

	t.Run("allow returns the code to this caller only", func(t *testing.T) {
		// given
		fx := newLinkFixture(t)
		info := clientInfo()
		id, err := fx.LinkLocalStartNewChallenge(model.AccountAuth_JsonAPI, info, model.AccountAuthAppGrant_Read)
		require.NoError(t, err)
		before := len(fx.events)

		// when
		code, hidden, err := fx.LinkLocalApproveChallenge(info.ProcessPath, info.Origin, true, testProtoGrant())

		// then the code comes back in the response...
		require.NoError(t, err)
		require.Len(t, code, 4)
		assert.Equal(t, info, hidden)

		// ...and nothing at all was broadcast, so no other session saw it —
		// and, deliberately, no hide either: the approving session is now
		// displaying this code, and LinkApprovalHide would tell it to
		// dismiss that. See LinkLocalApproveChallenge for the second-window
		// cost this defers.
		assert.Len(t, fx.events, before, "approval must not broadcast")

		// ...and it is the code that pairs
		_, appKey, grant, err := fx.LinkLocalSolveChallenge(&pb.RpcAccountLocalLinkSolveChallengeRequest{
			ChallengeId: id,
			Answer:      code,
		})
		require.NoError(t, err)
		assert.Equal(t, "appKey", appKey)
		assert.Equal(t, testProtoGrant(), grant, "the solver echoes the persisted approval")

		// ...and the prompt is dismissed by naming the caller, not the code
		hides := fx.hides()
		require.Len(t, hides, 1)
		assert.Equal(t, info, hides[0].ClientInfo)
	})

	t.Run("deny hides the prompt and yields no code", func(t *testing.T) {
		// given
		fx := newLinkFixture(t)
		info := clientInfo()
		_, err := fx.LinkLocalStartNewChallenge(model.AccountAuth_JsonAPI, info, model.AccountAuthAppGrant_Read)
		require.NoError(t, err)

		// when
		code, _, err := fx.LinkLocalApproveChallenge(info.ProcessPath, info.Origin, false, nil)

		// then
		require.NoError(t, err)
		assert.Empty(t, code)
		hides := fx.hides()
		require.Len(t, hides, 1)
		assert.Equal(t, info, hides[0].ClientInfo)

		// ...and the caller cannot make the user press Deny again
		_, err = fx.LinkLocalStartNewChallenge(model.AccountAuth_JsonAPI, info, model.AccountAuthAppGrant_Read)
		assert.ErrorIs(t, err, session.ErrChallengeDenied)
	})

	t.Run("nothing pending", func(t *testing.T) {
		fx := newLinkFixture(t)

		_, _, err := fx.LinkLocalApproveChallenge("", "chrome-extension://stranger", true, testProtoGrant())

		assert.ErrorIs(t, err, session.ErrNoPendingChallenge)
	})

	t.Run("app not running", func(t *testing.T) {
		s := New()

		_, _, err := s.LinkLocalApproveChallenge("", "chrome-extension://abc", true, testProtoGrant())

		assert.ErrorIs(t, err, ErrApplicationIsNotRunning)
	})
}

// TestLinkLocalApproveChallenge_ExpiredPromptIsNotApprovable closes P1 from
// the 4-lens review. ApproveChallenge neither swept nor checked stateSince,
// and it is the one entry point that did not, so an hours-stale prompt still
// minted a code — and stamped itself a fresh five minutes to spend it in.
// Nothing runs on a timer, so the TTL is only ever enforced by these sweeps.
func TestLinkLocalApproveChallenge_ExpiredPromptIsNotApprovable(t *testing.T) {
	// given a prompt raised long ago
	fx := newLinkFixture(t)
	now := time.Now()
	fx.sessions.(interface{ SetClock(func() time.Time) }).SetClock(func() time.Time { return now })
	info := &pb.EventAccountLinkApprovalRequestClientInfo{
		Name:        "Clipper",
		ProcessPath: "/Applications/Google Chrome.app",
		Origin:      "chrome-extension://abc",
	}
	_, err := fx.LinkLocalStartNewChallenge(model.AccountAuth_JsonAPI, info, model.AccountAuthAppGrant_Read)
	require.NoError(t, err)

	// when the user answers it a day later
	now = now.Add(24 * time.Hour)
	code, _, err := fx.LinkLocalApproveChallenge(info.ProcessPath, info.Origin, true, testProtoGrant())

	// then nothing is minted
	require.ErrorIs(t, err, session.ErrNoPendingChallenge)
	assert.Empty(t, code)

	// ...and the stale prompt was taken off screen by the sweep
	hides := fx.hides()
	require.Len(t, hides, 1)
	assert.Equal(t, info, hides[0].ClientInfo)
}
