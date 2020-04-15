package core

import (
	"context"
	"errors"
	"sync"
	"time"

	libCore "github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/gateway"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
)

var log = logging.Logger("anytype-mw-api")

var (
	ErrNotLoggedIn = errors.New("not logged in")
)

type Middleware struct {
	rootPath             string
	pin                  string
	mnemonic             string
	gatewayAddr          string
	accountSearchCancel  context.CancelFunc
	localAccounts        []*model.Account
	localAccountCachedAt *time.Time
	SendEvent            func(event *pb.Event)
	blocksService        block.Service
	linkPreview          linkpreview.LinkPreview

	Anytype libCore.Service

	debugGrpcEventSender      chan struct{}
	debugGrpcEventSenderMutex sync.Mutex
	m                         sync.RWMutex
}

func (mw *Middleware) Shutdown(request *pb.RpcShutdownRequest) *pb.RpcShutdownResponse {
	mw.m.Lock()
	defer mw.m.Unlock()
	mw.stop()
	return &pb.RpcShutdownResponse{
		Error: &pb.RpcShutdownResponseError{
			Code: pb.RpcShutdownResponseError_NULL,
		},
	}
}

func (mw *Middleware) getBlockService() (bs block.Service, err error) {
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.blocksService != nil {
		return mw.blocksService, nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) doBlockService(f func(bs block.Service) error) (err error) {
	bs, err := mw.getBlockService()
	if err != nil {
		return
	}
	return f(bs)
}

func (mw *Middleware) setBlockService(bs block.Service) {
	if mw.blocksService != nil {
		mw.blocksService.Close()
	}
	mw.blocksService = bs
}

// Start starts the anytype node and HTTP gateway
func (mw *Middleware) start() error {
	err := mw.Anytype.Start()
	if err != nil {
		return err
	}

	// start the local http gateway
	gateway.Host = &gateway.Gateway{
		Node: mw.Anytype,
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
func (mw *Middleware) stop() error {
	if gateway.Host != nil {
		err := gateway.Host.Stop()
		if err != nil {
			log.Warnf("error while stop gateway: %v", err)
		}
	}

	if mw.blocksService != nil {
		if err := mw.blocksService.Close(); err != nil {
			log.Warnf("error while stop block service: %v", err)
		}
	}

	if mw != nil && mw.Anytype != nil {
		err := mw.Anytype.Stop()
		if err != nil {
			log.Warnf("error while stop anytype: %v", err)
		}

		mw.Anytype = nil
		if mw.accountSearchCancel != nil {
			mw.accountSearchCancel()
		}

		mw.accountSearchCancel = nil
	}
	return nil
}
