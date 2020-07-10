package event

import (
	"sync"

	"github.com/anytypeio/go-anytype-middleware/lib-server"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/google/martian/log"
)

func NewGrpcSender() *GrpcSender {
	return &GrpcSender{ServerMutex: sync.Mutex{}}
}

type GrpcSender struct {
	Server      lib.ClientCommands_ListenEventsServer
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

func (es *GrpcSender) SetServer(server lib.ClientCommands_ListenEventsServer) {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()
	if es.ServerCh != nil {
		close(es.ServerCh)
	}
	es.Server = server
	es.ServerCh = make(chan struct{})
}
