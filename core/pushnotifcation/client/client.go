package client

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/secureservice"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/anytype/config"
)

func New() Client {
	return &client{}
}

const CName = "core.pushnotification.client"

type Client interface {
	app.Component
	SetToken(ctx context.Context, req *pushapi.SetTokenRequest) (resp *pushapi.Ok, err error)
	SubscribeAll(ctx context.Context, req *pushapi.SubscribeAllRequest) (resp *pushapi.Ok, err error)
	CreateSpace(ctx context.Context, req *pushapi.CreateSpaceRequest) (resp *pushapi.Ok, err error)
}

type client struct {
	pool        pool.Pool
	peerService peerservice.PeerService
	peerIds     []string
}

func NewClient() Client {
	return &client{}
}

func (c *client) SetToken(ctx context.Context, req *pushapi.SetTokenRequest) (resp *pushapi.Ok, err error) {
	err = c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		resp, err = c.SetToken(ctx, req)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *client) SubscribeAll(ctx context.Context, req *pushapi.SubscribeAllRequest) (resp *pushapi.Ok, err error) {
	err = c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		resp, err = c.SubscribeAll(ctx, req)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *client) CreateSpace(ctx context.Context, req *pushapi.CreateSpaceRequest) (resp *pushapi.Ok, err error) {
	err = c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		resp, err = c.CreateSpace(ctx, req)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *client) Init(a *app.App) (err error) {
	c.pool = app.MustComponent[pool.Pool](a)
	c.peerService = app.MustComponent[peerservice.PeerService](a)
	cfg := app.MustComponent[*config.Config](a).GetPushConfig()
	c.peerService.SetPeerAddrs(cfg.PeerId, cfg.Addr)
	c.peerIds = append(c.peerIds, cfg.PeerId)
	return
}

func (c *client) Name() (name string) {
	return CName
}

func (c *client) doClient(ctx context.Context, do func(c pushapi.DRPCPushClient) error) error {
	ctx = secureservice.CtxAllowAccountCheck(ctx)
	peer, err := c.pool.GetOneOf(ctx, c.peerIds)
	if err != nil {
		return err
	}
	return peer.DoDrpc(ctx, func(conn drpc.Conn) error {
		return do(pushapi.NewDRPCPushClient(conn))
	})
}
