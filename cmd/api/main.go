package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	_ "github.com/anyproto/anytype-heart/cmd/api/docs"
	"github.com/anyproto/anytype-heart/cmd/api/server"
	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/pb/service"
)

const (
	serverShutdownTime = 5 * time.Second
)

// RunApiServer starts the HTTP server and registers the API routes.
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
func RunApiServer(ctx context.Context, mw service.ClientCommandsServer, mwInternal core.MiddlewareInternal) {
	// Create a new server instance including the router
	srv := server.NewServer(mw, mwInternal)

	// Start the server in a goroutine so we can handle graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("API server error: %v\n", err)
		}
	}()

	// Graceful shutdown on CTRL+C / SIGINT
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), serverShutdownTime)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("API server shutdown failed: %v\n", err)
	}
}
