package clientserver

import (
	"context"

	"github.com/anyproto/any-sync/app"
)

const CName = "client.space.clientserver"

type ClientServer interface {
	app.ComponentRunnable
	Port() int
	ServerStarted() bool
}

type clientServer struct{}

func New() ClientServer { return &clientServer{} }

func (s *clientServer) Init(*app.App) error { return nil }
func (s *clientServer) Name() string { return CName }
func (s *clientServer) Run(context.Context) error { return nil }
func (s *clientServer) Close(context.Context) error { return nil }
func (s *clientServer) Port() int { return 0 }
func (s *clientServer) ServerStarted() bool { return false }
