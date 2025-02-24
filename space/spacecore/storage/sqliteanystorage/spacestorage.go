package sqliteanystorage

import (
	"context"
	"database/sql"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/headsync/statestorage"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/consensus/consensusproto"
)

func Create(ctx context.Context, readDb, writeDb *sql.DB, payload spacestorage.SpaceStorageCreatePayload) (spacestorage.SpaceStorage, error) {
	spaceId := payload.SpaceHeaderWithId.Id
	state := statestorage.State{
		AclId:       payload.AclWithId.Id,
		SettingsId:  payload.SpaceSettingsWithId.Id,
		SpaceId:     payload.SpaceHeaderWithId.Id,
		SpaceHeader: payload.SpaceHeaderWithId.RawHeader,
	}
	err := createChangesTable(writeDb, spaceId)
	if err != nil {
		return nil, err
	}
	stateStorage, err := CreateStateStorage(ctx, state, readDb, writeDb)
	if err != nil {
		return nil, err
	}
	headStorage, err := NewHeadStorage(readDb, writeDb, spaceId)
	if err != nil {
		return nil, err
	}
	aclStorage, err := CreateListStorage(ctx, &consensusproto.RawRecordWithId{
		Payload: payload.AclWithId.Payload,
		Id:      payload.AclWithId.Id,
	}, headStorage, readDb, writeDb)
	if err != nil {
		return nil, err
	}
	_, err = CreateTreeStorage(ctx, &treechangeproto.RawTreeChangeWithId{
		RawChange: payload.SpaceSettingsWithId.RawChange,
		Id:        payload.SpaceSettingsWithId.Id,
	}, headStorage, readDb, writeDb)
	if err != nil {
		return nil, err
	}
	return &spaceStorage{
		store:        nil,
		spaceId:      spaceId,
		headStorage:  headStorage,
		stateStorage: stateStorage,
		aclStorage:   aclStorage,
		readDb:       readDb,
		writeDb:      writeDb,
	}, nil
}

func New(ctx context.Context, spaceId string, readDb, writeDb *sql.DB) (spacestorage.SpaceStorage, error) {
	s := &spaceStorage{
		spaceId: spaceId,
		readDb:  readDb,
		writeDb: writeDb,
	}
	var err error
	s.headStorage, err = NewHeadStorage(readDb, writeDb, s.spaceId)
	if err != nil {
		return nil, err
	}
	s.stateStorage, err = NewStateStorage(ctx, s.spaceId, readDb, writeDb)
	if err != nil {
		return nil, err
	}
	state, err := s.stateStorage.GetState(ctx)
	if err != nil {
		return nil, err
	}
	s.aclStorage, err = NewListStorage(ctx, state.AclId, s.headStorage, readDb, writeDb)
	if err != nil {
		return nil, err
	}
	return s, nil
}

type spaceStorage struct {
	spaceId      string
	headStorage  headstorage.HeadStorage
	stateStorage statestorage.StateStorage
	aclStorage   list.Storage
	store        anystore.DB
	readDb       *sql.DB
	writeDb      *sql.DB
}

func (s *spaceStorage) Init(a *app.App) (err error) {
	return nil
}

func (s *spaceStorage) Name() (name string) {
	return spacestorage.CName
}

func (s *spaceStorage) Run(ctx context.Context) (err error) {
	return nil
}

func (s *spaceStorage) Close(ctx context.Context) (err error) {
	return nil
}

func (s *spaceStorage) Id() string {
	return s.spaceId
}

func (s *spaceStorage) HeadStorage() headstorage.HeadStorage {
	return s.headStorage
}

func (s *spaceStorage) StateStorage() statestorage.StateStorage {
	return s.stateStorage
}

func (s *spaceStorage) AclStorage() (list.Storage, error) {
	return s.aclStorage, nil
}

func (s *spaceStorage) TreeStorage(ctx context.Context, id string) (objecttree.Storage, error) {
	return NewTreeStorage(ctx, id, s.headStorage, s.readDb, s.writeDb)
}

func (s *spaceStorage) CreateTreeStorage(ctx context.Context, payload treestorage.TreeStorageCreatePayload) (objecttree.Storage, error) {
	return CreateTreeStorage(ctx, payload.RootRawChange, s.headStorage, s.readDb, s.writeDb)
}

func (s *spaceStorage) AnyStore() anystore.DB {
	return nil
}
