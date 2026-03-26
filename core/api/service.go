package api

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/server"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/vectorsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	CName           = "api"
	readTimeout     = 5 * time.Second
	shutdownTimeout = time.Millisecond
)

var (
	log = logging.Logger("api-service")

	mwSrv apicore.ClientCommands

	//go:embed docs/openapi.yaml
	openapiYAML []byte

	//go:embed docs/openapi.json
	openapiJSON []byte
)

type Service interface {
	app.ComponentRunnable
	ReassignAddress(ctx context.Context, listenAddr string) error
}

type apiService struct {
	mw                   apicore.ClientCommands
	accountService       apicore.AccountService
	eventService         apicore.EventService
	crossSpaceSubService apicore.CrossSpaceSubscriptionService
	vectorSearch         vectorsearch.VectorSearch

	listenAddr string

	srv     *server.Server
	httpSrv *http.Server

	lock sync.Mutex
}

func New() Service {
	return &apiService{mw: mwSrv}
}

func (s *apiService) Name() (name string) {
	return CName
}

// Init initializes the API service.
//
//	@title							Anytype API
//	@version						2025-11-08
//	@description					This API enables seamless interaction with Anytype's resources - spaces, objects, properties, types, templates, and beyond.
//	@termsOfService					https://anytype.io/terms_of_use
//	@contact.name					Anytype Support
//	@contact.url					https://anytype.io/contact
//	@contact.email					support@anytype.io
//	@license.name					Any Source Available License 1.0
//	@license.url					https://github.com/anyproto/anytype-api/blob/main/LICENSE.md
//	@host							http://127.0.0.1:31009
//	@securitydefinitions.bearerauth	BearerAuth
//	@externalDocs.description		OpenAPI
//	@externalDocs.url				https://swagger.io/resources/open-api/
func (s *apiService) Init(a *app.App) error {
	s.listenAddr = a.MustComponent(config.CName).(*config.Config).JsonApiListenAddr
	s.accountService = a.MustComponent(account.CName).(account.Service)
	s.eventService = a.MustComponent(event.CName).(apicore.EventService)
	s.crossSpaceSubService = a.MustComponent(crossspacesub.CName).(apicore.CrossSpaceSubscriptionService)
	s.vectorSearch = app.MustComponent[vectorsearch.VectorSearch](a)
	return nil
}

func (s *apiService) Run(ctx context.Context) error {
	if err := s.startServer(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}

func (s *apiService) Close(ctx context.Context) error {
	if s.srv != nil {
		s.srv.Stop()
	}

	return s.shutdownHTTP(ctx)
}

func (s *apiService) startServer() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.listenAddr == "" {
		log.Info("API server disabled (no listen address)")
		return nil
	}

	s.srv = server.NewServer(
		s.mw,
		s.accountService,
		s.eventService,
		s.crossSpaceSubService,
		s.vectorSearch,
		openapiYAML,
		openapiJSON,
	)

	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		host, _, splitErr := net.SplitHostPort(s.listenAddr)
		if splitErr != nil {
			log.Errorf("API server: failed to listen on %s: %v", s.listenAddr, err)
			return nil
		}
		log.Warnf("API server: %s is unavailable, picking a free port", s.listenAddr)
		listener, err = net.Listen("tcp", net.JoinHostPort(host, "0"))
		if err != nil {
			log.Errorf("API server: failed to listen on free port: %v", err)
			return nil
		}
	}

	s.httpSrv = &http.Server{
		Handler:           s.srv.Engine(),
		ReadHeaderTimeout: readTimeout,
	}

	log.Warnf("API server listening on %s", listener.Addr().String())

	go func() {
		if err := s.httpSrv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("API server error: %v", err)
		}
	}()

	return nil
}

func (s *apiService) shutdownHTTP(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	return s.httpSrv.Shutdown(shutdownCtx)
}

func (s *apiService) ReassignAddress(ctx context.Context, listenAddr string) error {
	if err := s.shutdownHTTP(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	s.listenAddr = listenAddr
	return s.startServer()
}

func SetMiddlewareParams(mw apicore.ClientCommands) {
	mwSrv = mw
}
