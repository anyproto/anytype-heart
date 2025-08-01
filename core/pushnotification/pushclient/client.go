package pushclient

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/rpcerr"
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
	SetToken(ctx context.Context, req *pushapi.SetTokenRequest) (err error)
	RevokeToken(ctx context.Context) (err error)
	SubscribeAll(ctx context.Context, req *pushapi.SubscribeAllRequest) (err error)
	CreateSpace(ctx context.Context, req *pushapi.CreateSpaceRequest) (err error)
	Notify(ctx context.Context, req *pushapi.NotifyRequest) (err error)
	NotifySilent(ctx context.Context, req *pushapi.NotifyRequest) (err error)
	Subscriptions(ctx context.Context, req *pushapi.SubscriptionsRequest) (resp *pushapi.SubscriptionsResponse, err error)
}

type client struct {
	pool        pool.Pool
	peerService peerservice.PeerService
	peerIds     []string
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

func (c *client) SetToken(ctx context.Context, req *pushapi.SetTokenRequest) (err error) {
	err = c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		_, err = c.SetToken(ctx, req)
		if err != nil {
			return fmt.Errorf("set token: %w", rpcerr.Unwrap(err))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (c *client) RevokeToken(ctx context.Context) (err error) {
	err = c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		_, err = c.RevokeToken(ctx, &pushapi.Ok{})
		if err != nil {
			return fmt.Errorf("revoke token: %w", rpcerr.Unwrap(err))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (c *client) SubscribeAll(ctx context.Context, req *pushapi.SubscribeAllRequest) error {
	return c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		_, err := c.SubscribeAll(ctx, req)
		if err != nil {
			return fmt.Errorf("subscribe all: %w", rpcerr.Unwrap(err))
		}
		return nil
	})
}

func (c *client) Subscribe(ctx context.Context, req *pushapi.SubscribeRequest) error {
	return c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		_, err := c.Subscribe(ctx, req)
		if err != nil {
			return fmt.Errorf("subscribe: %w", rpcerr.Unwrap(err))
		}
		return nil
	})
}

func (c *client) Unsubscribe(ctx context.Context, req *pushapi.UnsubscribeRequest) error {
	return c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		_, err := c.Unsubscribe(ctx, req)
		if err != nil {
			return fmt.Errorf("unsubscribe: %w", rpcerr.Unwrap(err))
		}
		return nil
	})
}

func (c *client) CreateSpace(ctx context.Context, req *pushapi.CreateSpaceRequest) error {
	return c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		_, err := c.CreateSpace(ctx, req)
		if err != nil {
			return fmt.Errorf("create space: %w", rpcerr.Unwrap(err))
		}
		return nil
	})
}

func (c *client) Notify(ctx context.Context, req *pushapi.NotifyRequest) error {
	return c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		_, err := c.Notify(ctx, req)
		if err != nil {
			return fmt.Errorf("notify: %w", rpcerr.Unwrap(err))
		}
		return nil
	})
}

func (c *client) NotifySilent(ctx context.Context, req *pushapi.NotifyRequest) error {
	return c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		_, err := c.NotifySilent(ctx, req)
		if err != nil {
			return fmt.Errorf("notify silent: %w", rpcerr.Unwrap(err))
		}
		return nil
	})
}

func (c *client) Subscriptions(ctx context.Context, req *pushapi.SubscriptionsRequest) (resp *pushapi.SubscriptionsResponse, err error) {
	err = c.doClient(ctx, func(c pushapi.DRPCPushClient) error {
		resp, err = c.Subscriptions(ctx, req)
		if err != nil {
			return fmt.Errorf("subscriptions: %w", rpcerr.Unwrap(err))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
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
