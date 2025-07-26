package api

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/server"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/subscription"
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
	mw                  apicore.ClientCommands
	accountService      apicore.AccountService
	eventService        apicore.EventService
	subscriptionService subscription.Service

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
//	@version						2025-05-20
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
	s.subscriptionService = app.MustComponent[subscription.Service](a)
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
		s.subscriptionService,
		openapiYAML,
		openapiJSON,
	)

	s.httpSrv = &http.Server{
		Addr:              s.listenAddr,
		Handler:           s.srv.Engine(),
		ReadHeaderTimeout: readTimeout,
	}

	log.Infof("Starting API server on %s", s.httpSrv.Addr)

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
