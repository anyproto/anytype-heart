package migratorfinisher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyproto/any-sync/app"
)

const (
	migratedName      = "space_store_migrated"
	objectStoreFolder = "objectstore"
	crdtDb            = "crdt"
)

const CName = "space.spacecore.storage.migratorfinisher"

type finisher struct {
	isMigrationDone bool

	newStorePath string
	oldPath      string
}

type Service interface {
	app.ComponentRunnable
	SetMigrationDone()
}

func New() Service {
	return &finisher{}
}

type pathProvider interface {
	GetNewSpaceStorePath() string
	GetOldSpaceStorePath() string
}

func (f *finisher) Init(a *app.App) (err error) {
	cfg := a.MustComponent("config").(pathProvider)
	f.newStorePath = cfg.GetNewSpaceStorePath()
	f.oldPath = cfg.GetOldSpaceStorePath()
	return nil
}

func (f *finisher) Name() (name string) {
	return CName
}

func (f *finisher) Run(ctx context.Context) (err error) {
	return nil
}

func (f *finisher) SetMigrationDone() {
	f.isMigrationDone = true
}

func (f *finisher) Close(ctx context.Context) error {
	if !f.isMigrationDone {
		return nil
	}
	err := f.removeCrdtIndexes()
	if err != nil {
		return nil
	}
	return f.renameOldStore()
}

func (f *finisher) renameOldStore() error {
	newName := migratedName
	newPath := filepath.Join(filepath.Dir(f.oldPath), newName+filepath.Ext(f.oldPath))
	err := os.Rename(f.oldPath, newPath)
	if err != nil {
		return fmt.Errorf("failed to rename: %w", err)
	}
	return nil
}

func (f *finisher) removeCrdtIndexes() error {
	rootDir := filepath.Join(filepath.Dir(f.newStorePath), objectStoreFolder)
	prefix := crdtDb
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), prefix) {
			if removeErr := os.Remove(path); removeErr != nil {
				return removeErr
			}
		}
		return nil
	})
}
