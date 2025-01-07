package application

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/migrator"
)

var (
	ErrAccountNotFound  = errors.New("account not found")
	ErrMigrationRunning = errors.New("migration is running")
)

type migration struct {
	mx         sync.Mutex
	isStarted  bool
	isFinished bool
	manager    *migrationManager
	err        error
	rootPath   string
	id         string
	done       chan struct{}
}

func newMigration(m *migrationManager, rootPath, id string) *migration {
	return &migration{
		done:     make(chan struct{}),
		rootPath: rootPath,
		id:       id,
		manager:  m,
	}
}

func newSuccessfulMigration(manager *migrationManager, rootPath, id string) *migration {
	m := newMigration(manager, rootPath, id)
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
	a := &app.App{}
	a.Register(m.manager.service.eventSender).
		Register(process.New()).
		Register(migrator.New())
	err := a.Start(context.Background())
	if err != nil {
		m.setFinished(err, true)
		return err
	}
	err = a.Close(context.Background())
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

func (m *migrationManager) getMigration(rootPath, id string) *migration {
	m.Lock()
	defer m.Unlock()
	if m.migrations == nil {
		m.migrations = make(map[string]*migration)
	}
	if m.migrations[id] == nil {
		// TODO: [store] add successful migration if we don't need a migration
		m.migrations[id] = newMigration(m, rootPath, id)
	}
	if m.migrations[id].finished() && !m.migrations[id].successful() {
		// resetting migration
		m.migrations[id] = newMigration(m, rootPath, id)
	}
	return m.migrations[id]
}

func (s *Service) AccountMigrate(ctx context.Context, req *pb.RpcAccountMigrateRequest) error {
	return s.migrationManager.getMigration(req.RootPath, req.Id).wait()
}
