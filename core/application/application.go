package application

import (
	"context"
	"errors"
	"runtime/trace"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/vcs"
)

var log = logging.Logger("anytype-core-account")

type Service struct {
	lock sync.RWMutex

	app *app.App

	// pre-derived keys (populated during wallet.create or wallet.recover)
	derivedKeys *crypto.DerivationResult

	// session signing key for session tokens
	sessionSigningKey []byte
	// sessionsByAppHash holds EVERY live session token minted from an app key
	// (directly or derived via the token auth branch), so revoking the key can
	// close all of them (H4: revocation must reach every session minted from
	// the key). appHashByToken is the reverse index, populated on both mint
	// paths. Entries are released only by CloseSession and LinkLocalRevokeApp;
	// tokens whose owners never close them stay tracked until process exit.
	//
	// Both maps are guarded by appSessionsLock, not by lock: the critical
	// sections deliberately span the session-service calls (see sessions.go),
	// and lock is held for the whole of AccountSelect/AccountStop, which would
	// stall WalletCreateSession/WalletCloseSession for their full duration.
	sessionsByAppHash map[string]map[string]struct{}
	appHashByToken    map[string]string
	// appSessionsLock serializes session mint, close and revoke. Minting from
	// a token or an app key MUST validate/read and track in one critical
	// section with the revoke sweep: otherwise a WalletCreateSession racing a
	// LinkLocalRevokeApp can mint from a not-yet-closed token after the index
	// was swept, laundering the revoked key into an untracked, unrevokable
	// session. Never acquire lock while holding appSessionsLock.
	appSessionsLock sync.Mutex

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
		sessionsByAppHash: make(map[string]map[string]struct{}),
		appHashByToken:    make(map[string]string),
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
	ctx, task := trace.NewTask(context.Background(), "application.stop")
	defer task.End()

	if s != nil && s.app != nil {
		mwVersion := vcs.GetVCSInfo().Version()
		log.Infow("closing app: initiated", "mwVersion", mwVersion)
		s.app.SetDeviceState(int(domain.CompStateAppClosingInitiated))
		start := time.Now()
		err := s.app.Close(ctx)
		if err != nil {
			log.Warnf("error while stop anytype: %v", err)
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
