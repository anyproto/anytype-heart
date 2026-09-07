//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package event

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
)

func NewGrpcSender() *GrpcSender {
	gs := &GrpcSender{
		shutdownCh: make(chan *SessionServer),
	}

	go func() {
		for srv := range gs.shutdownCh {
			gs.CloseSessionInstance(srv)
		}
	}()

	return gs
}

type GrpcSender struct {
	ServerMutex sync.RWMutex
	Servers     map[string]*SessionServer

	shutdownCh chan *SessionServer
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
		maxSessionQueueMessages,
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

// waitForSessionPollInterval bounds how stale WaitForSession's answer can be.
// WaitForSession runs at most once per login, so polling costs nothing measurable
// and buys the absence of a waiter registry: no channels to close exactly once,
// no lifecycle coupling to session teardown/supersede, and SetSessionServer — the
// hot path — stays untouched.
const waitForSessionPollInterval = 50 * time.Millisecond

// WaitForSession blocks until token's event stream is registered and reports
// whether it is. A client opens ListenSessionEvents and issues the RPC it
// expects an event from as two independent requests, so the stream is routinely
// not registered yet when the handler runs — and an event broadcast in that
// window is dropped, with nothing to redeliver it. Handlers whose only output is
// a fire-and-forget event call this first. Bounded by timeout and by ctx, so a
// client that never opens a stream just falls through.
func (es *GrpcSender) WaitForSession(ctx context.Context, token string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(waitForSessionPollInterval)
	defer ticker.Stop()
	for {
		if es.IsActive(token) {
			return true
		}
		select {
		case <-ctx.Done():
			return es.IsActive(token) // it may have attached as we gave up
		case <-ticker.C:
		}
	}
}

// scheduleClose tears a session down exactly once. It must not block the caller:
// onClose can fire from sendEvent while Broadcast holds ServerMutex.RLock, and
// CloseSessionInstance (run from the shutdownCh goroutine) needs
// ServerMutex.Lock — so the shutdownCh send happens on its own goroutine to
// avoid that deadlock. It routes the *SessionServer (not the token) so a stale
// auto-close cannot tear down a session that reconnected under the same token.
func (es *GrpcSender) scheduleClose(srv *SessionServer) {
	if srv.closing.CompareAndSwap(false, true) {
		go func() { es.shutdownCh <- srv }()
	}
}

// CloseSessionInstance tears down a specific session, but only if it is still
// the one registered under its token — so an auto-close triggered by an old
// session (send error / overflow) cannot kill a newer one that reconnected with
// the same token. The drain goroutine is always stopped (idempotent).
func (es *GrpcSender) CloseSessionInstance(srv *SessionServer) {
	es.ServerMutex.Lock()
	if cur, ok := es.Servers[srv.Token]; ok && cur == srv {
		delete(es.Servers, srv.Token)
		close(srv.Done)
	}
	es.ServerMutex.Unlock()
	srv.sender.close()
}

// CloseSession tears down whatever session currently holds token. Used by the
// explicit WalletCloseSession path (the caller wants this token closed,
// regardless of identity).
func (es *GrpcSender) CloseSession(token string) {
	es.ServerMutex.Lock()
	s, ok := es.Servers[token]
	if ok {
		delete(es.Servers, token)
		close(s.Done)
	}
	es.ServerMutex.Unlock()
	if ok {
		s.sender.close()
	}
}
