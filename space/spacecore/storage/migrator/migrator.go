package migrator

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/any-sync/commonspace/spacestorage/migration"

	"github.com/anyproto/anytype-heart/space/spacecore/oldstorage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

const CName = "client.storage.migration"

type migrator struct {
	storage    oldstorage.ClientStorage
	newStorage storage.ClientStorage
}

func New() app.ComponentRunnable {
	return &migrator{}
}

func (m *migrator) Init(a *app.App) (err error) {
	m.storage = app.MustComponent[oldstorage.ClientStorage](a)
	m.newStorage = app.MustComponent[storage.ClientStorage](a)
	return nil
}

func (m *migrator) Name() (name string) {
	return CName
}

func (m *migrator) Run(ctx context.Context) (err error) {
	migrator := migration.NewSpaceMigrator(m.storage, m.newStorage, 10)
	allIds, err := m.storage.AllSpaceIds()
	if err != nil {
		return err
	}
	for _, id := range allIds {
		st, err := migrator.MigrateId(ctx, id)
		if err != nil {
			return err
		}
		err = st.Close(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *migrator) Close(ctx context.Context) (err error) {
	return nil
}
