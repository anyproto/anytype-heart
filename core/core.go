package core

import (
	"context"
	"errors"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pb"
	libCore "github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/gateway"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
)

var log = logging.Logger("anytype-mw-api")

var (
	ErrNotLoggedIn = errors.New("not logged in")
)

type Middleware struct {
	rootPath            string
	pin                 string
	mnemonic            string
	gatewayAddr         string
	accountSearchCancel context.CancelFunc

	foundAccounts []*model.Account // found local&remote account for the current mnemonic

	EventSender event.Sender

	blocksService block.Service
	linkPreview   linkpreview.LinkPreview
	status        status.Service
	indexer       indexer.Indexer

	Anytype libCore.Service

	m sync.RWMutex
}

func New() *Middleware {
	return &Middleware{accountSearchCancel: func() {}}
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

func (mw *Middleware) setIndexer(is indexer.Indexer) {
	if mw.indexer != nil {
		mw.indexer.Close()
	}
	mw.indexer = is
}

func (mw *Middleware) setBlockService(bs block.Service) {
	if mw.blocksService != nil {
		mw.blocksService.Close()
	}
	mw.blocksService = bs
}

func (mw *Middleware) setStatusService(ss status.Service) {
	mw.status = ss
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

	gwAddr := gateway.GatewayAddr()
	mw.gatewayAddr = "http://" + gwAddr
	err = gateway.Host.Start(gwAddr)
	if err != nil {
		return err
	}

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

	if mw.status != nil {
		mw.status.Stop()
	}

	if mw.indexer != nil {
		mw.indexer.Close()
	}

	if mw != nil && mw.Anytype != nil {
		err := mw.Anytype.Stop()
		if err != nil {
			log.Warnf("error while stop anytype: %v", err)
		}

		mw.Anytype = nil
		mw.accountSearchCancel()
	}
	return nil
}

func (mw *Middleware) reindexDoc(id string) error {
	bs, err := mw.getBlockService()
	if err != nil {
		return err
	}
	return bs.Reindex(id)
}

func init() {
	logging.SetVersion(GitSummary)
}
