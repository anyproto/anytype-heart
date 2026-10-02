package payments

import (
	"context"
	"errors"
	"fmt"
	stdnet "net"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/anyproto/any-sync/net"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"storj.io/drpc"
	"storj.io/drpc/drpcconn"

	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

func statusResponse(productIds ...string) *psp.MembershipV2_GetStatusResponse {
	out := &psp.MembershipV2_GetStatusResponse{}
	for _, id := range productIds {
		out.Products = append(out.Products, &psp.MembershipV2_PurchasedProduct{
			Product: &psp.MembershipV2_Product{Id: id},
		})
	}
	return out
}

func productsResponse(ids ...string) *psp.MembershipV2_GetProductsResponse {
	out := &psp.MembershipV2_GetProductsResponse{}
	for _, id := range ids {
		out.Products = append(out.Products, &psp.MembershipV2_Product{Id: id})
	}
	return out
}

func newV2Fixture(t *testing.T) *fixture {
	return newFixture(t, withV2(), withRealCache(t.TempDir()))
}

// eventSummary is "status:FRESH:rev" / "products:FRESH:rev"
func eventSummaries(msgs []*pb.EventMessage) []string {
	var out []string
	for _, m := range msgs {
		switch v := m.Value.(type) {
		case *pb.EventMessageValueOfMembershipV2Update:
			st := v.MembershipV2Update.FetchState
			out = append(out, fmt.Sprintf("status:%s:%d", st.Freshness, st.Revision.Counter))
		case *pb.EventMessageValueOfMembershipV2ProductsUpdate:
			st := v.MembershipV2ProductsUpdate.FetchState
			out = append(out, fmt.Sprintf("products:%s:%d", st.Freshness, st.Revision.Counter))
		default:
			out = append(out, fmt.Sprintf("other:%T", v))
		}
	}
	return out
}

func (fx *fixture) getStatus(t *testing.T, noCache bool) (*pb.RpcMembershipV2GetStatusResponse, error) {
	resp, err := fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: noCache})
	require.NotNil(t, resp)
	require.NotNil(t, resp.FetchState, "FetchState must always be set")
	return resp, err
}

func (fx *fixture) getProducts(t *testing.T, noCache bool) (*pb.RpcMembershipV2GetProductsResponse, error) {
	resp, err := fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: noCache})
	require.NotNil(t, resp)
	require.NotNil(t, resp.FetchState, "FetchState must always be set")
	return resp, err
}

func purchasedIds(d *model.MembershipV2Data) []string {
	var ids []string
	for _, p := range d.Products {
		ids = append(ids, p.Product.Id)
	}
	return ids
}

func TestV2Freshness(t *testing.T) {
	t.Run("warm cache + transient error returns STALE data without error", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		resp, err := fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_FRESH, resp.FetchState.Freshness)
		assert.NotZero(t, resp.FetchState.LastSuccessfulFetchAt)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
		resp, err = fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)
		assert.Equal(t, model.MembershipV2_RefreshErrorPaymentNode, resp.FetchState.LastRefreshError)
		assert.Equal(t, []string{"p1"}, purchasedIds(resp.Data))
		assert.Equal(t, pb.RpcMembershipV2GetStatusResponseError_NULL, resp.Error.Code)

		// first success announces the resource; the failure publishes nothing
		assert.Equal(t, []string{"status:FRESH:1"}, eventSummaries(fx.events.all()))
	})

	t.Run("cold cache + transient error returns the error with NONE", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)

		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(nil, context.DeadlineExceeded)
		resp, err := fx.getProducts(t, true)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, model.MembershipV2_NONE, resp.FetchState.Freshness)
		assert.Equal(t, model.MembershipV2_RefreshErrorPaymentNode, resp.FetchState.LastRefreshError)
		assert.Empty(t, resp.Products)
		assert.Empty(t, fx.events.all())
	})

	t.Run("warm cache + non-transient error returns the error", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, psp.ErrInvalidSignature)
		resp, err := fx.getStatus(t, true)
		require.ErrorIs(t, err, psp.ErrInvalidSignature)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)
		assert.Equal(t, model.MembershipV2_RefreshErrorUnknown, resp.FetchState.LastRefreshError)
	})

	t.Run("successfully fetched empty is distinct from never fetched", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)

		resp, err := fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_NONE, resp.FetchState.Freshness)

		status, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_NONE, status.FetchState.Freshness)

		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse(), nil)
		resp, err = fx.getProducts(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_FRESH, resp.FetchState.Freshness)
		assert.NotNil(t, resp.Products)
		assert.Empty(t, resp.Products)

		// cache-only read now reports the successful empty result as FRESH
		resp, err = fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_FRESH, resp.FetchState.Freshness)
		// status is still never fetched
		status, err = fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_NONE, status.FetchState.Freshness)

		assert.Equal(t, []string{"products:FRESH:1"}, eventSummaries(fx.events.all()))
	})

	t.Run("local-only mode returns NONE without network", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.cfg.NetworkMode = pb.RpcAccount_LocalOnly

		resp, err := fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_NONE, resp.FetchState.Freshness)
		presp, err := fx.getProducts(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_NONE, presp.FetchState.Freshness)
	})

	t.Run("V2 disabled and invalid flag return explicit NONE", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)

		resp, err := fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{ForceRefreshSec: -1})
		require.ErrorIs(t, err, ErrInvalidForceRefresh)
		assert.Equal(t, model.MembershipV2_NONE, resp.FetchState.Freshness)

		fx.cfg.EnableMembershipV2 = false
		resp, err = fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
		require.ErrorIs(t, err, ErrV2NotEnabled)
		assert.Equal(t, model.MembershipV2_NONE, resp.FetchState.Freshness)
		presp, err := fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
		require.ErrorIs(t, err, ErrV2NotEnabled)
		assert.Equal(t, model.MembershipV2_NONE, presp.FetchState.Freshness)
	})
}

func TestV2RecoveryEvents(t *testing.T) {
	t.Run("error then unchanged success publishes one FRESH event for that resource", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
		resp, err := fx.getStatus(t, true)
		require.NoError(t, err)
		staleRev := resp.FetchState.Revision.Counter

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil).Times(2)
		resp, err = fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_FRESH, resp.FetchState.Freshness)
		assert.Greater(t, resp.FetchState.Revision.Counter, staleRev)
		// a further unchanged success publishes nothing
		_, err = fx.getStatus(t, true)
		require.NoError(t, err)

		assert.Equal(t, []string{"status:FRESH:1", "status:FRESH:3"}, eventSummaries(fx.events.all()))
		assert.Equal(t, int64(1), fx.stats.resource(v2Status).recoveryEvents.Load())
	})

	t.Run("products success does not announce status recovery", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err := fx.refreshV2(ctx, true)
		require.NoError(t, err)

		// status keeps failing while products succeed
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect).Times(2)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil).Times(2)
		_, err = fx.refreshV2(ctx, true)
		require.Error(t, err)
		_, err = fx.refreshV2(ctx, true)
		require.Error(t, err)

		assert.Equal(t, []string{"products:FRESH:1", "status:FRESH:1"}, eventSummaries(fx.events.all()))

		// status recovers: exactly one status event
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		changed, err := fx.refreshV2(ctx, true)
		require.NoError(t, err)
		assert.False(t, changed, "a recovery is not a change")
		assert.Equal(t, []string{"products:FRESH:1", "status:FRESH:1", "status:FRESH:3"}, eventSummaries(fx.events.all()))
	})

	t.Run("cache-only STALE read owes a recovery event", func(t *testing.T) {
		dir := t.TempDir()
		fx := newFixture(t, withV2(), withRealCache(dir))
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)
		fx.finish(t)

		// restart with an old success: served STALE, then an unchanged
		// success publishes FRESH
		fx = newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		fx.v2.mu.Lock()
		fx.v2LoadLocked()
		fx.v2.res[v2Status].lastSuccessAt = time.Now().Add(-time.Hour)
		fx.v2.mu.Unlock()

		resp, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err = fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Equal(t, []string{"status:FRESH:1"}, eventSummaries(fx.events.all()))
	})
}

func TestV2RevisionOnAnyDifference(t *testing.T) {
	fx := newV2Fixture(t)
	defer fx.finish(t)

	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
	fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
	_, err := fx.refreshV2(ctx, true)
	require.NoError(t, err)
	notified := fx.limitsNotifications.Load()

	// a change outside the P0 equality set (TeamOwnerID): published at a new
	// revision, but not a `changed` (no forced-poll stop, no limits work)
	r2 := statusResponse("p1")
	r2.TeamOwnerID = "owner2"
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(r2, nil)
	fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
	changed, err := fx.refreshV2(ctx, true)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, notified, fx.limitsNotifications.Load())
	assert.Equal(t, []string{"products:FRESH:1", "status:FRESH:1", "status:FRESH:2"}, eventSummaries(fx.events.all()))
	assert.Zero(t, fx.stats.resource(v2Status).recoveryEvents.Load(), "a difference is not a recovery")

	resp, err := fx.getStatus(t, false)
	require.NoError(t, err)
	assert.Equal(t, "owner2", resp.Data.TeamOwnerID)
	assert.Equal(t, uint64(2), resp.FetchState.Revision.Counter)
}

func TestV2CallerCancellation(t *testing.T) {
	t.Run("caller cancelling mid-RPC is not a resource outcome", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		fresh, err := fx.getStatus(t, true)
		require.NoError(t, err)

		cctx, cancel := context.WithCancel(ctx)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(context.Context, *psp.MembershipV2_GetStatusRequest) (*psp.MembershipV2_GetStatusResponse, error) {
				cancel()
				return nil, context.Canceled
			})
		resp, err := fx.V2GetStatus(cctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness, "fallback for the caller")

		fx.v2.mu.Lock()
		rs := fx.v2.res[v2Status]
		fx.v2.mu.Unlock()
		assert.NoError(t, rs.lastErr)
		assert.Equal(t, uint64(1), rs.acceptedSeq, "no watermark advance")
		assert.Equal(t, fresh.FetchState.Revision.Counter, rs.revision, "no revision bump")
		assert.True(t, rs.recoveryOwed)
		assert.Equal(t, int64(1), fx.stats.resource(v2Status).callerGone.Load())
	})

	t.Run("a newer cancelled call doesn't supersede an older in-flight success", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		aStarted, release := make(chan struct{}), make(chan struct{})
		bctx, bcancel := context.WithCancel(ctx)
		gomock.InOrder(
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					close(aStarted)
					<-release
					return productsResponse("p1"), nil
				}),
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					bcancel()
					return nil, context.Canceled
				}),
		)
		aDone := make(chan *pb.RpcMembershipV2GetProductsResponse)
		go func() {
			resp, err := fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
			assert.NoError(t, err)
			aDone <- resp
		}()
		<-aStarted
		_, err := fx.V2GetProducts(bctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
		require.Error(t, err)
		close(release)
		a := <-aDone
		assert.Equal(t, model.MembershipV2_FRESH, a.FetchState.Freshness)
		assert.Equal(t, []string{"products:FRESH:1"}, eventSummaries(fx.events.all()))
	})

	t.Run("a superseded success gets STALE, never another caller's error", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err := fx.getProducts(t, true)
		require.NoError(t, err)

		aStarted, release := make(chan struct{}), make(chan struct{})
		gomock.InOrder(
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					close(aStarted)
					<-release
					return productsResponse("p1"), nil
				}),
			// a newer, non-transient failure is accepted first
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(nil, psp.ErrInvalidSignature),
		)
		aDone := make(chan error)
		var a *pb.RpcMembershipV2GetProductsResponse
		go func() {
			var err error
			a, err = fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
			aDone <- err
		}()
		<-aStarted
		_, err = fx.getProducts(t, true)
		require.ErrorIs(t, err, psp.ErrInvalidSignature)
		close(release)
		require.NoError(t, <-aDone)
		assert.Equal(t, model.MembershipV2_STALE, a.FetchState.Freshness)
		assert.Equal(t, model.MembershipV2_RefreshErrorUnknown, a.FetchState.LastRefreshError)
	})
}

// rawEnc is a drpc encoding for a call whose payload doesn't matter
type rawEnc struct{}

func (rawEnc) Marshal(drpc.Message) ([]byte, error) { return []byte("x"), nil }
func (rawEnc) Unmarshal([]byte, drpc.Message) error { return nil }

// lostConnectionErr runs a real drpc call over a pipe whose peer closes
// mid-call; drpc reports the EOF-terminated manager as context.Canceled
func lostConnectionErr(ctx context.Context) error {
	c, s := stdnet.Pipe()
	go func() {
		buf := make([]byte, 64)
		_, _ = s.Read(buf)
		_ = s.Close()
	}()
	conn := drpcconn.New(c)
	defer conn.Close()
	var out struct{}
	return conn.Invoke(ctx, "/pp/GetStatus", rawEnc{}, struct{}{}, &out)
}

func TestV2LostConnection(t *testing.T) {
	t.Run("real drpc EOF mid-call surfaces as context.Canceled", func(t *testing.T) {
		err := lostConnectionErr(ctx)
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, IsTransientError(err), "can't be told from a cancellation without the caller ctx")
		assert.True(t, isTransientForCaller(ctx, err))
	})

	t.Run("warm cache: STALE without error, PaymentNode code", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(callCtx context.Context, _ *psp.MembershipV2_GetStatusRequest) (*psp.MembershipV2_GetStatusResponse, error) {
				return nil, lostConnectionErr(callCtx)
			})
		resp, err := fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)
		assert.Equal(t, model.MembershipV2_RefreshErrorPaymentNode, resp.FetchState.LastRefreshError)
		fx.v2.mu.Lock()
		lastErr := fx.v2.res[v2Status].lastErr
		fx.v2.mu.Unlock()
		assert.ErrorIs(t, lastErr, errConnLost)
		assert.True(t, IsTransientError(lastErr), "the stored error is transient without the caller ctx")
	})

	t.Run("cold cache: transient error, manual window opens", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(callCtx context.Context, _ *psp.MembershipV2_GetStatusRequest) (*psp.MembershipV2_GetStatusResponse, error) {
				return nil, lostConnectionErr(callCtx)
			})
		resp, err := fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{ForceRefreshSec: 180})
		require.Error(t, err)
		assert.True(t, IsTransientError(err), "the middleware maps it to PAYMENT_NODE_ERROR")
		assert.Equal(t, model.MembershipV2_NONE, resp.FetchState.Freshness)
		assert.Equal(t, int64(1), fx.refreshCtrlV2.stats.admissions[forceOriginManual].Load())
		fx.refreshCtrlV2.mu.Lock()
		fx.refreshCtrlV2.intents[forceOriginManual] = forceIntent{}
		fx.refreshCtrlV2.mu.Unlock()
	})
}

func TestV2Restart(t *testing.T) {
	t.Run("persisted success is served STALE after restart until verified, with a new epoch", func(t *testing.T) {
		dir := t.TempDir()
		fx := newFixture(t, withV2(), withRealCache(dir))
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse(), nil)
		resp, err := fx.getProducts(t, true)
		require.NoError(t, err)
		epoch := resp.FetchState.Revision.Epoch
		fx.finish(t)

		fx = newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		resp, err = fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness, "successful empty survives restart, unverified")
		assert.NotNil(t, resp.Products)
		assert.NotEqual(t, epoch, resp.FetchState.Revision.Epoch)
		status, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_NONE, status.FetchState.Freshness)
		assert.True(t, fx.v2NeedsFetch())

		// the start-up poll verifies: FRESH, published (the STALE answer is owed)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse(), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse(), nil)
		_, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		resp, err = fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_FRESH, resp.FetchState.Freshness)
		assert.ElementsMatch(t, []string{"products:FRESH:1", "status:FRESH:1"}, eventSummaries(fx.events.all()))
	})

	t.Run("restart after a failed write: the lost data is never served FRESH, the loop verifies", func(t *testing.T) {
		dir := t.TempDir()
		fx := newFixture(t, withV2(), withRealCache(dir))
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("x"), nil)
		_, err := fx.refreshV2(ctx, false)
		require.NoError(t, err)
		fx.service.cache = &failingCommitCache{CacheService: fx.service.cache, fail: true}
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1", "p2"), nil)
		r, err := fx.getStatus(t, true)
		require.NoError(t, err)
		require.Equal(t, []string{"p1", "p2"}, purchasedIds(r.Data))
		fx.finish(t)

		fx = newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		r2, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"p1"}, purchasedIds(r2.Data))
		assert.Equal(t, model.MembershipV2_STALE, r2.FetchState.Freshness)
		assert.True(t, fx.v2NeedsFetch())

		// one poll per resource verifies; the purchase comes back as an event
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1", "p2"), nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("x"), nil)
		_, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		r3, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"p1", "p2"}, purchasedIds(r3.Data))
		assert.Equal(t, model.MembershipV2_FRESH, r3.FetchState.Freshness)
		assert.Contains(t, eventSummaries(fx.events.all()), "status:FRESH:1")
	})

	t.Run("legacy cache entry migrates without a wipe and is served STALE", func(t *testing.T) {
		dir := t.TempDir()
		provider, err := anystoreprovider.NewInPath(dir)
		require.NoError(t, err)
		require.NoError(t, provider.Init(nil))
		store := keyvaluestore.NewJsonFromCollection[*cache.StorageV2Struct](provider.GetSystemCollection())
		require.NoError(t, store.Set(ctx, anystoreprovider.SystemKeys.PaymentCacheV2Key(1), &cache.StorageV2Struct{
			CurrentVersion: 1,
			ExpireTime:     time.Now().Add(time.Hour),
			// placeholder status written along with products
			V2Data:       &model.MembershipV2Data{},
			ProductsData: []*model.MembershipV2Product{{Id: "p1"}},
		}))
		require.NoError(t, provider.Close(ctx))

		fx := newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		resp, err := fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness, "legacy data has unknown age")
		assert.Zero(t, resp.FetchState.LastSuccessfulFetchAt, "no success time derived from the expiry")
		require.Len(t, resp.Products, 1)

		status, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_NONE, status.FetchState.Freshness, "placeholder status is not fetched")

		// the unexpired legacy expiry doesn't stop the background loop from
		// fetching the never-fetched status and the owed products recovery
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse(), nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"products:FRESH:1", "status:FRESH:1"}, eventSummaries(fx.events.all()))
	})
}

func TestV2BackgroundTriggers(t *testing.T) {
	t.Run("legacy entry with both resources and an unexpired expiry: verified at start, then idle", func(t *testing.T) {
		dir := t.TempDir()
		writeLegacyV2(t, dir, &cache.StorageV2Struct{
			CurrentVersion: 1,
			ExpireTime:     time.Now().Add(time.Hour),
			V2Data: &model.MembershipV2Data{Products: []*model.MembershipV2PurchasedProduct{
				{Product: &model.MembershipV2Product{Id: "p1"}},
			}},
			ProductsData: []*model.MembershipV2Product{{Id: "p1"}},
		})
		fx := newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)

		status, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, status.FetchState.Freshness)

		// unverified in this epoch: the start-up poll fetches despite the
		// unexpired expiry; the STALE status answer is owed an event
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"status:FRESH:1"}, eventSummaries(fx.events.all()))

		// verified, unexpired, nothing pending: idle
		changed, err := fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.False(t, changed)
	})

	t.Run("never fetched with an unexpired legacy expiry: the loop fetches", func(t *testing.T) {
		dir := t.TempDir()
		writeLegacyV2(t, dir, &cache.StorageV2Struct{
			CurrentVersion: 1,
			ExpireTime:     time.Now().Add(time.Hour),
			V2Data:         &model.MembershipV2Data{},
			ProductsData:   []*model.MembershipV2Product{{Id: "p1"}},
		})
		fx := newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse(), nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err := fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"status:FRESH:1"}, eventSummaries(fx.events.all()))
	})

	t.Run("owed only, no error, unexpired: the loop fetches", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.refreshV2(ctx, false)
		require.NoError(t, err)

		fx.v2.mu.Lock()
		fx.v2.res[v2Products].recoveryOwed = true
		require.NoError(t, fx.v2.res[v2Products].lastErr)
		fx.v2.mu.Unlock()

		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"products:FRESH:1", "status:FRESH:1", "products:FRESH:2"}, eventSummaries(fx.events.all()))
	})

	t.Run("a success time in the future (wall clock stepped back) is STALE", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err := fx.getProducts(t, true)
		require.NoError(t, err)
		fx.v2.mu.Lock()
		fx.v2.res[v2Products].lastSuccessAt = time.Now().Add(time.Hour)
		fx.v2.mu.Unlock()
		resp, err := fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)
	})
}

// countingCache counts cache writes
type countingCache struct {
	cache.CacheService
	commits []cache.V2Commit
}

func (c *countingCache) CacheV2Commit(ctx context.Context, commit cache.V2Commit) error {
	c.commits = append(c.commits, commit)
	return c.CacheService.CacheV2Commit(ctx, commit)
}

// hangingCache blocks every V2 commit until its ctx ends
type hangingCache struct {
	cache.CacheService
}

func (c *hangingCache) CacheV2Commit(ctx context.Context, _ cache.V2Commit) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestV2CacheBound(t *testing.T) {
	fx := newV2Fixture(t)
	defer fx.finish(t)
	old := cacheCommitTimeout
	cacheCommitTimeout = 100 * time.Millisecond
	defer func() { cacheCommitTimeout = old }()
	fx.v2.mu.Lock()
	fx.v2LoadLocked()
	fx.v2.mu.Unlock()
	fx.service.cache = &hangingCache{CacheService: fx.service.cache}

	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *psp.MembershipV2_GetStatusRequest) (*psp.MembershipV2_GetStatusResponse, error) {
			return statusResponse("p1"), nil
		})
	start := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
		assert.NoError(t, err)
		assert.Equal(t, model.MembershipV2_FRESH, resp.FetchState.Freshness)
	}()
	// cache-only reads and ProvideStat wait at most for the bounded commit
	for range 5 {
		_, err := fx.getProducts(t, false)
		require.NoError(t, err)
		_ = fx.ProvideStat()
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a hanging cache commit held the state lock")
	}
	assert.Less(t, time.Since(start), time.Second)
	assert.Equal(t, int64(1), fx.stats.persistFailures.Load())
	fx.v2.mu.Lock()
	assert.True(t, fx.v2.res[v2Status].unpersisted)
	fx.v2.mu.Unlock()
}

// flakyCache fails the first load and every commit
type flakyCache struct {
	cache.CacheService
	loads int
}

func (c *flakyCache) CacheV2Load(ctx context.Context) (cache.V2Snapshot, error) {
	c.loads++
	if c.loads == 1 {
		return cache.V2Snapshot{}, errors.New("database is locked")
	}
	return c.CacheService.CacheV2Load(ctx)
}

func (c *flakyCache) CacheV2Commit(context.Context, cache.V2Commit) error {
	return errors.New("database is locked")
}

func TestV2LateLoadKeepsMemory(t *testing.T) {
	dir := t.TempDir()
	writeLegacyV2(t, dir, &cache.StorageV2Struct{
		CurrentVersion: 1,
		V2Data: &model.MembershipV2Data{Products: []*model.MembershipV2PurchasedProduct{
			{Product: &model.MembershipV2Product{Id: "p1"}},
		}},
		ProductsData: []*model.MembershipV2Product{},
	})
	fx := newFixture(t, withV2(), withRealCache(dir))
	defer fx.finish(t)
	fx.service.cache = &flakyCache{CacheService: fx.service.cache}

	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p2"), nil)
	fresh, err := fx.getStatus(t, true) // the load fails, the write fails
	require.NoError(t, err)
	cached, err := fx.getStatus(t, false) // the load now succeeds (disk has p1)
	require.NoError(t, err)
	assert.Equal(t, []string{"p2"}, purchasedIds(cached.Data))
	assert.Equal(t, fresh.FetchState.Revision, cached.FetchState.Revision)
}

// hangingLoadCache blocks every V2 load until its ctx ends
type hangingLoadCache struct {
	cache.CacheService
}

func (c *hangingLoadCache) CacheV2Load(ctx context.Context) (cache.V2Snapshot, error) {
	<-ctx.Done()
	return cache.V2Snapshot{}, ctx.Err()
}

// failFirstLoad fails the first load only; commits pass through
type failFirstLoad struct {
	cache.CacheService
	loads int
}

func (c *failFirstLoad) CacheV2Load(ctx context.Context) (cache.V2Snapshot, error) {
	c.loads++
	if c.loads == 1 {
		return cache.V2Snapshot{}, errors.New("database is locked")
	}
	return c.CacheService.CacheV2Load(ctx)
}

func writeVerifiedProducts(t *testing.T, dir string) {
	writeLegacyV2(t, dir, &cache.StorageV2Struct{
		CurrentVersion:        1,
		MetaVersion:           1,
		ExpireTime:            time.Now().Add(time.Hour),
		V2Data:                &model.MembershipV2Data{},
		ProductsData:          []*model.MembershipV2Product{{Id: "p1"}},
		ProductsFetched:       true,
		ProductsLastSuccessAt: time.Now(),
	})
}

// assertOnePayloadPerRevision checks the invariant across answers
func assertOnePayloadPerRevision(t *testing.T, answers ...*pb.RpcMembershipV2GetProductsResponse) {
	t.Helper()
	seen := map[uint64]int{}
	for _, a := range answers {
		rev := a.FetchState.Revision.Counter
		if n, ok := seen[rev]; ok {
			assert.Equal(t, n, len(a.Products), "revision %d labels two payloads", rev)
		}
		seen[rev] = len(a.Products)
	}
}

func TestV2LateLoad(t *testing.T) {
	t.Run("a late load after a NONE answer gets a new revision and owes a recovery", func(t *testing.T) {
		dir := t.TempDir()
		writeVerifiedProducts(t, dir)
		fx := newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		fc := &failFirstLoad{CacheService: fx.service.cache}
		fx.service.cache = fc

		a, err := fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_NONE, a.FetchState.Freshness)
		b, err := fx.getProducts(t, false) // the failed load is retried
		require.NoError(t, err)
		assert.Equal(t, 2, fc.loads)
		require.Len(t, b.Products, 1)
		assert.Greater(t, b.FetchState.Revision.Counter, a.FetchState.Revision.Counter)
		assertOnePayloadPerRevision(t, a, b)

		// the client moves off NONE: the verifying poll publishes
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse(), nil)
		_, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.Contains(t, eventSummaries(fx.events.all()), "products:FRESH:2")
	})

	t.Run("a late load after a failure answered NONE gets a new revision", func(t *testing.T) {
		dir := t.TempDir()
		writeVerifiedProducts(t, dir)
		fx := newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		fx.service.cache = &failFirstLoad{CacheService: fx.service.cache}
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(nil, errors.New("permission denied"))
		a, _ := fx.getProducts(t, true)
		b, err := fx.getProducts(t, false)
		require.NoError(t, err)
		require.Len(t, b.Products, 1)
		assertOnePayloadPerRevision(t, a, b)
	})

	t.Run("a late load after a transient status failure: revision alias", func(t *testing.T) {
		dir := t.TempDir()
		writeLegacyV2(t, dir, &cache.StorageV2Struct{
			CurrentVersion: 1,
			V2Data: &model.MembershipV2Data{Products: []*model.MembershipV2PurchasedProduct{
				{Product: &model.MembershipV2Product{Id: "p1"}},
			}},
			ProductsData: []*model.MembershipV2Product{},
		})
		fx := newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		fx.service.cache = &flakyCache{CacheService: fx.service.cache}
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, ErrNoConnection)
		first, _ := fx.getStatus(t, true)
		second, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"p1"}, purchasedIds(second.Data))
		assert.NotEqual(t, first.FetchState.Revision.Counter, second.FetchState.Revision.Counter)
	})

	t.Run("a late load keeps the in-memory products and expiry", func(t *testing.T) {
		dir := t.TempDir()
		writeVerifiedProducts(t, dir) // disk: products p1, expiry +1h
		fx := newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		fx.service.cache = &flakyCache{CacheService: fx.service.cache}
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p2"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("s"), nil)
		_, err := fx.refreshV2(ctx, true) // load fails, both writes fail
		require.NoError(t, err)
		fx.v2.mu.Lock()
		memExpiry := fx.v2.expireTime
		fx.v2.mu.Unlock()
		resp, err := fx.getProducts(t, false) // the load now succeeds
		require.NoError(t, err)
		require.Len(t, resp.Products, 1)
		assert.Equal(t, "p2", resp.Products[0].Id)
		fx.v2.mu.Lock()
		assert.Equal(t, memExpiry, fx.v2.expireTime)
		fx.v2.mu.Unlock()
	})
}

func TestV2Verification(t *testing.T) {
	t.Run("restart: the loop verifies even with both resources and an unexpired expiry", func(t *testing.T) {
		dir := t.TempDir()
		now := time.Now()
		writeLegacyV2(t, dir, &cache.StorageV2Struct{
			CurrentVersion:        1,
			MetaVersion:           1,
			ExpireTime:            now.Add(time.Hour),
			V2Data:                &model.MembershipV2Data{TeamOwnerID: "o"},
			StatusFetched:         true,
			StatusLastSuccessAt:   now,
			ProductsData:          []*model.MembershipV2Product{{Id: "p1"}},
			ProductsFetched:       true,
			ProductsLastSuccessAt: now,
		})
		fx := newFixture(t, withV2(), withRealCache(dir))
		defer fx.finish(t)
		assert.True(t, fx.v2NeedsFetch(), "nothing read yet, but unverified in this epoch")
	})

	t.Run("a verified success older than the lifetime is STALE", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err := fx.getProducts(t, true)
		require.NoError(t, err)
		fx.v2.mu.Lock()
		fx.v2.res[v2Products].lastSuccessAt = time.Now().Add(-time.Hour)
		fx.v2.mu.Unlock()
		resp, err := fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)
	})

	t.Run("a superseded success behind a newer success and a newest failure: STALE, no error", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		aStarted, releaseA := make(chan struct{}), make(chan struct{})
		gomock.InOrder(
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					close(aStarted)
					<-releaseA
					return productsResponse("old"), nil
				}),
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("new"), nil),
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(nil, psp.ErrInvalidSignature),
		)
		aDone := make(chan error)
		var a *pb.RpcMembershipV2GetProductsResponse
		go func() {
			var err error
			a, err = fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
			aDone <- err
		}()
		<-aStarted
		_, err := fx.getProducts(t, true) // newer success
		require.NoError(t, err)
		_, err = fx.getProducts(t, true) // newest, permanent failure
		require.Error(t, err)
		close(releaseA)
		require.NoError(t, <-aDone, "never another caller's error")
		assert.Equal(t, model.MembershipV2_STALE, a.FetchState.Freshness)
		assert.Equal(t, "new", a.Products[0].Id)
	})
}

func TestV2LoadBound(t *testing.T) {
	fx := newV2Fixture(t)
	defer fx.finish(t)
	old := cacheCommitTimeout
	cacheCommitTimeout = 50 * time.Millisecond
	defer func() { cacheCommitTimeout = old }()
	fx.service.cache = &hangingLoadCache{CacheService: fx.service.cache}

	start := time.Now()
	resp, err := fx.getStatus(t, false)
	require.NoError(t, err)
	assert.Equal(t, model.MembershipV2_NONE, resp.FetchState.Freshness)
	assert.Less(t, time.Since(start), time.Second)
}

func TestV2Telemetry(t *testing.T) {
	fx := newV2Fixture(t)
	defer fx.finish(t)
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
	_, err := fx.getStatus(t, true)
	require.NoError(t, err)
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, psp.ErrInvalidSignature)
	_, err = fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
	require.Error(t, err)
	rstats := fx.stats.resource(v2Status)
	assert.Equal(t, int64(1), rstats.failures.Load())
	assert.Zero(t, rstats.transientFailures.Load(), "a permanent failure is not transient")
	assert.Equal(t, int64(1), rstats.answeredError.Load())
	assert.Equal(t, int64(fx.limitsNotifications.Load()), fx.stats.limitsNotifications.Load())
	assert.Equal(t, int64(1), fx.stats.limitsNotifications.Load())
}

func TestV2StatProviderLifecycle(t *testing.T) {
	stats := &fakeStatService{}
	fx := newFixture(t, withV2(), withRealCache(t.TempDir()), withStatService(stats))
	assert.True(t, stats.has(fx.service))
	fx.finish(t)
	assert.False(t, stats.has(fx.service))
}

func TestV2LastErrorBounded(t *testing.T) {
	var r resourceStats
	r.failed(errors.New(strings.Repeat("x", 10000)))
	assert.Len(t, r.lastError.Load(), maxLastErrorLen)
	assert.Equal(t, 256, maxLastErrorLen)

	// a multi-byte rune across the limit is not split
	msg := strings.Repeat("x", maxLastErrorLen-1) + "é"
	assert.True(t, utf8.ValidString(truncateErr(msg)))
	assert.Len(t, truncateErr(msg), maxLastErrorLen-1)

	fx := newV2Fixture(t)
	defer fx.finish(t)
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, errors.New(strings.Repeat("y", 10000)))
	_, _ = fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
	st := fx.ProvideStat().(paymentsStat)
	assert.LessOrEqual(t, len(st.Status.LastErr), maxLastErrorLen)
	assert.LessOrEqual(t, len(st.Status.LastError), maxLastErrorLen)
}

func TestV2RecoveryWithFailedWrite(t *testing.T) {
	fx := newV2Fixture(t)
	defer fx.finish(t)
	fc := &failingCommitCache{CacheService: fx.service.cache}
	fx.service.cache = fc
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
	_, _ = fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
	fc.fail = true
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
	resp, err := fx.getStatus(t, true)
	require.NoError(t, err)
	assert.Equal(t, model.MembershipV2_FRESH, resp.FetchState.Freshness)
	fx.v2.mu.Lock()
	assert.NoError(t, fx.v2.res[v2Status].lastErr, "a recovery clears the error even if its write failed")
	fx.v2.mu.Unlock()
}

func TestV2PersistWindow(t *testing.T) {
	assert.Equal(t, 5*time.Minute, persistUnchangedAfter())
	assert.Less(t, persistUnchangedAfter(), cache.CacheV2Lifetime())

	t.Run("a skipped write doesn't advance the persisted time", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil).Times(2)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)
		fx.v2.mu.Lock()
		persisted := fx.v2.res[v2Status].persistedSuccessAt
		fx.v2.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		_, err = fx.getStatus(t, true)
		require.NoError(t, err)
		fx.v2.mu.Lock()
		rs := fx.v2.res[v2Status]
		fx.v2.mu.Unlock()
		assert.Equal(t, persisted, rs.persistedSuccessAt)
		assert.True(t, rs.lastSuccessAt.After(persisted))
	})

	t.Run("a persisted time in the future (wall clock stepped back) is rewritten", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		cc := &countingCache{CacheService: fx.service.cache}
		fx.service.cache = cc
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil).Times(2)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)
		fx.v2.mu.Lock()
		fx.v2.res[v2Status].persistedSuccessAt = time.Now().Add(time.Hour)
		fx.v2.mu.Unlock()
		_, err = fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Len(t, cc.commits, 2)
	})
}

func TestV2PersistUnchanged(t *testing.T) {
	fx := newV2Fixture(t)
	defer fx.finish(t)
	cc := &countingCache{CacheService: fx.service.cache}
	fx.service.cache = cc

	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil).Times(3)
	_, err := fx.getStatus(t, true)
	require.NoError(t, err)
	require.Len(t, cc.commits, 1, "first success is written")

	_, err = fx.getStatus(t, true)
	require.NoError(t, err)
	assert.Len(t, cc.commits, 1, "unchanged success with a recent persisted time is not written")

	// the persisted success time got old: written again
	fx.v2.mu.Lock()
	fx.v2.res[v2Status].persistedSuccessAt = time.Now().Add(-persistUnchangedAfter())
	fx.v2.mu.Unlock()
	_, err = fx.getStatus(t, true)
	require.NoError(t, err)
	assert.Len(t, cc.commits, 2)

	// a recovery after a background failure (no STALE answer, so no debt)
	// is always written
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
	fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
	_, err = fx.refreshV2(ctx, true)
	require.Error(t, err)
	fx.v2.mu.Lock()
	require.False(t, fx.v2.res[v2Status].recoveryOwed)
	fx.v2.mu.Unlock()
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
	_, err = fx.getStatus(t, true)
	require.NoError(t, err)
	assert.Len(t, cc.commits, 3)

	// a change is always written
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p2"), nil)
	_, err = fx.getStatus(t, true)
	require.NoError(t, err)
	assert.Len(t, cc.commits, 4)
}

func writeLegacyV2(t *testing.T, dir string, entry *cache.StorageV2Struct) {
	provider, err := anystoreprovider.NewInPath(dir)
	require.NoError(t, err)
	require.NoError(t, provider.Init(nil))
	store := keyvaluestore.NewJsonFromCollection[*cache.StorageV2Struct](provider.GetSystemCollection())
	require.NoError(t, store.Set(ctx, anystoreprovider.SystemKeys.PaymentCacheV2Key(1), entry))
	require.NoError(t, provider.Close(ctx))
}

func TestV2OutcomeOrdering(t *testing.T) {
	t.Run("older failure completing after a newer success is dropped", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)

		olderStarted := make(chan struct{})
		releaseOlder := make(chan struct{})
		gomock.InOrder(
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					close(olderStarted)
					<-releaseOlder
					return nil, net.ErrUnableToConnect
				}),
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil),
		)
		olderDone := make(chan *pb.RpcMembershipV2GetProductsResponse)
		go func() {
			resp, err := fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
			assert.NoError(t, err)
			olderDone <- resp
		}()
		<-olderStarted
		newer, err := fx.getProducts(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_FRESH, newer.FetchState.Freshness)

		close(releaseOlder)
		older := <-olderDone
		// the caller gets the current committed snapshot
		assert.Equal(t, model.MembershipV2_FRESH, older.FetchState.Freshness)
		require.Len(t, older.Products, 1)
		fx.v2.mu.Lock()
		assert.NoError(t, fx.v2.res[v2Products].lastErr, "older failure must not restore lastErr")
		fx.v2.mu.Unlock()
		assert.Equal(t, int64(1), fx.stats.resource(v2Products).superseded.Load())
		assert.Equal(t, []string{"products:FRESH:1"}, eventSummaries(fx.events.all()))
	})

	t.Run("older success completing after a newer failure: applied, the failure stays", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err := fx.getProducts(t, true)
		require.NoError(t, err)

		olderStarted := make(chan struct{})
		releaseOlder := make(chan struct{})
		gomock.InOrder(
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					close(olderStarted)
					<-releaseOlder
					return productsResponse("p2"), nil
				}),
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect),
		)
		olderDone := make(chan *pb.RpcMembershipV2GetProductsResponse)
		go func() {
			resp, err := fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
			assert.NoError(t, err)
			olderDone <- resp
		}()
		<-olderStarted
		newer, err := fx.getProducts(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, newer.FetchState.Freshness)

		close(releaseOlder)
		older := <-olderDone
		assert.Equal(t, model.MembershipV2_STALE, older.FetchState.Freshness, "a newer failure is still the latest outcome")
		require.Len(t, older.Products, 1)
		assert.Equal(t, "p2", older.Products[0].Id, "the newer data is applied")
		assert.Greater(t, older.FetchState.Revision.Counter, newer.FetchState.Revision.Counter)
		cached, err := fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Equal(t, "p2", cached.Products[0].Id)
		assert.Equal(t, older.FetchState.Revision, cached.FetchState.Revision)
		assert.Equal(t, model.MembershipV2_STALE, cached.FetchState.Freshness)
		assert.Equal(t, []string{"products:FRESH:1"}, eventSummaries(fx.events.all()), "no FRESH event while failing")

		// the next success clears the failure and publishes
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p2"), nil)
		_, err = fx.getProducts(t, true)
		require.NoError(t, err)
		assert.Len(t, eventSummaries(fx.events.all()), 2)
	})

	t.Run("an impatient caller's timeout doesn't discard an older fetch's newer data", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err := fx.getProducts(t, true)
		require.NoError(t, err)

		started, release := make(chan struct{}), make(chan struct{})
		gomock.InOrder(
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					close(started)
					<-release
					return productsResponse("p1", "p2"), nil
				}),
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(c context.Context, _ *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					<-c.Done()
					return nil, c.Err()
				}),
		)
		done := make(chan *pb.RpcMembershipV2GetProductsResponse)
		go func() {
			resp, _ := fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
			done <- resp
		}()
		<-started
		short, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		_, _ = fx.V2GetProducts(short, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
		close(release)
		<-done
		cached, err := fx.getProducts(t, false)
		require.NoError(t, err)
		assert.Len(t, cached.Products, 2)
	})

	t.Run("status commits in reverse fetch order: the older one is dropped", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)

		seq1 := fx.v2AllocSeq(v2Status)
		seq2 := fx.v2AllocSeq(v2Status)
		data := func(id string) *model.MembershipV2Data {
			return &model.MembershipV2Data{Products: []*model.MembershipV2PurchasedProduct{{Product: &model.MembershipV2Product{Id: id}}}}
		}
		resp, _ := fx.commitV2(ctx, []v2Result{{res: v2Status, seq: seq2, status: data("new")}}, true, false)
		assert.Equal(t, model.MembershipV2_FRESH, resp[v2Status].state.Freshness)
		resp, _ = fx.commitV2(ctx, []v2Result{{res: v2Status, seq: seq1, status: data("old")}}, true, false)
		assert.Equal(t, []string{"new"}, purchasedIds(resp[v2Status].status))

		status, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"new"}, purchasedIds(status.Data))
	})

	t.Run("timeout racing an unchanged success: the caller is never left STALE", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		old := getProductsTimeout
		getProductsTimeout = 100 * time.Millisecond
		defer func() { getProductsTimeout = old }()

		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err := fx.getProducts(t, true)
		require.NoError(t, err)

		// A (older) times out after B (newer) committed an unchanged success:
		// A is superseded and gets B's FRESH snapshot, no event is needed
		aStarted := make(chan struct{})
		gomock.InOrder(
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(callCtx context.Context, _ *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					close(aStarted)
					<-callCtx.Done()
					return nil, callCtx.Err()
				}),
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil),
		)
		aDone := make(chan *pb.RpcMembershipV2GetProductsResponse)
		go func() {
			resp, err := fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
			assert.NoError(t, err)
			aDone <- resp
		}()
		<-aStarted
		_, err = fx.getProducts(t, true)
		require.NoError(t, err)
		a := <-aDone
		assert.Equal(t, model.MembershipV2_FRESH, a.FetchState.Freshness)
		assert.Equal(t, []string{"products:FRESH:1"}, eventSummaries(fx.events.all()))
	})
}

func TestV2StatusBudget(t *testing.T) {
	t.Run("occupied limiter: STALE within budget without calling GetStatus, debt owed", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		old := getStatusV2Budget
		getStatusV2Budget = 50 * time.Millisecond
		defer func() { getStatusV2Budget = old }()

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)

		fx.getStatusV2Limiter <- struct{}{} // a stuck fetch holds the limiter
		start := time.Now()
		resp, err := fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Less(t, time.Since(start), time.Second)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)
		assert.Equal(t, model.MembershipV2_RefreshErrorPaymentNode, resp.FetchState.LastRefreshError)
		fx.v2.mu.Lock()
		rs := fx.v2.res[v2Status]
		fx.v2.mu.Unlock()
		assert.NoError(t, rs.lastErr, "leaving the queue is not a resource outcome")
		assert.Equal(t, uint64(1), rs.acceptedSeq, "the accepted watermark doesn't move")
		assert.True(t, rs.recoveryOwed)
		assert.Equal(t, int64(1), fx.stats.resource(v2Status).queueExits.Load())
		<-fx.getStatusV2Limiter

		// the owed recovery: an unchanged success publishes FRESH
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err = fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Equal(t, []string{"status:FRESH:1", "status:FRESH:2"}, eventSummaries(fx.events.all()))
	})

	t.Run("an expired budget with a free limiter is a queue exit, not an outcome", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		old := getStatusV2Budget
		getStatusV2Budget = 0
		defer func() { getStatusV2Budget = old }()
		// select picks randomly between the expired budget and the free
		// limiter: both paths must leave the resource state untouched
		for range 50 {
			_, err := fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
			require.ErrorIs(t, err, context.DeadlineExceeded)
		}
		fx.v2.mu.Lock()
		rs := fx.v2.res[v2Status]
		fx.v2.mu.Unlock()
		assert.Zero(t, rs.acceptedSeq)
		assert.Zero(t, rs.nextSeq)
		assert.NoError(t, rs.lastErr)
		assert.Equal(t, int64(50), fx.stats.resource(v2Status).queueExits.Load())
	})

	t.Run("a canceled caller is the caller giving up, not an outcome", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		for range 50 {
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			_, err := fx.V2GetStatus(canceled, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
			require.ErrorIs(t, err, context.Canceled)
		}
		fx.v2.mu.Lock()
		rs := fx.v2.res[v2Status]
		fx.v2.mu.Unlock()
		assert.Zero(t, rs.nextSeq)
		assert.NoError(t, rs.lastErr)
		assert.Equal(t, int64(50), fx.stats.resource(v2Status).callerGone.Load())
	})

	t.Run("a caller deadline reached while queued is a queue exit", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.getStatusV2Limiter <- struct{}{}
		defer func() { <-fx.getStatusV2Limiter }()
		short, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		_, err := fx.V2GetStatus(short, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, int64(1), fx.stats.resource(v2Status).queueExits.Load())
		assert.Zero(t, fx.stats.resource(v2Status).callerGone.Load())
	})

	t.Run("a caller deadline after acquiring the limiter is a queue exit", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		for range 50 {
			expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
			_, _ = fx.V2GetStatus(expired, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
			cancel()
		}
		assert.Equal(t, int64(50), fx.stats.resource(v2Status).queueExits.Load())
		assert.Zero(t, fx.stats.resource(v2Status).callerGone.Load())
	})

	t.Run("a caller deadline shorter than the budget is a timeout outcome", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(callCtx context.Context, _ *psp.MembershipV2_GetStatusRequest) (*psp.MembershipV2_GetStatusResponse, error) {
				<-callCtx.Done()
				return nil, callCtx.Err()
			})
		short, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		resp, err := fx.V2GetStatus(short, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)
		rstats := fx.stats.resource(v2Status)
		assert.Equal(t, int64(1), rstats.timeouts.Load())
		assert.Equal(t, int64(1), rstats.failures.Load())
		assert.Zero(t, rstats.callerGone.Load())
	})

	t.Run("occupied limiter with cold cache returns the deadline error", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		old := getStatusV2Budget
		getStatusV2Budget = 50 * time.Millisecond
		defer func() { getStatusV2Budget = old }()

		fx.getStatusV2Limiter <- struct{}{}
		defer func() { <-fx.getStatusV2Limiter }()
		resp, err := fx.getStatus(t, true)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, model.MembershipV2_NONE, resp.FetchState.Freshness)
	})

	t.Run("incident path: warm cache, hanging GetStatus hits the budget", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		old := getStatusV2Budget
		getStatusV2Budget = 50 * time.Millisecond
		defer func() { getStatusV2Budget = old }()
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(callCtx context.Context, _ *psp.MembershipV2_GetStatusRequest) (*psp.MembershipV2_GetStatusResponse, error) {
				<-callCtx.Done()
				return nil, callCtx.Err()
			})
		resp, err := fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)
		assert.Equal(t, model.MembershipV2_RefreshErrorPaymentNode, resp.FetchState.LastRefreshError)
		fx.v2.mu.Lock()
		lastErr := fx.v2.res[v2Status].lastErr
		fx.v2.mu.Unlock()
		assert.ErrorIs(t, lastErr, context.DeadlineExceeded)
		rstats := fx.stats.resource(v2Status)
		assert.Equal(t, int64(1), rstats.failures.Load())
		assert.Equal(t, int64(1), rstats.timeouts.Load())
		assert.Zero(t, rstats.callerGone.Load())
		assert.Equal(t, int64(1), rstats.answeredStale.Load())
	})

	t.Run("a hanging GetStatus is bounded by the budget", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		old := getStatusV2Budget
		getStatusV2Budget = 50 * time.Millisecond
		defer func() { getStatusV2Budget = old }()

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(callCtx context.Context, _ *psp.MembershipV2_GetStatusRequest) (*psp.MembershipV2_GetStatusResponse, error) {
				<-callCtx.Done()
				return nil, callCtx.Err()
			})
		start := time.Now()
		_, err := fx.getStatus(t, true)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(start), time.Second)
		assert.Len(t, fx.getStatusV2Limiter, 0, "limiter released")
	})

	t.Run("caller cancellation is not transient: no STALE fallback", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)

		cctx, cancel := context.WithCancel(ctx)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(context.Context, *psp.MembershipV2_GetStatusRequest) (*psp.MembershipV2_GetStatusResponse, error) {
				cancel()
				// the pool can replace caller cancellation with this
				return nil, net.ErrUnableToConnect
			})
		resp, err := fx.V2GetStatus(cctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
		require.Error(t, err)
		assert.Equal(t, model.MembershipV2_STALE, resp.FetchState.Freshness)
	})
}

// failingCommitCache wraps the real cache and fails CacheV2Commit on demand
type failingCommitCache struct {
	cache.CacheService
	fail bool
}

func (c *failingCommitCache) CacheV2Commit(ctx context.Context, commit cache.V2Commit) error {
	if c.fail {
		return errors.New("disk full")
	}
	return c.CacheService.CacheV2Commit(ctx, commit)
}

func TestV2Commit(t *testing.T) {
	t.Run("persistence failure: memory is authoritative, published, unpersisted retried", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fc := &failingCommitCache{CacheService: fx.service.cache}
		fx.service.cache = fc

		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("x"), nil)
		_, err := fx.refreshV2(ctx, false)
		require.NoError(t, err)

		fc.fail = true
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p2"), nil)
		resp, err := fx.getStatus(t, true)
		require.NoError(t, err)
		assert.Equal(t, []string{"p2"}, purchasedIds(resp.Data))
		assert.Equal(t, model.MembershipV2_FRESH, resp.FetchState.Freshness)
		assert.Equal(t, int64(1), fx.stats.persistFailures.Load())
		assert.Equal(t, int32(2), fx.limitsNotifications.Load(), "a change is a change")

		// the same revision labels the same payload on every path
		cached, err := fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"p2"}, purchasedIds(cached.Data))
		assert.Equal(t, resp.FetchState.Revision, cached.FetchState.Revision)
		assert.Equal(t, model.MembershipV2_FRESH, cached.FetchState.Freshness)
		assert.Equal(t, []string{"products:FRESH:1", "status:FRESH:1", "status:FRESH:2"}, eventSummaries(fx.events.all()))

		// unpersisted: the loop fetches although nothing else is pending; the
		// write fails again, nothing new is published, nothing regresses
		fx.v2.mu.Lock()
		require.True(t, fx.v2.res[v2Status].unpersisted)
		fx.v2.mu.Unlock()
		require.True(t, fx.v2NeedsFetch())
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p2"), nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("x"), nil)
		_, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		cached, err = fx.getStatus(t, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"p2"}, purchasedIds(cached.Data))
		assert.Equal(t, resp.FetchState.Revision, cached.FetchState.Revision)

		// once the write works, the unchanged success is written
		fc.fail = false
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p2"), nil)
		_, err = fx.getStatus(t, true)
		require.NoError(t, err)
		snap, err := fc.CacheService.CacheV2Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"p2"}, purchasedIds(snap.Status))
		fx.v2.mu.Lock()
		assert.False(t, fx.v2.res[v2Status].unpersisted)
		fx.v2.mu.Unlock()
		assert.Equal(t, []string{"products:FRESH:1", "status:FRESH:1", "status:FRESH:2"}, eventSummaries(fx.events.all()))
	})

	t.Run("superseded success after a newer unpersisted success: one payload per revision", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fc := &failingCommitCache{CacheService: fx.service.cache}
		fx.service.cache = fc
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		_, err := fx.getProducts(t, true)
		require.NoError(t, err)

		started, release := make(chan struct{}), make(chan struct{})
		gomock.InOrder(
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					close(started)
					<-release
					return productsResponse("p2"), nil
				}),
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p3"), nil),
		)
		done := make(chan *pb.RpcMembershipV2GetProductsResponse)
		go func() {
			r, _ := fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
			done <- r
		}()
		<-started
		fc.fail = true
		newer, err := fx.getProducts(t, true)
		require.NoError(t, err)
		close(release)
		older := <-done
		assert.Equal(t, newer.FetchState.Revision, older.FetchState.Revision)
		assert.Equal(t, "p3", newer.Products[0].Id)
		assert.Equal(t, "p3", older.Products[0].Id)
	})

	t.Run("older success when never fetched and the newer failed: applied STALE, no error", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		started, release := make(chan struct{}), make(chan struct{})
		gomock.InOrder(
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
					close(started)
					<-release
					return productsResponse("p1"), nil
				}),
			fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect),
		)
		done := make(chan error)
		var older *pb.RpcMembershipV2GetProductsResponse
		go func() {
			var err error
			older, err = fx.V2GetProducts(ctx, &pb.RpcMembershipV2GetProductsRequest{NoCache: true})
			done <- err
		}()
		<-started
		_, err := fx.getProducts(t, true)
		require.Error(t, err)
		close(release)
		require.NoError(t, <-done)
		assert.Equal(t, model.MembershipV2_STALE, older.FetchState.Freshness)
		assert.Equal(t, model.MembershipV2_RefreshErrorPaymentNode, older.FetchState.LastRefreshError)
		require.Len(t, older.Products, 1)
	})

	t.Run("an error-code change bumps the revision, a repeat doesn't", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		ok, err := fx.getStatus(t, true)
		require.NoError(t, err)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect).Times(2)
		a, err := fx.getStatus(t, true)
		require.NoError(t, err)
		b, err := fx.getStatus(t, true)
		require.NoError(t, err)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, psp.ErrInvalidSignature)
		c, _ := fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
		assert.Equal(t, ok.FetchState.Revision.Counter+1, a.FetchState.Revision.Counter)
		assert.Equal(t, a.FetchState.Revision.Counter, b.FetchState.Revision.Counter)
		assert.Equal(t, b.FetchState.Revision.Counter+1, c.FetchState.Revision.Counter)
		assert.Equal(t, model.MembershipV2_RefreshErrorUnknown, c.FetchState.LastRefreshError)
	})

	t.Run("no partial expiry renewal", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		expiry := func() time.Time {
			snap, err := fx.service.cache.CacheV2Load(ctx)
			require.NoError(t, err)
			return snap.ExpireTime
		}

		// products changed, status failed: no renewal
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
		changed, err := fx.refreshV2(ctx, false)
		require.Error(t, err)
		assert.True(t, changed)
		assert.True(t, expiry().IsZero())

		// single-resource user success with a change: no renewal
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err = fx.getStatus(t, true)
		require.NoError(t, err)
		assert.True(t, expiry().IsZero())

		// paired success with a change: renewed
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1", "p2"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		changed, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.True(t, expiry().After(time.Now()))

		// both fetched, unexpired, nothing pending: the loop skips the network
		changed, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.False(t, changed)

		// P0 keeps today's rule: an unchanged paired success doesn't renew
		before := expiry()
		fx.v2.mu.Lock()
		memBefore := fx.v2.expireTime
		fx.v2.mu.Unlock()
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1", "p2"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		changed, err = fx.refreshV2(ctx, true)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, before, expiry())
		fx.v2.mu.Lock()
		assert.Equal(t, memBefore, fx.v2.expireTime)
		fx.v2.mu.Unlock()
	})

	t.Run("background loop fetches despite unexpired cache when an error is pending", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.refreshV2(ctx, false)
		require.NoError(t, err)

		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
		_, err = fx.getProducts(t, true)
		require.NoError(t, err)

		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"products:FRESH:1", "status:FRESH:1", "products:FRESH:3"}, eventSummaries(fx.events.all()))
	})

	t.Run("background loop fetches despite unexpired cache after a background failure", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err := fx.refreshV2(ctx, false)
		require.NoError(t, err)

		// a forced poll fails: an error is pending, but nobody was answered
		// STALE, so no recovery debt exists
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err = fx.refreshV2(ctx, true)
		require.Error(t, err)
		fx.v2.mu.Lock()
		owed := fx.v2.res[v2Products].recoveryOwed
		fx.v2.mu.Unlock()
		require.False(t, owed)

		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse("p1"), nil)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
		_, err = fx.refreshV2(ctx, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"products:FRESH:1", "status:FRESH:1", "products:FRESH:3"}, eventSummaries(fx.events.all()))
	})

	t.Run("field-scoped: a products commit never rewrites status", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("s1"), nil)
		_, err := fx.getStatus(t, true)
		require.NoError(t, err)

		// a products call that started before the status commit finishes
		// after it
		seq := fx.v2AllocSeq(v2Products)
		fx.commitV2(ctx, []v2Result{{res: v2Products, seq: seq, products: []*model.MembershipV2Product{{Id: "p"}}}}, true, false)

		snap, err := fx.service.cache.CacheV2Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"s1"}, purchasedIds(snap.Status))
		assert.True(t, snap.StatusFetched)
		assert.True(t, snap.ProductsFetched)
	})
}

func TestV2ManualRefresh(t *testing.T) {
	t.Run("forceRefreshSec admits a manual window without waiting for a blocked controller fetch", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		require.NotNil(t, fx.refreshCtrlV2)

		controllerBlocked := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		// the controller's purchase poll hangs in GetProducts
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).DoAndReturn(
			func(callCtx context.Context, _ *psp.MembershipV2_GetProductsRequest) (*psp.MembershipV2_GetProductsResponse, error) {
				close(controllerBlocked)
				select {
				case <-release:
				case <-callCtx.Done():
				}
				return nil, net.ErrUnableToConnect
			})
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil).AnyTimes()
		fx.forceRefreshV2(30 * time.Minute)
		<-controllerBlocked

		start := time.Now()
		resp, err := fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{ForceRefreshSec: 600})
		require.NoError(t, err)
		assert.Less(t, time.Since(start), time.Second)
		assert.Equal(t, model.MembershipV2_FRESH, resp.FetchState.Freshness)
		assert.Equal(t, int64(1), fx.refreshCtrlV2.stats.admissions[forceOriginManual].Load())

		// a second press within 30s only fetches
		_, err = fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{ForceRefreshSec: 180})
		require.NoError(t, err)
		assert.Equal(t, int64(1), fx.refreshCtrlV2.stats.admissions[forceOriginManual].Load())
		assert.Equal(t, int64(1), fx.refreshCtrlV2.stats.manualRejected.Load())
		assert.Equal(t, int64(2), fx.stats.manualRequests.Load())
	})

	t.Run("a caller whose deadline passed opens no window", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(callCtx context.Context, _ *psp.MembershipV2_GetStatusRequest) (*psp.MembershipV2_GetStatusResponse, error) {
				<-callCtx.Done()
				return nil, callCtx.Err()
			})
		short, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		_, _ = fx.V2GetStatus(short, &pb.RpcMembershipV2GetStatusRequest{ForceRefreshSec: 180})
		assert.Zero(t, fx.refreshCtrlV2.stats.admissions[forceOriginManual].Load())
	})

	t.Run("a non-transient failure opens no window", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, psp.ErrInvalidSignature)
		_, err := fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{ForceRefreshSec: 180})
		require.Error(t, err)
		assert.Zero(t, fx.refreshCtrlV2.stats.admissions[forceOriginManual].Load())
	})

	t.Run("a transient failure on a cold cache opens a window", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
		_, err := fx.V2GetStatus(ctx, &pb.RpcMembershipV2GetStatusRequest{ForceRefreshSec: 180})
		require.Error(t, err)
		assert.Equal(t, int64(1), fx.refreshCtrlV2.stats.admissions[forceOriginManual].Load())
		// stop the forced poll the window started
		fx.refreshCtrlV2.mu.Lock()
		fx.refreshCtrlV2.intents[forceOriginManual] = forceIntent{}
		fx.refreshCtrlV2.mu.Unlock()
	})

	t.Run("V2AnyNameAllocate admits a purchase window without blocking", func(t *testing.T) {
		fx := newV2Fixture(t)
		defer fx.finish(t)
		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.Address{}).Maybe()
		fx.ppclient2.EXPECT().AnyNameAllocate(gomock.Any(), gomock.Any()).Return(&psp.MembershipV2_AnyNameAllocateResponse{}, nil)
		fx.ppclient2.EXPECT().GetProducts(gomock.Any(), gomock.Any()).Return(productsResponse(), nil).AnyTimes()
		fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil).AnyTimes()

		_, err := fx.V2AnyNameAllocate(ctx, &pb.RpcMembershipV2AnyNameAllocateRequest{NsName: "alice"})
		require.NoError(t, err)
		assert.Equal(t, int64(1), fx.refreshCtrlV2.stats.admissions[forceOriginPurchase].Load())
	})
}

func TestV2ProvideStat(t *testing.T) {
	fx := newV2Fixture(t)
	defer fx.finish(t)
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(statusResponse("p1"), nil)
	_, _ = fx.getStatus(t, true)
	fx.ppclient2.EXPECT().GetStatus(gomock.Any(), gomock.Any()).Return(nil, net.ErrUnableToConnect)
	_, _ = fx.getStatus(t, true) // accepted failure answered as STALE
	_, _ = fx.getStatus(t, false)
	_, _ = fx.getProducts(t, false)

	st := fx.ProvideStat().(paymentsStat)
	assert.True(t, st.V2Enabled)
	require.NotNil(t, st.Status)
	assert.Equal(t, "STALE", st.Status.Freshness)
	assert.Equal(t, int64(2), st.Status.Attempts["user"])
	assert.Equal(t, int64(1), st.Status.Successes)
	assert.Equal(t, int64(1), st.Status.Failures, "a failure answered as STALE still counts")
	assert.Equal(t, int64(1), st.Status.TransientFailures)
	assert.Equal(t, int64(1), st.Status.AnsweredFresh)
	assert.Equal(t, int64(1), st.Status.AnsweredStale)
	assert.Equal(t, int64(1), st.Status.CacheReadStale, "cache-only reads are counted apart from attempts")
	assert.Equal(t, int64(1), st.Products.CacheReadNone)
	assert.Zero(t, st.Products.Attempts["user"])
	require.NotNil(t, st.Force)
}
