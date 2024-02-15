package core

import (
	"context"
	"errors"
	"testing"

	mock_ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient/mock"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"
	mock_payments "github.com/anyproto/anytype-heart/core/payments/mock_payments"

	"github.com/ethereum/go-ethereum/common"

	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/pb"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/util/crypto"
)

func TestGetStatus(t *testing.T) {
	t.Run("fail if GetSubscriptionStatus returns error", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		pp := mock_ppclient.NewMockAnyPpClientService(c)
		ps := mock_payments.NewMockService(t)

		pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		ps.EXPECT().CacheGet().Return(nil, errors.New("test error")).Once()

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
		resp := getStatus(context.Background(), pp, ps, w, req)
		assert.Equal(t, pb.RpcPaymentsSubscriptionGetStatusResponseErrorCode(pb.RpcPaymentsSubscriptionGetStatusResponseError_PAYMENT_NODE_ERROR), resp.Error.Code)
		assert.Equal(t, "test error", resp.Error.Description)
	})

	t.Run("success", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		pp := mock_ppclient.NewMockAnyPpClientService(c)
		ps := mock_payments.NewMockService(t)

		var sr psp.GetSubscriptionResponse
		sr.Tier = psp.SubscriptionTier_TierExplorer
		sr.Status = psp.SubscriptionStatus_StatusActive
		sr.DateStarted = 1234567890
		sr.DateEnds = 1234567890
		sr.IsAutoRenew = true
		sr.PaymentMethod = psp.PaymentMethod_MethodCrypto
		sr.RequestedAnyName = "something.any"

		var psgsr pb.RpcPaymentsSubscriptionGetStatusResponse
		psgsr.Tier = pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierExplorer)
		psgsr.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(2)
		psgsr.DateStarted = 1234567890
		psgsr.DateEnds = 1234567890
		psgsr.IsAutoRenew = true
		psgsr.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(1)
		psgsr.RequestedAnyName = "something.any"

		pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		ps.EXPECT().CacheGet().Return(nil, errors.New("test error")).Once()
		ps.EXPECT().CacheSet(&psgsr, uint16(14400)).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, lifetimeMinutes uint16) (err error) {
			return nil
		}).Once()

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
		resp := getStatus(context.Background(), pp, ps, w, req)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierExplorer), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
		assert.Equal(t, uint64(1234567890), resp.DateStarted)
		assert.Equal(t, uint64(1234567890), resp.DateEnds)
		assert.Equal(t, true, resp.IsAutoRenew)
		assert.Equal(t, pb.RpcPaymentsSubscriptionPaymentMethod(1), resp.PaymentMethod)
		assert.Equal(t, "something.any", resp.RequestedAnyName)
	})
}

func TestGetPaymentURL(t *testing.T) {
	t.Run("fail if BuySubscription method fails", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		pp := mock_ppclient.NewMockAnyPpClientService(c)

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
			RequestedTier:    pb.RpcPaymentsSubscription_TierBuilder1Year,
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
			RequestedTier:    pb.RpcPaymentsSubscription_TierBuilder1Year,
			PaymentMethod:    pb.RpcPaymentsSubscription_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		// Call the function being tested
		resp := getPaymentURL(context.Background(), pp, w, req)
		assert.Equal(t, "https://xxxx.com", resp.PaymentUrl)
	})
}

func TestGetPortalURL(t *testing.T) {
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

func TestGetVerificationEmail(t *testing.T) {
	t.Run("fail if GetVerificationEmail method fails", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		pp.EXPECT().GetVerificationEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetVerificationEmailResponse, error) {
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
		req := &pb.RpcPaymentsSubscriptionGetVerificationEmailRequest{}
		req.Email = "some@mail.com"
		req.SubscribeToNewsletter = true

		// Call the function being tested
		resp := getVerificationEmail(context.Background(), pp, w, req)
		assert.True(t, resp.Error != nil)
	})

	t.Run("success", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		pp.EXPECT().GetVerificationEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetVerificationEmailResponse, error) {
			return &psp.GetVerificationEmailResponse{}, nil
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
		req := &pb.RpcPaymentsSubscriptionGetVerificationEmailRequest{}
		req.Email = "some@mail.com"
		req.SubscribeToNewsletter = true

		// Call the function being tested
		resp := getVerificationEmail(context.Background(), pp, w, req)
		assert.True(t, resp.Error == nil)
	})
}

func TestVerifyEmailCode(t *testing.T) {
	t.Run("fail if VerifyEmail method fails", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		// no errors
		pp.EXPECT().VerifyEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyEmailResponse, error) {
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
		w.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))
		w.EXPECT().Account().Return(&ak)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest{}
		req.Code = "1234"

		// Call the function being tested
		resp := verifyEmailCode(context.Background(), pp, w, req)
		assert.True(t, resp.Error != nil)
	})

	t.Run("success", func(t *testing.T) {
		c := gomock.NewController(t)
		defer c.Finish()

		var pp *mock_ppclient.MockAnyPpClientService
		pp = mock_ppclient.NewMockAnyPpClientService(c)

		// no errors
		pp.EXPECT().VerifyEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyEmailResponse, error) {
			return &psp.VerifyEmailResponse{}, nil
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
		w.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))
		w.EXPECT().Account().Return(&ak)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest{}
		req.Code = "1234"

		// Call the function being tested
		resp := verifyEmailCode(context.Background(), pp, w, req)
		assert.True(t, resp.Error == nil)
	})
}
