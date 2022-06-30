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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var log = logging.Logger("anytype-grpc")

func NewGrpcSender() *GrpcSender {
	return &GrpcSender{}
}

type GrpcSender struct {
	ServerMutex sync.Mutex
	Servers     map[string]SessionServer
}

func (es *GrpcSender) Init(_ *app.App) (err error) {
	return
}

func (es *GrpcSender) Name() (name string) {
	return CName
}

func (es *GrpcSender) Send(pb *pb.Event) {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()

	var toClose []string
	for id, s := range es.Servers {
		fmt.Println("send to server", id)
		err := s.Server.Send(pb)
		if err != nil {
			if s, ok := status.FromError(err); ok && s.Code() == codes.Unavailable {
				toClose = append(toClose, id)
			}
			log.Errorf("failed to send event: %s", err.Error())
		}
	}

	for _, id := range toClose {
		log.Errorf("close %s", id)
		s := es.Servers[id]
		close(s.Done)
		delete(es.Servers, id)
	}
	return
}

func (es *GrpcSender) SendSession(sessionId string, pb *pb.Event) {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()

	for id, s := range es.Servers {
		if id == sessionId {
			continue
		}
		fmt.Println("send to server session", id)
		err := s.Server.Send(pb)
		if err != nil {
			log.Errorf("failed to send event: %s", err.Error())
		}
	}
	return
}

type SessionServer struct {
	Done   chan struct{}
	Server service.ClientCommands_ListenSessionEventsServer
}

func (es *GrpcSender) SetSessionServer(token string, server service.ClientCommands_ListenEventsServer) SessionServer {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()

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
