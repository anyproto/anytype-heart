// +build !nogrpcserver,!_test

package event

import (
	"sync"

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
