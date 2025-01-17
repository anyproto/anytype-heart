package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/api/server"
	"github.com/anyproto/anytype-heart/pb/service"
)

const (
	CName    = "api"
	httpPort = ":31009"
	timeout  = 5 * time.Second
)

var (
	mwSrv                   service.ClientCommandsServer
	ErrPortAlreadyUsed      = fmt.Errorf("port %s is already in use", httpPort)
	ErrServerAlreadyStarted = fmt.Errorf("server already started")
	ErrServerNotStarted     = fmt.Errorf("server not started")
)

type Service interface {
	Start() error
	Stop() error
	app.ComponentRunnable
}

type apiService struct {
	srv     *server.Server
	httpSrv *http.Server
	mw      service.ClientCommandsServer
}

func New() Service {
	return &apiService{
		mw: mwSrv,
	}
}

func (s *apiService) Name() (name string) {
	return CName
}

// Init initializes the API service.
//
//	@title						Anytype API
//	@version					1.0
//	@description				This API allows interaction with Anytype resources such as spaces, objects, and object types.
//	@termsOfService				https://anytype.io/terms_of_use
//	@contact.name				Anytype Support
//	@contact.url				https://anytype.io/contact
//	@contact.email				support@anytype.io
//	@license.name				Any Source Available License 1.0
//	@license.url				https://github.com/anyproto/anytype-ts/blob/main/LICENSE.md
//	@host						localhost:31009
//	@BasePath					/v1
//	@securityDefinitions.basic	BasicAuth
//	@externalDocs.description	OpenAPI
//	@externalDocs.url			https://swagger.io/resources/open-api/
func (s *apiService) Init(a *app.App) (err error) {
	s.srv = server.NewServer(a, s.mw)
	return nil
}

func (s *apiService) Run(ctx context.Context) (err error) {
	// TODO: remove once client takes responsibility
	s.httpSrv = &http.Server{
		Addr:              httpPort,
		Handler:           s.srv.Engine(),
		ReadHeaderTimeout: timeout,
	}

	fmt.Printf("Starting API server on %s\n", s.httpSrv.Addr)

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if strings.Contains(err.Error(), "address already in use") {
				fmt.Printf("API server ListenAndServe error: %v\n", ErrPortAlreadyUsed)
			} else {
				fmt.Printf("API server ListenAndServe error: %v\n", err)
			}
		}
	}()

	return nil
}

func (s *apiService) Close(ctx context.Context) (err error) {
	if s.httpSrv == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	return nil
}

func (s *apiService) Start() error {
	if s.httpSrv != nil {
		return ErrServerAlreadyStarted
	}

	s.httpSrv = &http.Server{
		Addr:              httpPort,
		Handler:           s.srv.Engine(),
		ReadHeaderTimeout: timeout,
	}

	fmt.Printf("Starting API server on %s\n", s.httpSrv.Addr)

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if strings.Contains(err.Error(), "address already in use") {
				fmt.Printf("API server ListenAndServe error: %v\n", ErrPortAlreadyUsed)
			} else {
				fmt.Printf("API server ListenAndServe error: %v\n", err)
			}
		}
	}()

	return nil
}

func (s *apiService) Stop() error {
	if s.httpSrv == nil {
		return ErrServerNotStarted
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	// Clear the server reference to allow reinitialization
	s.httpSrv = nil
	return nil
}

func SetMiddlewareParams(mw service.ClientCommandsServer) {
	mwSrv = mw
}
