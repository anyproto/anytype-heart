package oldstorage

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"

	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/badgerstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/sqlitestorage"
)

type SpaceStorageMode int

const CName = "client.spacecore.oldstorage"

const (
	SpaceStorageModeSqlite SpaceStorageMode = iota // used for new account repos
	SpaceStorageModeBadger                         // used for existing account repos
)

type ClientStorage interface {
	oldstorage.SpaceStorageProvider
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
	GetSpaceStorageMode() storage.SpaceStorageMode
}

func (s *storageService) Name() (name string) {
	return CName
}

func (s *storageService) Init(a *app.App) (err error) {
	mode := a.MustComponent("config").(configGetter).GetSpaceStorageMode()
	if mode == storage.SpaceStorageModeBadger {
		// for already existing account repos
		s.ClientStorage = badgerstorage.New()
	} else if mode == storage.SpaceStorageModeSqlite {
		// sqlite used for new account repos
		s.ClientStorage = sqlitestorage.New()
	} else {
		return fmt.Errorf("unknown storage mode %d", mode)
	}

	return s.ClientStorage.Init(a)
}
