//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package event

import (
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/status"
	"google.golang.org/grpc/codes"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-grpc")

func NewGrpcSender() *GrpcSender {
	gs := &GrpcSender{
		shutdownCh: make(chan string),
	}

	go func() {
		for id := range gs.shutdownCh {
			gs.CloseSession(id)
		}
	}()

	return gs
}

type GrpcSender struct {
	ServerMutex sync.RWMutex
	Servers     map[string]SessionServer

	shutdownCh chan string
}

func (es *GrpcSender) Init(_ *app.App) (err error) {
	return
}

func (es *GrpcSender) Name() (name string) {
	return CName
}

func (es *GrpcSender) IsActive(spaceID string, token string) bool {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	s, ok := es.Servers[token]
	return ok && s.SpaceID == spaceID
}

func (es *GrpcSender) SendToSession(spaceID string, token string, event *pb.Event) {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	if s, ok := es.Servers[token]; ok && s.SpaceID == spaceID {
		es.sendEvent(s, event)
	}
}

func (es *GrpcSender) sendEvent(server SessionServer, event *pb.Event) {
	go func() {
		err := server.Server.Send(event)
		if err != nil {
			if s, ok := status.FromError(err); ok && s.Code() == codes.Unavailable {
				es.shutdownCh <- server.Token
			}
			log.With("session", server.Token, "spaceID", server.SpaceID).Errorf("failed to send event: %s", err)
		}
	}()
}

func (es *GrpcSender) Broadcast(event *pb.Event) {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	for _, s := range es.Servers {
		es.sendEvent(s, event)
	}
}

func (es *GrpcSender) BroadcastForSpace(spaceID string, event *pb.Event) {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	for _, s := range es.Servers {
		if s.SpaceID == spaceID {
			es.sendEvent(s, event)
		}
	}
}

// BroadcastToOtherSessions broadcasts the event from current session. Do not broadcast to the current session
func (es *GrpcSender) BroadcastToOtherSessions(spaceID string, token string, event *pb.Event) {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	for _, s := range es.Servers {
		if s.Token != token && s.SpaceID == spaceID {
			es.sendEvent(s, event)
		}
	}
}

type SessionServer struct {
	Token   string
	SpaceID string
	Done    chan struct{}
	Server  service.ClientCommands_ListenSessionEventsServer
}

func (es *GrpcSender) SetSessionServer(spaceID string, token string, server service.ClientCommands_ListenSessionEventsServer) SessionServer {
	log.Warnf("listening %s\n", token)
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()
	if es.Servers == nil {
		es.Servers = map[string]SessionServer{}
	}
	srv := SessionServer{
		Token:   token,
		Done:    make(chan struct{}),
		Server:  server,
		SpaceID: spaceID,
	}

	// Old connection with this token will be cancelled automatically
	es.Servers[token] = srv
	return srv
}

func (es *GrpcSender) SetSpaceID(token string, spaceID string) error {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()

	s, ok := es.Servers[token]
	if !ok {
		return fmt.Errorf("unknown session %s", token)
	}
	s.SpaceID = spaceID
	es.Servers[token] = s
	return nil
}

func (es *GrpcSender) CloseSession(token string) {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()

	s, ok := es.Servers[token]
	if ok {
		log.Errorf("method close session %s", token)
		close(s.Done)
		delete(es.Servers, token)
	}
}
