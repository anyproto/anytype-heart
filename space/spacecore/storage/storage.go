package storage

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"

	"github.com/anyproto/anytype-heart/space/spacecore/storage/badgerstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/sqlitestorage"
)

type SpaceStorageMode int

const (
	SpaceStorageModeSqlite SpaceStorageMode = iota // used for new account repos
	SpaceStorageModeBadger                         // used for existing account repos
)

type ClientStorage interface {
	spacestorage.SpaceStorageProvider
	app.ComponentRunnable
	AllSpaceIds() (ids []string, err error)
	GetSpaceID(objectID string) (spaceID string, err error)
	BindSpaceID(spaceID, objectID string) (err error)
	DeleteSpaceStorage(ctx context.Context, spaceId string) error
	MarkSpaceCreated(id string) (err error)
	UnmarkSpaceCreated(id string) (err error)
	IsSpaceCreated(id string) (created bool)
}

// storageService is a proxy for the actual storage implementation
type storageService struct {
	ClientStorage
}

func New() ClientStorage {
	return &storageService{}
}

type configGetter interface {
	GetSpaceStorageMode() SpaceStorageMode
}

func (s *storageService) Name() (name string) {
	return spacestorage.CName
}

func (s *storageService) Init(a *app.App) (err error) {
	mode := a.MustComponent("config").(configGetter).GetSpaceStorageMode()
	if mode == SpaceStorageModeBadger {
		// for already existing account repos
		b := badgerstorage.New()
		fmt.Println(b.Name())
		s.ClientStorage = badgerstorage.New()
	} else if mode == SpaceStorageModeSqlite {
		// sqlite used for new account repos
		b := sqlitestorage.New()
		fmt.Println(b.Name())
		s.ClientStorage = sqlitestorage.New()
	} else {
		return fmt.Errorf("unknown storage mode %d", mode)
	}

	return s.ClientStorage.Init(a)
}
