package application

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/migrator"
)

var ErrAccountNotFound = errors.New("account not found")

type migration struct {
	isStarted atomic.Bool
	isSuccess atomic.Bool
	service   *Service
	err       error
	done      chan struct{}
}

func newMigration(s *Service) *migration {
	return &migration{
		done:    make(chan struct{}),
		service: s,
	}
}

func newSuccessfulMigration(s *Service) *migration {
	m := newMigration(s)
	m.isSuccess.Store(true)
	close(m.done)
	return m
}

func (m *migration) wait() error {
	if !m.isStarted.CompareAndSwap(false, true) {
		<-m.done
		return m.err
	}
	a := &app.App{}
	a.Register(m.service.eventSender).
		Register(process.New()).
		Register(migrator.New())
	err := a.Start(context.Background())
	if err != nil {
		m.err = err
		close(m.done)
		return err
	}
	err = a.Close(context.Background())
	if err != nil {
		m.err = err
		close(m.done)
		return err
	}
	close(m.done)
	m.isSuccess.Store(true)
	return nil
}

func (m *migration) successful() bool {
	return m.isSuccess.Load()
}

type migrationManager struct {
	migrations map[string]*migration
	service    *Service
	sync.Mutex
}

func newMigrationManager(s *Service) *migrationManager {
	return &migrationManager{
		service: s,
	}
}

func (m *migrationManager) getMigration(rootPath, id string) *migration {
	m.Lock()
	defer m.Unlock()
	if m.migrations == nil {
		m.migrations = make(map[string]*migration)
	}
	if m.migrations[id] == nil {
		// TODO: [store] add successful migration if we don't need a migration
		m.migrations[id] = newMigration(m.service)
	}
	return m.migrations[id]
}

func (s *Service) AccountMigrate(ctx context.Context, req *pb.RpcAccountMigrateRequest) error {
	return s.migrationManager.getMigration(req.RootPath, req.Id).wait()
}
