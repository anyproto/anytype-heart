package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/api/server"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	CName    = "api"
	httpPort = ":31009"
	timeout  = 5 * time.Second
)

var (
	logger = logging.Logger(CName)
	mwSrv  service.ClientCommandsServer
)

type Api interface {
	app.ComponentRunnable
}

type apiService struct {
	srv     *server.Server
	httpSrv *http.Server
	mw      service.ClientCommandsServer
}

func New() Api {
	return &apiService{
		mw: mwSrv,
	}
}

func (s *apiService) Name() (name string) {
	return CName
}

// @title						Anytype API
// @version					1.0
// @description				This API allows interaction with Anytype resources such as spaces, objects, and object types.
// @termsOfService				https://anytype.io/terms_of_use
// @contact.name				Anytype Support
// @contact.url				https://anytype.io/contact
// @contact.email				support@anytype.io
// @license.name				Any Source Available License 1.0
// @license.url				https://github.com/anyproto/anytype-ts/blob/main/LICENSE.md
// @host						localhost:31009
// @BasePath					/v1
// @securityDefinitions.basic	BasicAuth
// @externalDocs.description	OpenAPI
// @externalDocs.url			https://swagger.io/resources/open-api/
func (s *apiService) Init(a *app.App) (err error) {
	fmt.Println("Initializing API service...")

	s.srv = server.NewServer(a, s.mw)
	s.httpSrv = &http.Server{
		Addr:              httpPort,
		Handler:           s.srv.Engine(),
		ReadHeaderTimeout: timeout,
	}

	return nil
}

func (s *apiService) Run(ctx context.Context) (err error) {
	fmt.Printf("Starting API server on %s\n", s.httpSrv.Addr)

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("API server ListenAndServe error: %v\n", err)
		}
	}()

	return nil
}

func (s *apiService) Close(ctx context.Context) (err error) {
	fmt.Println("Closing API service...")

	// Give the server a short time to finish ongoing requests.
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
		fmt.Printf("API server shutdown error: %v\n", err)
		return err
	}

	fmt.Println("API service stopped gracefully.")
	return nil
}

func SetMiddlewareParams(mw service.ClientCommandsServer) {
	mwSrv = mw
}
