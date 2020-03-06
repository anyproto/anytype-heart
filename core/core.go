package core

import (
	"context"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/gateway"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("anytype-mw")

type MiddlewareState struct {
	// client-state: blocks range, text range, focus, screen position, etc
	// history list
	// request list
	// computed state
}

type Middleware struct {
	state                MiddlewareState
	rootPath             string
	pin                  string
	mnemonic             string
	gatewayAddr          string
	accountSearchCancel  context.CancelFunc
	localAccounts        []*model.Account
	localAccountCachedAt *time.Time
	SendEvent            func(event *pb.Event)
	blockService         block.Service
	linkPreview          linkpreview.LinkPreview

	Anytype core.Service

	debugGrpcEventSender      chan struct{}
	debugGrpcEventSenderMutex sync.Mutex
}

// Start starts the anytype node and HTTP gateway
func (mw *Middleware) Start() error {
	err := mw.Anytype.Start()
	if err != nil {
		return err
	}

	// start the local http gateway
	gateway.Host = &gateway.Gateway{
		Node: mw.Anytype.(*core.Anytype),
	}

	err = gateway.Host.Start(gateway.GatewayAddr())
	if err != nil {
		return err
	}

	mw.gatewayAddr = "http://" + gateway.GatewayAddr()
	log.Debug("Gateway started: " + mw.gatewayAddr)

	mw.linkPreview = linkpreview.NewWithCache()
	return nil
}

// Stop stops the anytype node and HTTP gateway
func (mw *Middleware) Stop() error {
	if gateway.Host != nil {
		err := gateway.Host.Stop()
		if err != nil {
			return err
		}
	}

	if mw != nil && mw.Anytype != nil {
		err := mw.Anytype.Stop()
		if err != nil {
			return err
		}

		mw.Anytype = nil
		if mw.accountSearchCancel != nil {
			mw.accountSearchCancel()
		}

		mw.accountSearchCancel = nil
	}

	return nil
}
