//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package core

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	lib "github.com/anytypeio/go-anytype-middleware/pb/service"
)

func (mw *Middleware) ListenEvents(_ *pb.Empty, server lib.ClientCommands_ListenEventsServer) {
}

func (mw *Middleware) ListenSessionEvents(req *pb.StreamRequest, server lib.ClientCommands_ListenSessionEventsServer) {
	// TODO: check token here
	if req.Token == "" {
		log.Error("ListenSessionEvents: empty token")
		return
	}
	var srv event.SessionServer
	if sender, ok := mw.EventSender.(*event.GrpcSender); ok {
		srv = sender.SetSessionServer(req.Token, server)
	} else {
		log.Fatal("failed to ListenEvents: has a wrong Sender")
		return
	}

	var stopChan = make(chan os.Signal, 2)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-stopChan:
		log.Errorf("stream %s interrupted", req.Token)
		return
	case <-srv.Done:
		log.Errorf("stream %s closed", req.Token)
		return
	}
}
