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
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

var (
	ErrAccountNotFound  = errors.New("account not found")
	ErrMigrationRunning = errors.New("migration is running")
)

func (s *Service) AccountMigrate(ctx context.Context, req *pb.RpcAccountMigrateRequest) error {
	if s.rootPath == "" {
		s.rootPath = req.RootPath
	}
	if s.tmpPath == "" {
		s.tmpPath = req.TmpPath
	}
	return s.migrationManager.getOrCreateMigration(req.RootPath, req.Id).wait()
}

func (s *Service) AccountMigrateCancel(ctx context.Context, req *pb.RpcAccountMigrateCancelRequest) error {
	m := s.migrationManager.getMigration(req.Id)
	if m == nil {
		return nil
	}
	m.cancelMigration()
	return nil
}

func (s *Service) migrate(ctx context.Context, id string) error {
	res, err := core.WalletAccountAt(s.mnemonic, 0)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.rootPath, id)); err != nil {
		if os.IsNotExist(err) {
			return ErrAccountNotFound
		}
		return err
	}
	cfg := anytype.BootstrapConfig(false, os.Getenv("ANYTYPE_STAGING") == "1")
	cfg.PeferYamuxTransport = true
	cfg.DisableNetworkIdCheck = true
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(s.rootPath, s.tmpPath, res),
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
	id         string
	done       chan struct{}
}

func newMigration(m *migrationManager, id string) *migration {
	ctx, cancel := context.WithCancel(context.Background())
	return &migration{
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
		id:      id,
		manager: m,
	}
}

func newSuccessfulMigration(manager *migrationManager, id string) *migration {
	m := newMigration(manager, id)
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
		m.manager.setMigrationRunning(m.id, false)
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
	if !m.manager.setMigrationRunning(m.id, true) {
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
	err := m.manager.service.migrate(m.ctx, m.id)
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

func (m *migrationManager) setMigrationRunning(id string, isRunning bool) bool {
	m.Lock()
	defer m.Unlock()
	if (m.runningMigration != "" && m.runningMigration != id) && isRunning {
		return false
	}
	if m.runningMigration == "" && !isRunning {
		panic("migration is not running")
	}
	if isRunning {
		m.runningMigration = id
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

func (m *migrationManager) getOrCreateMigration(rootPath, id string) *migration {
	m.Lock()
	defer m.Unlock()
	if m.migrations == nil {
		m.migrations = make(map[string]*migration)
	}
	if m.migrations[id] == nil {
		sqlitePath := filepath.Join(rootPath, id, config.SpaceStoreSqlitePath)
		baderPath := filepath.Join(rootPath, id, config.SpaceStoreBadgerPath)
		if anyPathExists([]string{sqlitePath, baderPath}) {
			m.migrations[id] = newMigration(m, id)
		} else {
			m.migrations[id] = newSuccessfulMigration(m, id)
		}
	}
	if m.migrations[id].finished() && !m.migrations[id].successful() {
		// resetting migration
		m.migrations[id] = newMigration(m, id)
	}
	return m.migrations[id]
}

func (m *migrationManager) getMigration(id string) *migration {
	m.Lock()
	defer m.Unlock()
	return m.migrations[id]
}

func anyPathExists(paths []string) bool {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
