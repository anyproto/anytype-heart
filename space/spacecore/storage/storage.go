package storage

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"

	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage"
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
	DeleteSpaceStorage(ctx context.Context, spaceId string) error
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
	s.ClientStorage = anystorage.New()
	return s.ClientStorage.Init(a)
}
