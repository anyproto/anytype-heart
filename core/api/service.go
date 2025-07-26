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
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/server"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	CName       = "api"
	readTimeout = 5 * time.Second
)

var (
	mwSrv apicore.ClientCommands

	//go:embed docs/openapi.yaml
	openapiYAML []byte

	//go:embed docs/openapi.json
	openapiJSON []byte
)

type Service interface {
	app.ComponentRunnable
	ReassignAddress(ctx context.Context, listenAddr string) (err error)
}

type apiService struct {
	srv                 *server.Server
	httpSrv             *http.Server
	mw                  apicore.ClientCommands
	accountService      apicore.AccountService
	eventService        apicore.EventService
	subscriptionService subscription.Service
	listenAddr          string
	lock                sync.Mutex

	eventQueue  *mb.MB[*pb.EventMessage]
	eventCtx    context.Context
	eventCancel context.CancelFunc
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
func (s *apiService) Init(a *app.App) (err error) {
	s.listenAddr = a.MustComponent(config.CName).(*config.Config).JsonApiListenAddr
	s.accountService = a.MustComponent(account.CName).(account.Service)
	s.eventService = a.MustComponent(event.CName).(apicore.EventService)
	s.subscriptionService = app.MustComponent[subscription.Service](a)
	return nil
}

func (s *apiService) Run(ctx context.Context) (err error) {
	// Start event listener first so the queue is ready
	s.startEventListener()
	s.runServer()
	return nil
}

func (s *apiService) Close(ctx context.Context) (err error) {
	if s.eventCancel != nil {
		s.eventCancel()
	}
	if s.eventQueue != nil {
		s.eventQueue.Close()
	}
	if s.srv != nil {
		s.srv.Stop()
	}
	return s.shutdown(ctx)
}

func (s *apiService) runServer() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.listenAddr == "" {
		// means that API is disabled
		return
	}

	s.srv = server.NewServer(s.mw, s.accountService, s.eventService, openapiYAML, openapiJSON)

	// Set the event queue and subscription service for real-time updates
	if s.eventQueue != nil {
		s.srv.SetEventQueue(s.eventQueue)
	}
	if s.subscriptionService != nil {
		s.srv.SetSubscriptionService(s.subscriptionService)
	}

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

func SetMiddlewareParams(mw apicore.ClientCommands) {
	mwSrv = mw
}

// startEventListener starts a goroutine to listen for events from internal subscriptions
func (s *apiService) startEventListener() {
	// Create the event queue first
	s.eventQueue = mb.New[*pb.EventMessage](0)
	s.eventCtx, s.eventCancel = context.WithCancel(context.Background())

	go s.listenForEvents()
}

// listenForEvents processes events from the internal subscription queue
func (s *apiService) listenForEvents() {
	log := logging.Logger("api-event-listener")
	log.Warn("Starting API event listener")

	for {
		select {
		case <-s.eventCtx.Done():
			log.Warn("Stopping API event listener")
			return
		default:
			msgs, err := s.eventQueue.Wait(s.eventCtx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Errorf("Error waiting for events: %v", err)
				}
				return
			}

			if len(msgs) > 0 && s.srv != nil {
				// Process events through the API server
				event := &pb.Event{Messages: msgs}
				s.srv.ProcessEvent(event)
			}
		}
	}
}

// GetEventQueue returns the event queue for internal subscriptions
func (s *apiService) GetEventQueue() *mb.MB[*pb.EventMessage] {
	return s.eventQueue
}
