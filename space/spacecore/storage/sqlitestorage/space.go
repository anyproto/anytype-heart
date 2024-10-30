package sqlitestorage

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/liststorage"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	_ "github.com/mattn/go-sqlite3"
)

const (
	treeTypeTree     = 0
	treeTypeList     = 1
	treeTypeSettings = 2
)

type spaceStorage struct {
	mu sync.RWMutex

	service *storageService

	spaceId         string
	spaceSettingsId string
	aclId           string
	hash            string
	oldHash         string

	isDeleted bool

	aclStorage liststorage.ListStorage

	header *spacesyncproto.RawSpaceHeaderWithId
}

func newSpaceStorage(s *storageService, spaceId string) (spacestorage.SpaceStorage, error) {
	ss := &spaceStorage{
		spaceId: spaceId,
		service: s,
		header: &spacesyncproto.RawSpaceHeaderWithId{
			Id: spaceId,
		},
	}
	err := s.stmt.loadSpace.QueryRow(spaceId).Scan(
		&ss.header.RawHeader,
		&ss.spaceSettingsId,
		&ss.aclId,
		&ss.hash,
		&ss.oldHash,
		&ss.isDeleted,
	)
	if err != nil {
		return nil, replaceNoRowsErr(err, spacestorage.ErrSpaceStorageMissing)
	}
	if ss.aclStorage, err = newListStorage(ss, ss.aclId); err != nil {
		return nil, err
	}
	return ss, nil
}

func createSpaceStorage(s *storageService, payload spacestorage.SpaceStorageCreatePayload) (spacestorage.SpaceStorage, error) {
	tx, err := s.writeDb.Begin()
	if err != nil {
		return nil, err
	}

	if _, err = tx.Stmt(s.stmt.createSpace).Exec(
		// space(id, header, settingsId, aclId)
		payload.SpaceHeaderWithId.Id,
		payload.SpaceHeaderWithId.RawHeader,
		payload.SpaceSettingsWithId.Id,
		payload.AclWithId.Id,
	); err != nil {
		_ = tx.Rollback()
		if isUniqueConstraint(err) {
			return nil, spacestorage.ErrSpaceStorageExists
		}
		return nil, err
	}

	if _, err = tx.Stmt(s.stmt.createTree).Exec(
		// settings tree (id, spaceId, heads, type)
		payload.SpaceSettingsWithId.Id,
		payload.SpaceHeaderWithId.Id,
		treestorage.CreateHeadsPayload([]string{payload.SpaceSettingsWithId.Id}),
		treeTypeSettings,
	); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if _, err = tx.Stmt(s.stmt.createTree).Exec(
		// acl list tree (id, spaceId, heads, type)
		payload.AclWithId.Id,
		payload.SpaceHeaderWithId.Id,
		payload.AclWithId.Id,
		treeTypeList,
	); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if _, err = tx.Stmt(s.stmt.createChange).Exec(
		// settings change (id, spaceId, treeId, data)
		payload.SpaceSettingsWithId.Id,
		payload.SpaceHeaderWithId.Id,
		payload.SpaceSettingsWithId.Id,
		payload.SpaceSettingsWithId.RawChange,
	); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if _, err = tx.Stmt(s.stmt.createChange).Exec(
		// acl change (id, spaceId, treeId, data)
		payload.AclWithId.Id,
		payload.SpaceHeaderWithId.Id,
		payload.AclWithId.Id,
		payload.AclWithId.Payload,
	); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	ss := &spaceStorage{
		spaceId:         payload.SpaceHeaderWithId.Id,
		service:         s,
		spaceSettingsId: payload.SpaceSettingsWithId.Id,
		header:          payload.SpaceHeaderWithId,
		aclId:           payload.AclWithId.Id,
	}

	if ss.aclStorage, err = newListStorage(ss, ss.aclId); err != nil {
		return nil, err
	}

	return ss, nil
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

func (s *spaceStorage) Id() string {
	return s.spaceId
}

func (s *spaceStorage) SetSpaceDeleted() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.service.stmt.updateSpaceIsDeleted.Exec(true, s.spaceId); err != nil {
		return err
	}
	s.isDeleted = true
	return nil
}

func (s *spaceStorage) IsSpaceDeleted() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isDeleted, nil
}

func (s *spaceStorage) SetTreeDeletedStatus(id, state string) error {
	_, err := s.service.stmt.updateTreeDelStatus.Exec(id, state, s.spaceId, state)
	return replaceNoRowsErr(err, ErrTreeNotFound)
}

func (s *spaceStorage) TreeDeletedStatus(id string) (status string, err error) {
	var nullString sql.NullString
	if err = s.service.stmt.treeDelStatus.QueryRow(id).Scan(&nullString); err != nil {
		err = replaceNoRowsErr(err, nil)
		return "", err
	}
	return nullString.String, nil
}

func (s *spaceStorage) AllDeletedTreeIds() (ids []string, err error) {
	rows, err := s.service.stmt.allTreeDelStatus.Query(s.spaceId, spacestorage.TreeDeletedStatusDeleted)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		err = errors.Join(err, rows.Close())
	}()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *spaceStorage) SpaceSettingsId() string {
	return s.spaceSettingsId
}

func (s *spaceStorage) AclStorage() (liststorage.ListStorage, error) {
	return s.aclStorage, nil
}

func (s *spaceStorage) SpaceHeader() (*spacesyncproto.RawSpaceHeaderWithId, error) {
	return s.header, nil
}

func (s *spaceStorage) StoredIds() (result []string, err error) {
	rows, err := s.service.stmt.treeIdsBySpace.Query(s.spaceId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		err = errors.Join(err, rows.Close())
	}()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return
}

func (s *spaceStorage) TreeRoot(id string) (*treechangeproto.RawTreeChangeWithId, error) {
	var data []byte
	if err := s.service.stmt.change.QueryRow(id, s.spaceId).Scan(&data); err != nil {
		return nil, replaceNoRowsErr(err, ErrTreeNotFound)
	}
	return &treechangeproto.RawTreeChangeWithId{
		RawChange: data,
		Id:        id,
	}, nil
}

func (s *spaceStorage) TreeStorage(id string) (treestorage.TreeStorage, error) {
	return newTreeStorage(s, id)
}

func (s *spaceStorage) HasTree(id string) (bool, error) {
	var res int
	if err := s.service.stmt.hasTree.QueryRow(id, s.spaceId).Scan(&res); err != nil {
		err = replaceNoRowsErr(err, nil)
		return false, err
	}
	return res > 0, nil
}

func (s *spaceStorage) CreateTreeStorage(payload treestorage.TreeStorageCreatePayload) (treestorage.TreeStorage, error) {
	return createTreeStorage(s, payload)
}

func (s *spaceStorage) WriteSpaceHash(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.service.stmt.updateSpaceHash.Exec(hash, s.spaceId); err != nil {
		return err
	}
	s.hash = hash
	return nil
}

func (s *spaceStorage) ReadSpaceHash() (hash string, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hash, nil
}

func (s *spaceStorage) Close(_ context.Context) (err error) {
	s.service.unlockSpaceStorage(s.spaceId)
	return nil
}
