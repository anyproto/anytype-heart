package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/api/server"
	"github.com/anyproto/anytype-heart/pb/service"
)

const (
	CName       = "api"
	readTimeout = 5 * time.Second
)

var (
	mwSrv service.ClientCommandsServer
)

type Service interface {
	app.ComponentRunnable
	ReassignAddress(ctx context.Context, listenAddr string) (err error)
}

type apiService struct {
	srv            *server.Server
	httpSrv        *http.Server
	mw             service.ClientCommandsServer
	accountService account.Service
	listenAddr     string
	lock           sync.Mutex
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
//	@version						1.0
//	@description					This API allows interaction with Anytype resources such as spaces, objects and types.
//	@termsOfService					https://anytype.io/terms_of_use
//	@contact.name					Anytype Support
//	@contact.url					https://anytype.io/contact
//	@contact.email					support@anytype.io
//	@license.name					Any Source Available License 1.0
//	@license.url					https://github.com/anyproto/anytype-ts/blob/main/LICENSE.md
//	@host							localhost:31009
//	@BasePath						/v1
//	@securitydefinitions.bearerauth	BearerAuth
//	@externalDocs.description		OpenAPI
//	@externalDocs.url				https://swagger.io/resources/open-api/
func (s *apiService) Init(a *app.App) (err error) {
	s.listenAddr = a.MustComponent(config.CName).(*config.Config).JsonApiListenAddr
	s.accountService = a.MustComponent(account.CName).(account.Service)
	return nil
}

func (s *apiService) Run(ctx context.Context) (err error) {
	s.runServer()
	return nil
}

func (s *apiService) Close(ctx context.Context) (err error) {
	return s.shutdown(ctx)
}

func (s *apiService) runServer() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.listenAddr == "" {
		// means that API is disabled
		return
	}

	s.srv = server.NewServer(s.accountService, s.mw)
	s.httpSrv = &http.Server{
		Addr:              s.listenAddr,
		Handler:           s.srv.Engine(),
		ReadHeaderTimeout: readTimeout,
	}

	fmt.Printf("Starting API server on %s\n", s.httpSrv.Addr)

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("API server ListenAndServe error: %v\n", err)
		}
	}()
}

func (s *apiService) shutdown(ctx context.Context) (err error) {
	if s.httpSrv == nil {
		return nil
	}
	s.lock.Lock()
	defer s.lock.Unlock()

	// we don't want graceful shutdown here and block the app close
	shutdownCtx, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	return nil
}

func (s *apiService) ReassignAddress(ctx context.Context, listenAddr string) (err error) {
	err = s.shutdown(ctx)
	if err != nil {
		return err
	}

	s.listenAddr = listenAddr
	s.runServer()
	return nil
}

func SetMiddlewareParams(mw service.ClientCommandsServer) {
	mwSrv = mw
}
