package core

import (
	"context"
	"errors"
	"testing"

	mock_ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient/mock"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/ethereum/go-ethereum/common"

	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/pb"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/util/crypto"
)

func TestPaymentsSubscriptionGetStatus(t *testing.T) {
	t.Run("fail if GetSubscriptionStatus returns error", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		SignKey := "psqF8Rj52Ci6gsUl5ttwBVhINTP8Yowc2hea73MeFm4Ek9AxedYSB4+r7DYCclDL4WmLggj2caNapFUmsMtn5Q=="
		decodedSignKey, err := crypto.DecodeKeyFromString(
			SignKey,
			crypto.UnmarshalEd25519PrivateKey,
			nil)

		assert.NoError(t, err)

		w := mock_wallet.NewMockWallet(t)
		var ak accountdata.AccountKeys
		ak.PeerId = "123"
		ak.SignKey = decodedSignKey

		// just return something
		w.EXPECT().GetAccountPrivkey().Return(decodedSignKey)
		w.EXPECT().Account().Return(&ak)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetStatusRequest{}

		// Call the function being tested
		resp := subscriptionGetStatus(context.Background(), pp, w, req)
		assert.Equal(t, pb.RpcPaymentsSubscriptionGetStatusResponseErrorCode(pb.RpcPaymentsSubscriptionGetStatusResponseError_PAYMENT_NODE_ERROR), resp.Error.Code)
		assert.Equal(t, "test error", resp.Error.Description)
	})

	t.Run("success", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			var out psp.GetSubscriptionResponse

			out.Tier = psp.SubscriptionTier_TierFriend
			out.Status = psp.SubscriptionStatus_StatusActive
			out.DateStarted = 1234567890
			out.DateEnds = 1234567890
			out.IsAutoRenew = true
			out.PaymentMethod = psp.PaymentMethod_MethodCrypto
			out.RequestedAnyName = "something.any"

			return &out, nil
		}).MinTimes(1)

		// mock the GetAccountPrivkey method
		SignKey := "psqF8Rj52Ci6gsUl5ttwBVhINTP8Yowc2hea73MeFm4Ek9AxedYSB4+r7DYCclDL4WmLggj2caNapFUmsMtn5Q=="
		decodedSignKey, err := crypto.DecodeKeyFromString(
			SignKey,
			crypto.UnmarshalEd25519PrivateKey,
			nil)

		assert.NoError(t, err)

		w := mock_wallet.NewMockWallet(t)
		var ak accountdata.AccountKeys
		ak.PeerId = "123"
		ak.SignKey = decodedSignKey

		w.EXPECT().GetAccountPrivkey().Return(decodedSignKey)
		w.EXPECT().Account().Return(&ak)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetStatusRequest{}

		// Call the function being tested
		resp := subscriptionGetStatus(context.Background(), pp, w, req)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierFriend), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
		assert.Equal(t, uint64(1234567890), resp.DateStarted)
		assert.Equal(t, uint64(1234567890), resp.DateEnds)
		assert.Equal(t, true, resp.IsAutoRenew)
		assert.Equal(t, pb.RpcPaymentsSubscriptionPaymentMethod(1), resp.PaymentMethod)
		assert.Equal(t, "something.any", resp.RequestedAnyName)
	})
}

func TestPaymentsGetPaymentURL(t *testing.T) {
	t.Run("fail if BuySubscription method fails", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		pp.EXPECT().BuySubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.BuySubscriptionResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		// mock the GetAccountPrivkey method
		SignKey := "psqF8Rj52Ci6gsUl5ttwBVhINTP8Yowc2hea73MeFm4Ek9AxedYSB4+r7DYCclDL4WmLggj2caNapFUmsMtn5Q=="
		decodedSignKey, err := crypto.DecodeKeyFromString(
			SignKey,
			crypto.UnmarshalEd25519PrivateKey,
			nil)

		assert.NoError(t, err)

		w := mock_wallet.NewMockWallet(t)
		var ak accountdata.AccountKeys
		ak.PeerId = "123"
		ak.SignKey = decodedSignKey

		w.EXPECT().GetAccountPrivkey().Return(decodedSignKey)
		w.EXPECT().Account().Return(&ak)

		w.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))
		//ethPrivateKey := ecdsa.PrivateKey{}
		//w.EXPECT().GetAccountEthPrivkey().Return(&ethPrivateKey)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPaymentUrlRequest{
			RequestedTier:    pb.RpcPaymentsSubscription_TierPatron1Year,
			PaymentMethod:    pb.RpcPaymentsSubscription_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		// Call the function being tested
		resp := getPaymentURL(context.Background(), pp, w, req)

		assert.Equal(t, pb.RpcPaymentsSubscriptionGetPaymentUrlResponseErrorCode(pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError_PAYMENT_NODE_ERROR), resp.Error.Code)
		assert.Equal(t, "bad error", resp.Error.Description)
	})

	t.Run("success", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		pp.EXPECT().BuySubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.BuySubscriptionResponse, error) {
			var out psp.BuySubscriptionResponse
			out.PaymentUrl = "https://xxxx.com"

			return &out, nil
		}).MinTimes(1)

		// mock the GetAccountPrivkey method
		SignKey := "psqF8Rj52Ci6gsUl5ttwBVhINTP8Yowc2hea73MeFm4Ek9AxedYSB4+r7DYCclDL4WmLggj2caNapFUmsMtn5Q=="
		decodedSignKey, err := crypto.DecodeKeyFromString(
			SignKey,
			crypto.UnmarshalEd25519PrivateKey,
			nil)

		assert.NoError(t, err)

		w := mock_wallet.NewMockWallet(t)
		var ak accountdata.AccountKeys
		ak.PeerId = "123"
		ak.SignKey = decodedSignKey

		w.EXPECT().GetAccountPrivkey().Return(decodedSignKey)
		w.EXPECT().Account().Return(&ak)
		w.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))
		//ethPrivateKey := ecdsa.PrivateKey{}
		//w.EXPECT().GetAccountEthPrivkey().Return(&ethPrivateKey)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPaymentUrlRequest{
			RequestedTier:    pb.RpcPaymentsSubscription_TierPatron1Year,
			PaymentMethod:    pb.RpcPaymentsSubscription_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		// Call the function being tested
		resp := getPaymentURL(context.Background(), pp, w, req)
		assert.Equal(t, "https://xxxx.com", resp.PaymentUrl)
	})
}

func TestPaymentsGetPortalURL(t *testing.T) {
	t.Run("fail if GetPortal method fails", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		pp.EXPECT().GetSubscriptionPortalLink(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetSubscriptionPortalLinkResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		// mock the GetAccountPrivkey method
		SignKey := "psqF8Rj52Ci6gsUl5ttwBVhINTP8Yowc2hea73MeFm4Ek9AxedYSB4+r7DYCclDL4WmLggj2caNapFUmsMtn5Q=="
		decodedSignKey, err := crypto.DecodeKeyFromString(
			SignKey,
			crypto.UnmarshalEd25519PrivateKey,
			nil)

		assert.NoError(t, err)

		w := mock_wallet.NewMockWallet(t)
		var ak accountdata.AccountKeys
		ak.PeerId = "123"
		ak.SignKey = decodedSignKey

		w.EXPECT().GetAccountPrivkey().Return(decodedSignKey)
		w.EXPECT().Account().Return(&ak)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest{}

		// Call the function being tested
		resp := getPortalLink(context.Background(), pp, w, req)

		assert.Equal(t, "bad error", resp.Error.Description)
	})

	t.Run("success", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		pp.EXPECT().GetSubscriptionPortalLink(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetSubscriptionPortalLinkResponse, error) {
			return &psp.GetSubscriptionPortalLinkResponse{
				PortalUrl: "https://xxxx.com",
			}, nil
		}).MinTimes(1)

		// mock the GetAccountPrivkey method
		SignKey := "psqF8Rj52Ci6gsUl5ttwBVhINTP8Yowc2hea73MeFm4Ek9AxedYSB4+r7DYCclDL4WmLggj2caNapFUmsMtn5Q=="
		decodedSignKey, err := crypto.DecodeKeyFromString(
			SignKey,
			crypto.UnmarshalEd25519PrivateKey,
			nil)

		assert.NoError(t, err)

		w := mock_wallet.NewMockWallet(t)
		var ak accountdata.AccountKeys
		ak.PeerId = "123"
		ak.SignKey = decodedSignKey

		w.EXPECT().GetAccountPrivkey().Return(decodedSignKey)
		w.EXPECT().Account().Return(&ak)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest{}

		// Call the function being tested
		resp := getPortalLink(context.Background(), pp, w, req)

		assert.Equal(t, "https://xxxx.com", resp.PortalUrl)
	})
}
