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
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
)

const (
	CName           = "api"
	readTimeout     = 5 * time.Second
	shutdownTimeout = time.Millisecond
)

var (
	log = logging.Logger("api-service")

	mwSrv apicore.ClientCommands

	// The generated documents, one per API version. They are data only:
	// `make openapi` writes just openapi.{json,yaml} (--outputTypes json,yaml),
	// no docs.go — nothing in this binary ever read swag's global registry, and
	// the bytes below are what the /docs routes actually serve.
	//go:embed docs/v1/openapi.yaml
	openapiV1YAML []byte

	//go:embed docs/v1/openapi.json
	openapiV1JSON []byte

	//go:embed docs/v2/openapi.yaml
	openapiV2YAML []byte

	//go:embed docs/v2/openapi.json
	openapiV2JSON []byte
)

type Service interface {
	app.ComponentRunnable
	ReassignAddress(ctx context.Context, listenAddr string) error
	RevokeToken(token string)
}

type apiService struct {
	mw                   apicore.ClientCommands
	accountService       apicore.AccountService
	eventService         apicore.EventService
	crossSpaceSubService apicore.CrossSpaceSubscriptionService
	chatSubService       apicore.ChatSubscriptionService
	fileObjectService    apicore.FileObjectService
	objectReader         apicore.ObjectReader
	objectCreator        apicore.ObjectCreator
	objectMutator        apicore.ObjectMutator
	objectStore          objectstore.ObjectStore

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
	// The adapters below (chatSubAdapter, objectRead/Create/Mutate) stay in
	// package api on purpose, even though the object adapters serve /v2 only:
	// package api is this tree's composition root — the only package that
	// touches *app.App and the heart-internal services (block/cache,
	// objectcreator, space, chatsubscription, fileobject). What they produce
	// are implementations of the apicore ports, and apicore is shared by both
	// API versions, so an adapter is a shared-side artifact by construction.
	// Keeping them here is also what keeps core/api/v2 free of heart-internal
	// imports: v2 is HTTP plus logic over ports, which is what makes it
	// testable against mock_apicore. See core/api/APIV2_LAYOUT_PLAN.md §10.
	s.chatSubService = &chatSubAdapter{svc: a.MustComponent(chatsubscription.CName).(chatsubscription.Service)}
	s.fileObjectService = a.MustComponent(fileobject.CName).(apicore.FileObjectService)
	s.objectReader = newObjectReadAdapter(app.MustComponent[cache.ObjectGetterComponent](a))
	s.objectCreator = newObjectCreateAdapter(app.MustComponent[objectcreator.Service](a), app.MustComponent[space.Service](a))
	s.objectMutator = newObjectMutateAdapter(app.MustComponent[cache.ObjectGetterComponent](a))
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (s *apiService) Run(ctx context.Context) error {
	if err := s.startServer(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}

// The accountId probe below is structural — if account.Service ever renamed
// AccountID, the probe would still compile, silently return "" and degrade
// every current-user placeholder to a warning. This assertion turns that
// rename into a compile error instead.
var _ interface{ AccountID() string } = (account.Service)(nil)

// accountId returns the caller's account identity for API v2's stored-view
// placeholder substitution (`_filter_template_2_` → participant id). The
// apicore.AccountService port only exposes GetInfo, so the richer concrete
// account component is probed for its AccountID; a foreign implementation
// degrades to "" (the placeholder then warns instead of resolving).
func (s *apiService) accountId() string {
	if withId, ok := s.accountService.(interface{ AccountID() string }); ok {
		return withId.AccountID()
	}
	return ""
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
		s.chatSubService,
		s.fileObjectService,
		server.V2Deps{Reader: s.objectReader, Creator: s.objectCreator, Mutator: s.objectMutator, Store: s.objectStore, AccountId: s.accountId()},
		s.listenAddr,
		server.OpenApiDocs{
			V1YAML: openapiV1YAML,
			V1JSON: openapiV1JSON,
			V2YAML: openapiV2YAML,
			V2JSON: openapiV2JSON,
		},
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

// RevokeToken removes a cached API session token from the server's in-memory cache.
func (s *apiService) RevokeToken(token string) {
	s.lock.Lock()
	srv := s.srv
	s.lock.Unlock()
	if srv != nil {
		srv.RevokeToken(token)
	}
}

func SetMiddlewareParams(mw apicore.ClientCommands) {
	mwSrv = mw
}
