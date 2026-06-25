//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package event

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
)

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
	Servers     map[string]*SessionServer

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
	// Non-blocking, order-preserving: the session's single drain goroutine calls
	// Send sequentially (never concurrently on the stream). A slow client that
	// overflows the bounded queue is closed via the sender's onClose.
	server.sender.enqueue(event)
}

func (es *GrpcSender) Broadcast(event *pb.Event) {
	es.ServerMutex.RLock()
	defer es.ServerMutex.RUnlock()
	if len(es.Servers) == 0 {
		log.Warnf("no servers to broadcast event")
	}
	for _, s := range es.Servers {
		es.sendEvent(s, event)
	}
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

type SessionServer struct {
	Token   string
	Done    chan struct{}
	Server  service.ClientCommands_ListenSessionEventsServer
	closing atomic.Bool
	sender  *sessionSender
}

func (es *GrpcSender) SetSessionServer(token string, server service.ClientCommands_ListenSessionEventsServer) *SessionServer {
	es.ServerMutex.Lock()
	if es.Servers == nil {
		es.Servers = map[string]*SessionServer{}
	}
	old := es.Servers[token]
	srv := &SessionServer{
		Token:  token,
		Done:   make(chan struct{}),
		Server: server,
	}
	// One drain goroutine per session calls Send in order; Send is never invoked
	// concurrently on the stream. onClose runs the existing teardown.
	srv.sender = newSessionSender(
		func(e *pb.Event) error { return srv.Server.Send(e) },
		func() { es.scheduleClose(srv) },
		maxSessionQueueLen,
	)
	es.Servers[token] = srv
	es.ServerMutex.Unlock()

	// A reconnect with the same token supersedes the old session (its stream is
	// canceled by gRPC); stop the old drain goroutine so it does not leak.
	if old != nil {
		old.sender.close()
	}
	return srv
}

// scheduleClose tears a session down exactly once. It must not block the caller:
// onClose can fire from sendEvent while Broadcast holds ServerMutex.RLock, and
// CloseSession (run from the shutdownCh goroutine) needs ServerMutex.Lock — so
// the shutdownCh send happens on its own goroutine to avoid that deadlock.
func (es *GrpcSender) scheduleClose(srv *SessionServer) {
	if srv.closing.CompareAndSwap(false, true) {
		go func() { es.shutdownCh <- srv.Token }()
	}
}

func (es *GrpcSender) CloseSession(token string) {
	es.ServerMutex.Lock()
	defer es.ServerMutex.Unlock()

	s, ok := es.Servers[token]
	if ok {
		s.sender.close()
		close(s.Done)
		delete(es.Servers, token)
	}
}
