package core

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/payments"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// fakePaymentsService returns a fixed V2 response together with an error
type fakePaymentsService struct {
	payments.Service
	status   *pb.RpcMembershipV2GetStatusResponse
	products *pb.RpcMembershipV2GetProductsResponse
	err      error
}

func (f *fakePaymentsService) V2GetStatus(context.Context, *pb.RpcMembershipV2GetStatusRequest) (*pb.RpcMembershipV2GetStatusResponse, error) {
	return f.status, f.err
}

func (f *fakePaymentsService) V2GetProducts(context.Context, *pb.RpcMembershipV2GetProductsRequest) (*pb.RpcMembershipV2GetProductsResponse, error) {
	return f.products, f.err
}

func TestMembershipV2Middleware(t *testing.T) {
	t.Run("not logged in: explicit NONE and NOT_LOGGED_IN, no panic", func(t *testing.T) {
		mw := New()
		status := mw.MembershipV2GetStatus(context.Background(), &pb.RpcMembershipV2GetStatusRequest{NoCache: true})
		require.NotNil(t, status.FetchState)
		assert.Equal(t, model.MembershipV2_NONE, status.FetchState.Freshness)
		assert.Equal(t, pb.RpcMembershipV2GetStatusResponseError_NOT_LOGGED_IN, status.Error.Code)

		products := mw.MembershipV2GetProducts(context.Background(), &pb.RpcMembershipV2GetProductsRequest{})
		require.NotNil(t, products.FetchState)
		assert.Equal(t, model.MembershipV2_NONE, products.FetchState.Freshness)
		assert.Equal(t, pb.RpcMembershipV2GetProductsResponseError_NOT_LOGGED_IN, products.Error.Code)
	})

	t.Run("keeps the service envelope on errors", func(t *testing.T) {
		state := &model.MembershipV2FetchState{
			Freshness: model.MembershipV2_STALE,
			Revision:  &model.MembershipV2Revision{Epoch: 7, Counter: 3},
		}
		data := &model.MembershipV2Data{TeamOwnerID: "owner"}
		ps := &fakePaymentsService{
			status:   &pb.RpcMembershipV2GetStatusResponse{Data: data, FetchState: state},
			products: &pb.RpcMembershipV2GetProductsResponse{Products: []*model.MembershipV2Product{{Id: "p"}}, FetchState: state},
			err:      context.DeadlineExceeded,
		}
		status := membershipV2GetStatus(context.Background(), ps, nil, &pb.RpcMembershipV2GetStatusRequest{})
		assert.Equal(t, data, status.Data)
		assert.Equal(t, state, status.FetchState)
		assert.Equal(t, pb.RpcMembershipV2GetStatusResponseError_PAYMENT_NODE_ERROR, status.Error.Code, "deadline maps to PAYMENT_NODE_ERROR")

		products := membershipV2GetProducts(context.Background(), ps, nil, &pb.RpcMembershipV2GetProductsRequest{})
		require.Len(t, products.Products, 1)
		assert.Equal(t, state, products.FetchState)
		assert.Equal(t, pb.RpcMembershipV2GetProductsResponseError_PAYMENT_NODE_ERROR, products.Error.Code)
	})

	t.Run("error without a response gets NONE", func(t *testing.T) {
		ps := &fakePaymentsService{err: net.ErrUnableToConnect}
		status := membershipV2GetStatus(context.Background(), ps, nil, &pb.RpcMembershipV2GetStatusRequest{})
		assert.Equal(t, model.MembershipV2_NONE, status.FetchState.Freshness)
		assert.Equal(t, pb.RpcMembershipV2GetStatusResponseError_PAYMENT_NODE_ERROR, status.Error.Code)
	})

	t.Run("invalid forceRefreshSec maps to BAD_INPUT", func(t *testing.T) {
		ps := &fakePaymentsService{
			status: &pb.RpcMembershipV2GetStatusResponse{FetchState: payments.NoneFetchState()},
			err:    payments.ErrInvalidForceRefresh,
		}
		status := membershipV2GetStatus(context.Background(), ps, nil, &pb.RpcMembershipV2GetStatusRequest{ForceRefreshSec: -1})
		assert.Equal(t, pb.RpcMembershipV2GetStatusResponseError_BAD_INPUT, status.Error.Code)
	})

	t.Run("success passes through", func(t *testing.T) {
		ps := &fakePaymentsService{status: &pb.RpcMembershipV2GetStatusResponse{
			FetchState: &model.MembershipV2FetchState{Freshness: model.MembershipV2_FRESH},
			Error:      &pb.RpcMembershipV2GetStatusResponseError{Code: pb.RpcMembershipV2GetStatusResponseError_NULL},
		}}
		status := membershipV2GetStatus(context.Background(), ps, nil, &pb.RpcMembershipV2GetStatusRequest{})
		assert.Equal(t, pb.RpcMembershipV2GetStatusResponseError_NULL, status.Error.Code)
		assert.Equal(t, model.MembershipV2_FRESH, status.FetchState.Freshness)
	})
}
