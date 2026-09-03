package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/application/accountdirlock"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/vcs"
)

var (
	ErrAccountNotFound  = errors.New("account not found")
	ErrMigrationRunning = errors.New("migration is running")
)

func (s *Service) AccountMigrate(ctx context.Context, req *pb.RpcAccountMigrateRequest) error {
	if err := s.validateAccountID(req.Id); err != nil {
		return err
	}
	rootPath := req.RootPath
	if rootPath == "" {
		rootPath = s.rootPath
	}
	if s.rootPath == "" {
		s.rootPath = rootPath
	}
	migration, err := s.migrationManager.getOrCreateMigration(rootPath, req.Id, req.FulltextPrimaryLanguage)
	if err != nil {
		return err
	}
	return migration.wait()
}

func (s *Service) AccountMigrateCancel(ctx context.Context, req *pb.RpcAccountMigrateCancelRequest) error {
	m := s.migrationManager.getMigration(req.Id)
	if m == nil {
		return nil
	}
	m.cancelMigration()
	return nil
}

func (s *Service) migrate(ctx context.Context, rootPath, id, lang string) (err error) {
	if s.derivedKeys == nil {
		return ErrWalletNotInitialized
	}
	if _, err := os.Stat(filepath.Join(rootPath, id)); err != nil {
		if os.IsNotExist(err) {
			return ErrAccountNotFound
		}
		return err
	}
	lease, err := accountdirlock.Acquire(ctx, rootPath, id, vcs.GetVCSInfo().Version())
	if err != nil {
		if errors.Is(err, accountdirlock.ErrLocked) {
			return errors.Join(ErrAnotherProcessIsRunning, err)
		}
		return err
	}
	defer func() {
		err = errors.Join(err, lease.Release())
	}()
	cfg := anytype.BootstrapConfig(false, "")
	cfg.PeferYamuxTransport = true
	cfg.DisableNetworkIdCheck = true
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(rootPath, *s.derivedKeys, lang),
		s.eventSender,
	}
	a := &app.App{}
	anytype.BootstrapMigration(a, comps...)
	err = a.Start(ctx)
	if err != nil {
		return err
	}
	return a.Close(ctx)
}

type migration struct {
	mx         sync.Mutex
	isStarted  bool
	isFinished bool
	ctx        context.Context
	cancel     context.CancelFunc
	manager    *migrationManager
	err        error
	key        string
	rootPath   string
	id         string
	done       chan struct{}
	lang       string
}

func newMigration(m *migrationManager, key, rootPath, id, lang string) *migration {
	ctx, cancel := context.WithCancel(context.Background())
	return &migration{
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		key:      key,
		rootPath: rootPath,
		id:       id,
		lang:     lang,
		manager:  m,
	}
}

func newSuccessfulMigration(manager *migrationManager, key, rootPath, id, lang string) *migration {
	m := newMigration(manager, key, rootPath, id, lang)
	m.setFinished(nil, false)
	return m
}

func (m *migration) setFinished(err error, notify bool) {
	m.mx.Lock()
	defer m.mx.Unlock()
	m.isFinished = true
	m.err = err
	close(m.done)
	if notify {
		m.manager.setMigrationRunning(m.key, false)
	}
}

func (m *migration) cancelMigration() {
	m.cancel()
	err := m.wait()
	if err != nil {
		log.Warn("failed to wait for migration to finish", zap.Error(err))
	}
}

func (m *migration) wait() error {
	m.mx.Lock()
	if !m.manager.setMigrationRunning(m.key, true) {
		m.mx.Unlock()
		return ErrMigrationRunning
	}
	if !m.isStarted {
		m.isStarted = true
	} else {
		m.mx.Unlock()
		<-m.done
		return m.err
	}
	m.mx.Unlock()
	err := m.manager.service.migrate(m.ctx, m.rootPath, m.id, m.lang)
	if err != nil {
		m.setFinished(err, true)
		return err
	}
	m.setFinished(nil, true)
	return nil
}

func (m *migration) successful() bool {
	m.mx.Lock()
	defer m.mx.Unlock()
	return m.isFinished && m.err == nil
}

func (m *migration) finished() bool {
	m.mx.Lock()
	defer m.mx.Unlock()
	return m.isFinished
}

type migrationManager struct {
	migrations       map[string]*migration
	service          *Service
	runningMigration string
	sync.Mutex
}

func newMigrationManager(s *Service) *migrationManager {
	return &migrationManager{
		service: s,
	}
}

func (m *migrationManager) setMigrationRunning(key string, isRunning bool) bool {
	m.Lock()
	defer m.Unlock()
	if (m.runningMigration != "" && m.runningMigration != key) && isRunning {
		return false
	}
	if m.runningMigration == "" && !isRunning {
		panic("migration is not running")
	}
	if isRunning {
		m.runningMigration = key
	} else {
		m.runningMigration = ""
	}
	return true
}

func (m *migrationManager) isRunning() bool {
	m.Lock()
	defer m.Unlock()
	return m.runningMigration != ""
}

func (m *migrationManager) getOrCreateMigration(rootPath, id, lang string) (*migration, error) {
	canonicalRoot, err := accountdirlock.CanonicalRootPath(rootPath)
	if err != nil {
		return nil, errors.Join(accountdirlock.ErrUnavailable, err)
	}
	key := canonicalRoot + "\x00" + id
	m.Lock()
	defer m.Unlock()
	if m.migrations == nil {
		m.migrations = make(map[string]*migration)
	}
	if m.migrations[key] == nil {
		sqlitePath := filepath.Join(canonicalRoot, id, config.SpaceStoreSqlitePath)
		baderPath := filepath.Join(canonicalRoot, id, config.SpaceStoreBadgerPath)
		if anyPathExists([]string{sqlitePath, baderPath}) {
			m.migrations[key] = newMigration(m, key, canonicalRoot, id, lang)
		} else {
			m.migrations[key] = newSuccessfulMigration(m, key, canonicalRoot, id, lang)
		}
	}
	if m.migrations[key].finished() && !m.migrations[key].successful() {
		// resetting migration
		m.migrations[key] = newMigration(m, key, canonicalRoot, id, lang)
	}
	return m.migrations[key], nil
}

func (m *migrationManager) getMigration(id string) *migration {
	m.Lock()
	defer m.Unlock()
	if running := m.migrations[m.runningMigration]; running != nil && running.id == id {
		return running
	}
	for _, migration := range m.migrations {
		if migration.id == id {
			return migration
		}
	}
	return nil
}

func anyPathExists(paths []string) bool {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
