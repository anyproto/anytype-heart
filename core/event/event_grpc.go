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
)

var log = logging.Logger("anytype-grpc")

func NewGrpcSender() *GrpcSender {
	return &GrpcSender{}
}

type GrpcSender struct {
	Server      service.ClientCommands_ListenEventsServer
	ServerMutex sync.Mutex
	ServerCh    chan struct{}

	Servers map[string]SessionServer
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
	if es.Server == nil {
		log.Errorf("failed to send event: server not set")
		return
	}

	err := es.Server.Send(pb)
	if err != nil {
		log.Errorf("failed to send event: %s", err.Error())
	}

	for id, s := range es.Servers {
		fmt.Println("send to server", id)
		err := s.Server.Send(pb)
		if err != nil {
			log.Errorf("failed to send event: %s", err.Error())
		}
	}
	return
}

func (es *GrpcSender) SetServer(server service.ClientCommands_ListenEventsServer) {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()
	if es.ServerCh != nil {
		close(es.ServerCh)
	}
	es.Server = server
	es.ServerCh = make(chan struct{})
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
