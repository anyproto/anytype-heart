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
	var sender *event.GrpcSender
	var ok bool
	if sender, ok = mw.EventSender.(*event.GrpcSender); ok {
		sender.SetServer(server)
	} else {
		log.Fatal("failed to ListenEvents: has a wrong Sender")
		return
	}

	sender.ServerMutex.Lock()
	serverCh := sender.ServerCh
	sender.ServerMutex.Unlock()

	var stopChan = make(chan os.Signal, 2)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-stopChan:
		return
	case <-serverCh:
		return
	}
}
