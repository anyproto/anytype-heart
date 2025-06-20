package emailcollector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	proto "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"
	"github.com/anyproto/any-sync/util/periodicsync"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

const CName = "emailcollector"

var log = logging.Logger(CName)

const (
	refreshIntervalSecs = 60
	timeout             = 30 * time.Second
)

// EmailCollector is a simple component that will save email to the DB
// even if we are offline. And then send it to the payment service
// when we are online.
type EmailCollector interface {
	SetRequest(req *pb.RpcMembershipGetVerificationEmailRequest) error
	SendRequest(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailRequest) (*pb.RpcMembershipGetVerificationEmailResponse, error)

	app.ComponentRunnable
}

type emailcollector struct {
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	cfg      *config.Config
	store    keyvaluestore.Store[pb.RpcMembershipGetVerificationEmailRequest]
	periodic periodicsync.PeriodicSync
	ppclient ppclient.AnyPpClientService
	wallet   wallet.Wallet
}

func New() EmailCollector {
	return &emailcollector{}
}

func (e *emailcollector) Name() string {
	return CName
}

func (e *emailcollector) Init(a *app.App) error {
	e.componentCtx, e.componentCtxCancel = context.WithCancel(context.Background())

	e.cfg = app.MustComponent[*config.Config](a)
	e.ppclient = app.MustComponent[ppclient.AnyPpClientService](a)
	e.wallet = app.MustComponent[wallet.Wallet](a)

	// Initialize keyvaluestore
	anystoreProvider := app.MustComponent[anystoreprovider.Provider](a)
	store := anystoreProvider.GetCommonDb()
	var err error
	e.store, err = keyvaluestore.NewJson[pb.RpcMembershipGetVerificationEmailRequest](store, "payments/emailcollector/email")
	if err != nil {
		return fmt.Errorf("init request store: %w", err)
	}

	// run periodic cycle to send email to the payment service
	e.periodic = periodicsync.NewPeriodicSync(refreshIntervalSecs, timeout, e.periodicUpdateEmail, logger.CtxLogger{Logger: log.Desugar()})
	return nil
}

func (e *emailcollector) Run(ctx context.Context) (err error) {
	// skip running loop if we are on a custom network or in local-only mode
	if e.cfg.GetNetworkMode() != pb.RpcAccount_DefaultConfig {
		// do not trace to log to prevent spamming
		return nil
	}

	e.periodic.Run()
	return nil
}

func (e *emailcollector) Close(_ context.Context) (err error) {
	if e.componentCtxCancel != nil {
		e.componentCtxCancel()
	}
	e.periodic.Close()
	return nil
}

// Call "SetRequest" by any external component to save email to the DB
// and to the "email" field in-memory.
// Once email is set - this component will send it to the payment service
// when it will be online
func (e *emailcollector) SetRequest(req *pb.RpcMembershipGetVerificationEmailRequest) error {
	log.Debug("emailcollector: setting email")

	err := e.set(req)
	if err != nil {
		log.Error("emailcollector: failed to set email", zap.Error(err))
		return err
	}

	return nil
}

func (e *emailcollector) periodicUpdateEmail(ctx context.Context) error {
	// skip running loop if we are on a custom network or in local-only mode
	if e.cfg.GetNetworkMode() != pb.RpcAccount_DefaultConfig {
		// do not trace to log to prevent spamming
		return nil
	}

	req, err := e.get()
	if err != nil {
		// skip it if we have no email to send
		return nil
	}

	// 1 - check if we have something to send
	if req.Email == "" {
		// skip if email is not set or was already sent
		log.Debug("emailcollector: email is not set or was already sent")
		return nil
	}

	// send to pp node (do not check response)
	// this is the default request without SubscribeToNewsletter, InsiderTipsAndTutorials fields set
	log.Debug("emailcollector: sending email to pp node")
	_, err = e.SendRequest(
		ctx,
		&req,
	)
	if err != nil {
		log.Debug("emailcollector: failed to send email to pp node", zap.Error(err))
		return err
	}

	// save to db empty string (email sent)
	req.Email = ""

	err = e.set(&req)
	if err != nil {
		log.Error("emailcollector: failed to set email", zap.Error(err))
		return err
	}

	return nil
}

func (e *emailcollector) SendRequest(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailRequest) (*pb.RpcMembershipGetVerificationEmailResponse, error) {
	// 1 - send request
	bsr := proto.GetVerificationEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:              e.wallet.Account().SignKey.GetPublic().Account(),
		Email:                   req.Email,
		SubscribeToNewsletter:   req.SubscribeToNewsletter,
		InsiderTipsAndTutorials: req.InsiderTipsAndTutorials,
		IsOnboardingList:        req.IsOnboardingList,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		log.Error("can not marshal GetVerificationEmailRequest", zap.Error(err))
		return nil, errors.New("can not marshal GetVerificationEmailRequest")
	}

	privKey := e.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign GetVerificationEmailRequest", zap.Error(err))
		return nil, errors.New("can not sign GetVerificationEmailRequest")
	}

	reqSigned := proto.GetVerificationEmailRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	_, err = e.ppclient.GetVerificationEmail(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	var out pb.RpcMembershipGetVerificationEmailResponse
	out.Error = &pb.RpcMembershipGetVerificationEmailResponseError{
		Code: pb.RpcMembershipGetVerificationEmailResponseError_NULL,
	}

	return &out, nil
}

// will save data to e.email...
func (e *emailcollector) get() (pb.RpcMembershipGetVerificationEmailRequest, error) {
	if e.store == nil {
		return pb.RpcMembershipGetVerificationEmailRequest{}, errors.New("store not initialized")
	}

	req, err := e.store.Get(e.componentCtx, "req")
	if err != nil {
		return pb.RpcMembershipGetVerificationEmailRequest{}, err
	}
	return req, nil
}

func (e *emailcollector) set(req *pb.RpcMembershipGetVerificationEmailRequest) error {
	if e.store == nil {
		return errors.New("store not initialized")
	}

	return e.store.Set(e.componentCtx, "req", *req)
}
