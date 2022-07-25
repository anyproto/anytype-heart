package cafe

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	walletUtil "github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/mr-tron/base58"
	"github.com/textileio/go-threads/core/thread"
	"google.golang.org/grpc"
)

var _ pb.APIClient = (*Online)(nil)
var log = logging.Logger("anytype-cafe-client")

const (
	CName                = "cafeclient"
	simultaneousRequests = 4
)

type Client interface {
	app.Component
	pb.APIClient
	GetConnState() (connected, conenctedBefore bool, lastChange time.Time)
}

type Token struct {
	Token     string
	ExpiresAt time.Time
}

type Online struct {
	client        pb.APIClient
	token         *Token
	getTokenMutex sync.Mutex

	limiter chan struct{}

	device      walletUtil.Keypair
	account     walletUtil.Keypair
	apiInsecure bool
	grpcAddress string

	conn *grpc.ClientConn

	connLastStateChange time.Time
	connectedOnce       bool
	connected           bool

	connMutex sync.Mutex
}

func (c *Online) Init(a *app.App) (err error) {
	wl := a.MustComponent(wallet.CName).(wallet.Wallet)
	cfg := a.MustComponent(config.CName).(*config.Config)

	c.grpcAddress = cfg.CafeNodeGrpcAddr()
	c.apiInsecure = cfg.CafeAPIInsecure

	c.device, err = wl.GetDevicePrivkey()
	if err != nil {
		return err
	}
	c.account, err = wl.GetAccountPrivkey()
	if err != nil {
		return err
	}

	return nil
}

func (c *Online) Name() (name string) {
	return CName
}

func (c *Online) getSignature(payload string) (*pb.WithSignature, error) {
	as, err := c.account.Sign([]byte(payload))
	if err != nil {
		return nil, fmt.Errorf("can't create account signature")
	}

	asB58 := base58.Encode(as)
	ds, err := c.device.Sign([]byte(payload + asB58))
	if err != nil {
		return nil, fmt.Errorf("can't create device signature")
	}

	return &pb.WithSignature{
		AccountAddress:   c.account.Address(),
		DeviceAddress:    c.device.Address(),
		AccountSignature: asB58,
		DeviceSignature:  base58.Encode(ds),
	}, nil
}

func (c *Online) GetConnState() (connected bool, wasConnectedBefore bool, lastChange time.Time) {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	return c.connected, c.connectedOnce, c.connLastStateChange
}

func (c *Online) Run(ctx context.Context) error {
	// todo: get version from component
	var version string
	opts := []grpc.DialOption{grpc.WithUserAgent(version), grpc.WithPerRPCCredentials(thread.Credentials{})}

	if c.apiInsecure {
		opts = append(opts, grpc.WithInsecure())
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	}

	conn, err := grpc.Dial(c.grpcAddress, opts...)
	if err != nil {
		return err
	}

	c.client = pb.NewAPIClient(conn)
	c.conn = conn

	return nil
}

func (c *Online) healthCheckMetric() {
	go func() {
		state := connectivity.Idle
		for {
			if !c.conn.WaitForStateChange(context.Background(), state) {
				return
			}
			state2 := c.conn.GetState()
			if state2 != connectivity.Ready && state2 != connectivity.TransientFailure {
				state = state2
				continue
			}
			var after time.Duration
			c.connMutex.Lock()
			if !c.connLastStateChange.IsZero() {
				after = time.Since(c.connLastStateChange)
			}
			c.connLastStateChange = time.Now()
			c.connected = state2 == connectivity.Ready
			if c.connected {
				c.connectedOnce = true
			}
			c.connMutex.Unlock()
			if state2 == connectivity.Ready {
				log.With("after", after).Debug("cafe grpc got connected")
			} else if state2 == connectivity.TransientFailure {
				if c.connectedOnce {
					log.With("after", after).Warn("cafe grpc got disconnected")
				} else {
					log.With("after", after).Warn("cafe grpc not able to connect for the first time")
				}
			}

			event := metrics.CafeGrpcConnectStateChanged{
				AfterMs:         after.Milliseconds(),
				Connected:       state2 == connectivity.Ready,
				ConnectedBefore: c.connectedOnce,
			}
			metrics.SharedClient.RecordEvent(event)
			state = state2
		}
	}()
}

func (c *Online) withToken(ctx context.Context) (context.Context, error) {
	token, err := c.requestToken(ctx)
	if err != nil {
		return nil, err
	}

	ctx = thread.NewTokenContext(ctx, thread.Token(token.Token))
	return ctx, nil
}

func (c *Online) requestToken(ctx context.Context) (*Token, error) {
	c.getTokenMutex.Lock()
	defer c.getTokenMutex.Unlock()
	if c.token != nil && c.token.ExpiresAt.After(time.Now().Add(time.Second*30)) {
		return c.token, nil
	}

	server, err := c.client.AuthGetToken(ctx)
	if err != nil {
		return nil, err
	}

	sig, err := c.getSignature("")
	if err != nil {
		return nil, err
	}

	err = server.Send(&pb.AuthGetTokenRequest{Signature: sig})
	if err != nil {
		return nil, fmt.Errorf("failed to send auth code request: %w", err)
	}

	resp, err := server.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth code %w", err)
	}

	authCode := resp.GetAuthCode()
	sig, err = c.getSignature(authCode)
	if err != nil {
		return nil, err
	}

	err = server.Send(&pb.AuthGetTokenRequest{AuthCode: authCode, Signature: sig})
	if err != nil {
		return nil, fmt.Errorf("failed to send auth code request: %w", err)
	}

	resp, err = server.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to get token %w", err)
	}

	if resp.GetToken() == nil {
		return nil, fmt.Errorf("failed to get token: token is nil")
	}

	expiresAt := time.Unix(resp.GetToken().ExpiresAt, 0)
	c.token = &Token{Token: resp.GetToken().Token, ExpiresAt: expiresAt}

	return c.token, nil
}

func (c *Online) AuthGetToken(ctx context.Context, opts ...grpc.CallOption) (pb.API_AuthGetTokenClient, error) {
	return c.client.AuthGetToken(ctx, opts...)
}

func (c *Online) ThreadLogFollow(ctx context.Context, in *pb.ThreadLogFollowRequest, opts ...grpc.CallOption) (*pb.ThreadLogFollowResponse, error) {
	ctx, err := c.withToken(ctx)
	if err != nil {
		return nil, err
	}
	return c.client.ThreadLogFollow(ctx, in, opts...)
}

func (c *Online) GetFilePins(ctx context.Context, in *pb.GetFilePinsRequest, opts ...grpc.CallOption) (*pb.GetFilePinsResponse, error) {
	ctx, err := c.withToken(ctx)
	if err != nil {
		return nil, err
	}

	return c.client.GetFilePins(ctx, in, opts...)
}

func (c *Online) GetAccountState(ctx context.Context, in *pb.GetAccountStateRequest, opts ...grpc.CallOption) (*pb.GetAccountStateResponse, error) {
	ctx, err := c.withToken(ctx)
	if err != nil {
		return nil, err
	}

	return c.client.GetAccountState(ctx, in, opts...)
}

func (c *Online) FilePin(ctx context.Context, in *pb.FilePinRequest, opts ...grpc.CallOption) (*pb.FilePinResponse, error) {
	<-c.limiter
	defer func() { c.limiter <- struct{}{} }()

	ctx, err := c.withToken(ctx)
	if err != nil {
		return nil, err
	}

	return c.client.FilePin(ctx, in, opts...)
}

func (c *Online) ProfileFind(ctx context.Context, in *pb.ProfileFindRequest, opts ...grpc.CallOption) (pb.API_ProfileFindClient, error) {
	ctx, err := c.withToken(ctx)
	if err != nil {
		return nil, err
	}

	return c.client.ProfileFind(ctx, in, opts...)
}

func (c *Online) GetConfig(ctx context.Context, in *pb.GetConfigRequest, opts ...grpc.CallOption) (*pb.GetConfigResponse, error) {
	ctx, err := c.withToken(ctx)
	if err != nil {
		return nil, err
	}

	return c.client.GetConfig(ctx, in, opts...)
}

func (c *Online) AccountDelete(ctx context.Context, in *pb.AccountDeleteRequest, opts ...grpc.CallOption) (*pb.AccountDeleteResponse, error) {
	ctx, err := c.withToken(ctx)
	if err != nil {
		return nil, err
	}

	return c.client.AccountDelete(ctx, in, opts...)
}

func New() Client {
	limiter := make(chan struct{}, simultaneousRequests)

	for i := 0; i < cap(limiter); i++ {
		limiter <- struct{}{}
	}

	return &Online{
		limiter:       limiter,
		getTokenMutex: sync.Mutex{},
	}
}

func (c *Online) Close() error {
	return c.conn.Close()
}
