// +build debug,!_test

package core

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/anytypeio/go-anytype-middleware/lib-debug"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) ListenEvents(_ *pb.Empty, server lib.ClientCommands_ListenEventsServer) {
	mw.debugGrpcEventSenderMutex.Lock()
	if mw.debugGrpcEventSender != nil {
		close(mw.debugGrpcEventSender)
	}
	mw.debugGrpcEventSender = make(chan struct{})
	mw.debugGrpcEventSenderMutex.Unlock()

	mw.SendEvent = func(event *pb.Event) {
		server.Send(event)
	}
	var stopChan = make(chan os.Signal, 2)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	select {
	case <-stopChan:
		return
	case <-mw.debugGrpcEventSender:
		return
	}
}
