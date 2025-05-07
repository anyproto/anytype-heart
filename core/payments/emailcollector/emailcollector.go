package emailcollector

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/dgraph-io/badger/v4"

	ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	proto "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"
)

const CName = "emailcollector"

var log = logging.Logger(CName)

var dbKey = "payments/emailcollector/email"

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
	cfg        *config.Config
	dbProvider datastore.Datastore
	db         *badger.DB
	periodic   periodicsync.PeriodicSync
	ppclient   ppclient.AnyPpClientService
	wallet     wallet.Wallet
	closing    chan struct{}

	// this is in-memory object that was read from the DB
	// if Email field empty - then no need to send it to the pp node
	req pb.RpcMembershipGetVerificationEmailRequest
}

func New() EmailCollector {
	return &emailcollector{}
}

func (e *emailcollector) Name() string {
	return "emailcollector"
}

func (e *emailcollector) Init(a *app.App) error {
	e.closing = make(chan struct{})
	e.cfg = app.MustComponent[*config.Config](a)
	e.dbProvider = app.MustComponent[datastore.Datastore](a)
	e.ppclient = app.MustComponent[ppclient.AnyPpClientService](a)
	e.wallet = app.MustComponent[wallet.Wallet](a)

	db, err := e.dbProvider.LocalStorage()
	if err != nil {
		return err
	}
	e.db = db

	// run periodic cycle to send email to the payment service
	e.periodic = periodicsync.NewPeriodicSync(refreshIntervalSecs, timeout, e.periodicUpdateEmail, logger.CtxLogger{Logger: log.Desugar()})

	// read: db -> req field
	err = e.get()
	if err != nil {
		log.Error("emailcollector: failed to get email", zap.Error(err))
		// not an error, just no email in the DB
		return nil
	}

	return nil
}

func (e *emailcollector) Run(ctx context.Context) (err error) {
	e.periodic.Run()
	return nil
}

func (e *emailcollector) Close(_ context.Context) (err error) {
	close(e.closing)

	e.periodic.Close()
	return nil
}

// Call "SetRequest" by any external component to save email to the DB
// and to the "email" field in-memory.
// Once email is set - this component will send it to the payment service
// when it will be online
func (e *emailcollector) SetRequest(req *pb.RpcMembershipGetVerificationEmailRequest) error {
	e.req = *req

	log.Debug("emailcollector: setting email", zap.String("email", req.Email))

	err := e.set(req)
	if err != nil {
		log.Error("emailcollector: failed to set email", zap.Error(err))
		return err
	}

	return nil
	// try to send it right now
	//return e.periodicUpdateEmail(context.Background())
}

func (e *emailcollector) periodicUpdateEmail(ctx context.Context) error {
	// skip running loop if we are on a custom network or in local-only mode
	if e.cfg.GetNetworkMode() != pb.RpcAccount_DefaultConfig {
		// do not trace to log to prevent spamming
		return nil
	}

	// 1 - check if we have something to send
	if e.req.Email == "" {
		// skip if email is not set or was already sent
		log.Debug("emailcollector: email is not set or was already sent")
		return nil
	}

	// send to pp node (do not check response)
	// this is the default request without SubscribeToNewsletter, InsiderTipsAndTutorials fields set
	log.Debug("emailcollector: sending email to pp node", zap.String("email", e.req.Email))
	_, err := e.SendRequest(
		ctx,
		&e.req,
	)
	if err != nil {
		log.Debug("emailcollector: failed to send email to pp node", zap.Error(err))
		return err
	}

	// save to db empty string (email sent)
	e.req.Email = ""

	err = e.set(&e.req)
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
func (e *emailcollector) get() (err error) {
	if e.db == nil {
		return errors.New("db not initialized")
	}

	err = e.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(dbKey))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			// convert value to out
			return json.Unmarshal(val, &e.req)
		})
	})

	return err
}

func (e *emailcollector) set(in *pb.RpcMembershipGetVerificationEmailRequest) (err error) {
	if e.db == nil {
		return errors.New("db not initialized")
	}

	// save to db
	return e.db.Update(func(txn *badger.Txn) error {
		// convert
		bytes, err := json.Marshal(in)
		if err != nil {
			return err
		}

		return txn.Set([]byte(dbKey), bytes)
	})
}
