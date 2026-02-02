//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package event

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

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
		callbacks:  make(map[string]func(*pb.Event)),
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
	Servers     map[string]*SessionServer

	callbackMu sync.RWMutex
	callbacks  map[string]func(*pb.Event)

	shutdownCh chan string
}

func (es *GrpcSender) Init(_ *app.App) (err error) {
	return
}

func (es *GrpcSender) Name() (name string) {
	return CName
}

func (es *GrpcSender) IsActive(token string) bool {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	_, ok := es.Servers[token]
	return ok
}

func (es *GrpcSender) SendToSession(token string, event *pb.Event) {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	if s, ok := es.Servers[token]; ok {
		es.sendEvent(s, event)
	}
}

func (es *GrpcSender) sendEvent(server *SessionServer, event *pb.Event) {
	if len(event.Messages) == 0 {
		return
	}
	go func() {
		err := server.Server.Send(event)
		if err != nil {
			if s, ok := status.FromError(err); ok && s.Code() == codes.Unavailable {
				if server.closing.CompareAndSwap(false, true) {
					es.shutdownCh <- server.Token
				}
			}
		}
	}()
}

func (es *GrpcSender) Broadcast(event *pb.Event) {
	es.ServerMutex.RLock()
	if len(es.Servers) == 0 {
		log.Warnf("no servers to broadcast event")
	}
	for _, s := range es.Servers {
		es.sendEvent(s, event)
	}
	es.ServerMutex.RUnlock()

	// Also call registered callbacks
	es.callbackMu.RLock()
	for _, cb := range es.callbacks {
		go cb(event)
	}
	es.callbackMu.RUnlock()
}

// BroadcastToOtherSessions broadcasts the event from current session. Do not broadcast to the current session
func (es *GrpcSender) BroadcastToOtherSessions(token string, event *pb.Event) {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	for _, s := range es.Servers {
		if s.Token != token {
			es.sendEvent(s, event)
		}
	}
}

// BroadcastExceptSessions broadcasts the event to session except provided
func (es *GrpcSender) BroadcastExceptSessions(event *pb.Event, exceptTokens []string) {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()

	for _, s := range es.Servers {
		if !slices.Contains(exceptTokens, s.Token) {
			es.sendEvent(s, event)
		}
	}
}

// RegisterCallback registers a callback to receive all broadcast events.
// Returns a function to unregister the callback.
func (es *GrpcSender) RegisterCallback(callback func(*pb.Event)) func() {
	es.callbackMu.Lock()
	defer es.callbackMu.Unlock()

	// Generate a unique ID for this callback
	id := generateCallbackId()
	es.callbacks[id] = callback

	return func() {
		es.callbackMu.Lock()
		defer es.callbackMu.Unlock()
		delete(es.callbacks, id)
	}
}

var callbackIdCounter uint64

func generateCallbackId() string {
	return fmt.Sprintf("cb_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&callbackIdCounter, 1))
}

type SessionServer struct {
	Token   string
	Done    chan struct{}
	Server  service.ClientCommands_ListenSessionEventsServer
	closing atomic.Bool
}

func (es *GrpcSender) SetSessionServer(token string, server service.ClientCommands_ListenSessionEventsServer) *SessionServer {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()
	if es.Servers == nil {
		es.Servers = map[string]*SessionServer{}
	}
	srv := &SessionServer{
		Token:  token,
		Done:   make(chan struct{}),
		Server: server,
	}

	// Old connection with this token will be cancelled automatically
	es.Servers[token] = srv
	return srv
}

func (es *GrpcSender) CloseSession(token string) {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()

	s, ok := es.Servers[token]
	if ok {
		close(s.Done)
		delete(es.Servers, token)
	}
}
