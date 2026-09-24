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
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
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
	// ReassignAddress rebinds the JSON API to listenAddr, returning the bind
	// outcome directly to the caller. A failed bind is reported through the
	// returned status (Success == false), not through err: err is reserved
	// for failures that kept the rebind from being attempted at all (e.g.
	// shutting down the previous listener).
	ReassignAddress(ctx context.Context, listenAddr string) (*pb.EventAccountJsonApiStatus, error)
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
	objectProvenance     apicore.ObjectProvenance
	widgets              apicore.Widgets
	objectStore          objectstore.ObjectStore

	listenAddr string

	srv      *server.Server
	httpSrv  *http.Server
	listener net.Listener

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
	// testable against mock_apicore.
	s.chatSubService = &chatSubAdapter{svc: a.MustComponent(chatsubscription.CName).(chatsubscription.Service)}
	s.fileObjectService = a.MustComponent(fileobject.CName).(apicore.FileObjectService)
	s.objectReader = newObjectReadAdapter(app.MustComponent[cache.ObjectGetterComponent](a))
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.objectCreator = newObjectCreateAdapter(app.MustComponent[objectcreator.Service](a), app.MustComponent[space.Service](a),
		app.MustComponent[template.Service](a), s.objectStore)
	s.objectMutator = newObjectMutateAdapter(app.MustComponent[cache.ObjectGetterComponent](a))
	s.objectProvenance = newObjectProvenanceAdapter(app.MustComponent[space.Service](a), a.MustComponent(account.CName).(account.Service))
	s.widgets = newWidgetAdapter(app.MustComponent[cache.ObjectGetterComponent](a), app.MustComponent[space.Service](a))
	return nil
}

func (s *apiService) Run(ctx context.Context) error {
	// A failed bind must not fail the app's component lifecycle: the JSON
	// API is an optional side feature of account select/create. The bind
	// outcome (success or failure) is reported to clients via the
	// accountJsonApiStatus event published from startServer, not via this
	// return value.
	s.startServer(s.listenAddr)
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

// startServer attempts one bind of the JSON API to listenAddr and reports
// the outcome as a status value — a failed bind is a value (Success ==
// false), never a Go error, so every caller (the component lifecycle and
// ReassignAddress) reads it off one place. Returns nil only when the server
// is disabled (listenAddr == ""), in which case nothing is published.
//
// Publishing happens here, outside bindLocked's critical section: the
// eventService.Broadcast it triggers is synchronous on the mobile/library
// sender (unlike the queued gRPC sender), so a client callback that
// re-enters this service — e.g. retrying AccountChangeJsonApiAddr from
// inside its own event handler — must not find s.lock still held.
func (s *apiService) startServer(listenAddr string) *pb.EventAccountJsonApiStatus {
	status := s.bindLocked(listenAddr)
	if status != nil {
		s.publishStatus(status)
	}
	return status
}

// bindLocked does the actual (re)bind under s.lock and returns the outcome
// without publishing it. listenAddr is taken as a parameter, and s.listenAddr
// is written here (under the lock) rather than by the caller, so there is
// exactly one, synchronized writer.
func (s *apiService) bindLocked(listenAddr string) *pb.EventAccountJsonApiStatus {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.listenAddr = listenAddr
	if listenAddr == "" {
		log.Info("API server disabled (no listen address)")
		return nil
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Errorf("API server failed to start on %s: %v", listenAddr, err)
		return &pb.EventAccountJsonApiStatus{Success: false, ListenAddr: listenAddr, Error: err.Error()}
	}

	// server.NewServer panics if the account's tech space can't be
	// resolved. Until ownership of ln is handed to the Serve goroutine
	// below, this keeps the listener from leaking past that panic (or any
	// early return added here later).
	closeListener := true
	defer func() {
		if closeListener {
			_ = ln.Close()
		}
	}()

	s.srv = server.NewServer(
		s.mw,
		s.accountService,
		s.eventService,
		s.crossSpaceSubService,
		s.chatSubService,
		s.fileObjectService,
		server.V2Deps{Reader: s.objectReader, Creator: s.objectCreator, Mutator: s.objectMutator, Provenance: s.objectProvenance, Widgets: s.widgets, ChatSub: s.chatSubService, Store: s.objectStore, AccountId: s.accountId()},
		listenAddr,
		server.OpenApiDocs{
			V1YAML: openapiV1YAML,
			V1JSON: openapiV1JSON,
			V2YAML: openapiV2YAML,
			V2JSON: openapiV2JSON,
		},
	)

	// httpSrv is captured by the goroutine below instead of read back off
	// s.httpSrv: a concurrent ReassignAddress can only start once this call
	// releases s.lock, but its own bindLocked call would still overwrite
	// s.httpSrv before this goroutine gets scheduled. A live field read
	// would then hand ln — the listener THIS call just bound — to the
	// newer server, silently keeping the address this call owns reachable
	// (routed through the wrong handler) even after a later shutdownHTTP.
	httpSrv := &http.Server{
		Handler:           s.srv.Engine(),
		ReadHeaderTimeout: readTimeout,
	}
	s.httpSrv = httpSrv
	s.listener = ln

	status := &pb.EventAccountJsonApiStatus{Success: true, ListenAddr: ln.Addr().String()}
	log.Infof("Starting API server on %s", status.ListenAddr)

	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("API server error: %v", err)
		}
	}()

	closeListener = false
	return status
}

// publishStatus broadcasts the outcome of one bind attempt as an
// account-level event (spaceId ""), same type ReassignAddress hands back to
// its own caller. Must be called without s.lock held.
func (s *apiService) publishStatus(status *pb.EventAccountJsonApiStatus) {
	s.eventService.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountJsonApiStatus{
		AccountJsonApiStatus: status,
	}))
}

// shutdownHTTP tears down whatever is currently bound, if anything. The
// listener is closed explicitly rather than left to httpSrv.Shutdown alone:
// Shutdown only closes listeners Serve has already registered with the
// server, and the Serve goroutine bindLocked starts may not have run yet by
// the time shutdownHTTP is called — without this, that still-unregistered
// listener would keep accepting connections indefinitely.
func (s *apiService) shutdownHTTP(ctx context.Context) error {
	s.lock.Lock()
	httpSrv := s.httpSrv
	listener := s.listener
	s.httpSrv = nil
	s.listener = nil
	s.lock.Unlock()

	if httpSrv == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	err := httpSrv.Shutdown(shutdownCtx)
	if listener != nil {
		_ = listener.Close()
	}
	return err
}

func (s *apiService) ReassignAddress(ctx context.Context, listenAddr string) (*pb.EventAccountJsonApiStatus, error) {
	if err := s.shutdownHTTP(ctx); err != nil {
		return nil, fmt.Errorf("shutdown server: %w", err)
	}

	return s.startServer(listenAddr), nil
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
