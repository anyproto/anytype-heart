//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package core

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	lib "github.com/anyproto/anytype-heart/pb/service"
)

func (mw *Middleware) ListenSessionEvents(req *pb.StreamRequest, server lib.ClientCommands_ListenSessionEventsServer) {
	if err := mw.applicationService.ValidateSessionToken(req.Token); err != nil {
		log.Errorf("ListenSessionEvents: %s", err)
		return
	}

	var srv event.SessionServer
	if sender, ok := mw.applicationService.GetEventSender().(*event.GrpcSender); ok {
		srv = sender.SetSessionServer(req.Token, server)
	} else {
		log.Fatal("failed to ListenEvents: has a wrong Sender")
		return
	}
	if mw.GetApp() != nil {
		notifier := app.MustComponent[session.NewSessionNotifier](mw.GetApp())
		notifier.Notify(session.NewContext(session.WithSession(req.Token)))
	}
	var stopChan = make(chan os.Signal, 2)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-stopChan:
		log.Errorf("stream %s interrupted", req.Token)
	case <-srv.Done:
		log.Errorf("stream %s closed", req.Token)
	case <-srv.Server.Context().Done():
		log.Errorf("stream %s context canceled", req.Token)
	}
}
