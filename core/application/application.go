package application

import (
	"context"
	"errors"
	"runtime/trace"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/application/accountdirlock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/vcs"
)

var log = logging.Logger("anytype-core-account")

type Service struct {
	lock sync.RWMutex

	app          *app.App
	accountLease *accountdirlock.Lease

	// pre-derived keys (populated during wallet.create or wallet.recover)
	derivedKeys *crypto.DerivationResult

	// session signing key for session tokens
	sessionSigningKey []byte
	sessionsByAppHash map[string]string

	rootPath                string
	fulltextPrimaryLanguage string
	clientWithVersion       string
	eventSender             event.Sender
	sessions                session.Service
	traceRecorder           *traceRecorder
	migrationManager        *migrationManager

	appAccountStartInProcessCancel      context.CancelFunc
	appAccountStartInProcessCancelMutex sync.Mutex
}

func New() *Service {
	s := &Service{
		sessions:          session.New(),
		traceRecorder:     &traceRecorder{},
		sessionsByAppHash: make(map[string]string),
	}
	m := newMigrationManager(s)
	s.migrationManager = m
	return s
}

func (s *Service) GetApp() *app.App {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.app
}

func (s *Service) requireClientWithVersion() {
	if s.clientWithVersion == "" {
		panic(errors.New("client platform with the version must be set using the InitialSetParameters method"))
	}
}

func (s *Service) Stop() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.stop()
}

func (s *Service) stop() error {
	return errors.Join(s.closeApp(), s.releaseAccountLease())
}

// closeApp closes all account components but deliberately retains the account
// lease. Operations such as restart, move, and deletion must remain protected
// until they have finished touching account data.
func (s *Service) closeApp() error {
	ctx, task := trace.NewTask(context.Background(), "application.stop")
	defer task.End()

	if s != nil && s.app != nil {
		mwVersion := vcs.GetVCSInfo().Version()
		log.Infow("closing app: initiated", "mwVersion", mwVersion)
		s.app.SetDeviceState(int(domain.CompStateAppClosingInitiated))
		start := time.Now()
		closeErr := s.app.Close(ctx)
		if closeErr != nil {
			log.Warnf("error while stop anytype: %v", closeErr)
		}
		log.Infow("closing app: finished", "mwVersion", mwVersion, "tookMs", time.Since(start).Milliseconds())
		// Drain zap's buffered sink (the "closing app: finished" line above
		// must reach disk) and stop the lumberjack flush goroutine. Both
		// calls are bounded so a stuck disk doesn't hang process exit;
		// errors are benign (stderr Sync can fail on some platforms) and
		// intentionally ignored.
		_ = logging.SyncWithTimeout(3 * time.Second)
		_ = logging.CloseSink(3 * time.Second)

		s.app = nil
		return closeErr
	}
	return nil
}

func (s *Service) SetClientVersion(platform string, version string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.clientWithVersion = platform + ":" + version
}

func (s *Service) SetEventSender(sender event.Sender) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.eventSender = sender
}

func (s *Service) GetEventSender() event.Sender {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.eventSender
}
