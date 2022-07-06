//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package event

import (
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pb/service"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/gogo/status"
	"google.golang.org/grpc/codes"
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

func (es *GrpcSender) Send(event *pb.Event) {
	es.broadcast(nil, event)
}

func (es *GrpcSender) SendSession(sessionId string, event *pb.Event) {
	es.broadcast(&sessionId, event)
}

func (es *GrpcSender) broadcast(ignoreSessionId *string, event *pb.Event) {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	for id, s := range es.Servers {
		if ignoreSessionId != nil && *ignoreSessionId == id {
			continue
		}
		id := id
		s := s
		go func() {
			err := s.Server.Send(event)
			if err != nil {
				if s, ok := status.FromError(err); ok && s.Code() == codes.Unavailable {
					es.shutdownCh <- id
				}
				log.Errorf("failed to send event: %s", err.Error())
			}
		}()
	}
}

type SessionServer struct {
	Done   chan struct{}
	Server service.ClientCommands_ListenSessionEventsServer
}

func (es *GrpcSender) SetSessionServer(token string, server service.ClientCommands_ListenEventsServer) SessionServer {
	fmt.Printf("listening %s\n", token)
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()

	if s, ok := es.Servers[token]; ok {
		close(s.Done)
	}

	if es.Servers == nil {
		es.Servers = map[string]SessionServer{}
	}
	srv := SessionServer{
		Done:   make(chan struct{}),
		Server: server,
	}
	es.Servers[token] = srv
	return srv
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
