package sqlitestorage

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/mattn/go-sqlite3"
)

var ErrLocked = errors.New("space storage locked")

type configGetter interface {
	GetSpaceStorePath() string
}

type storageService struct {
	db   *sql.DB
	stmt struct {
		createSpace,
		createTree,
		createChange,
		updateSpaceHash,
		updateSpaceOldHash,
		updateSpaceIsCreated,
		updateSpaceIsDeleted,
		treeIdsBySpace,
		updateTreeDelStatus,
		treeDelStatus,
		change,
		hasTree,
		hasChange,
		updateTreeHeads,
		deleteTree,
		deleteChangesByTree,
		loadTreeHeads,
		loadSpace,
		spaceIds,
		spaceIsCreated,
		upsertBind,
		deleteSpace,
		deleteTreesBySpace,
		deleteChangesBySpace,
		deleteBindsBySpace,
		getBind *sql.Stmt
	}
	dbPath       string
	lockedSpaces map[string]*lockSpace
	mu           sync.Mutex
}

type lockSpace struct {
	ch  chan struct{}
	err error
}

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

func New() ClientStorage {
	return &storageService{}
}

func (s *storageService) Init(a *app.App) (err error) {
	s.dbPath = a.MustComponent("config").(configGetter).GetSpaceStorePath()
	s.lockedSpaces = map[string]*lockSpace{}
	return
}

func (s *storageService) Run(ctx context.Context) (err error) {
	if s.db, err = sql.Open("sqlite3", s.dbPath); err != nil {
		return
	}
	if _, err = s.db.Exec(sqlCreateTables); err != nil {
		return
	}

	return initStmts(s)
}

func (s *storageService) Name() (name string) {
	return spacestorage.CName
}

func (s *storageService) SpaceStorage(id string) (spacestorage.SpaceStorage, error) {
	return newSpaceStorage(s, id)
}

func (s *storageService) WaitSpaceStorage(ctx context.Context, id string) (store spacestorage.SpaceStorage, err error) {
	var ls *lockSpace
	ls, err = s.checkLock(id, func() error {
		store, err = newSpaceStorage(s, id)
		return err
	})
	if errors.Is(err, ErrLocked) {
		select {
		case <-ls.ch:
			if ls.err != nil {
				return nil, err
			}
			return s.WaitSpaceStorage(ctx, id)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return
}

func (s *storageService) MarkSpaceCreated(id string) (err error) {
	_, err = s.stmt.updateSpaceIsCreated.Exec(true, id)
	return
}

func (s *storageService) UnmarkSpaceCreated(id string) (err error) {
	_, err = s.stmt.updateSpaceIsCreated.Exec(false, id)
	return
}

func (s *storageService) IsSpaceCreated(id string) (created bool) {
	_ = s.stmt.spaceIsCreated.QueryRow(id).Scan(&created)
	return
}

func (s *storageService) SpaceExists(id string) bool {
	var created bool
	err := s.stmt.spaceIsCreated.QueryRow(id).Scan(&created)
	return err == nil
}

func (s *storageService) checkLock(id string, openFunc func() error) (ls *lockSpace, err error) {
	s.mu.Lock()
	var ok bool
	if ls, ok = s.lockedSpaces[id]; ok {
		s.mu.Unlock()
		return ls, ErrLocked
	}
	ch := make(chan struct{})
	ls = &lockSpace{
		ch: ch,
	}
	s.lockedSpaces[id] = ls
	s.mu.Unlock()
	if err = openFunc(); err != nil {
		s.unlockSpaceStorage(id)
		return nil, err
	}
	return nil, nil
}

func (s *storageService) waitLock(ctx context.Context, id string, action func() error) (err error) {
	var ls *lockSpace
	ls, err = s.checkLock(id, action)
	if errors.Is(err, ErrLocked) {
		select {
		case <-ls.ch:
			if ls.err != nil {
				return ls.err
			}
			return s.waitLock(ctx, id, action)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return
}

func (s *storageService) DeleteSpaceStorage(ctx context.Context, spaceId string) error {
	err := s.waitLock(ctx, spaceId, func() error {
		return s.deleteSpace(spaceId)
	})
	if err == nil {
		s.unlockSpaceStorage(spaceId)
	}
	return err
}

func (s *storageService) deleteSpace(spaceId string) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	for _, stmt := range []*sql.Stmt{
		s.stmt.deleteSpace,
		s.stmt.deleteChangesBySpace,
		s.stmt.deleteBindsBySpace,
		s.stmt.deleteTreesBySpace,
	} {
		if _, err = tx.Stmt(stmt).Exec(spaceId); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (s *storageService) unlockSpaceStorage(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ls, ok := s.lockedSpaces[id]; ok {
		close(ls.ch)
		delete(s.lockedSpaces, id)
	}
}

func (s *storageService) CreateSpaceStorage(payload spacestorage.SpaceStorageCreatePayload) (spacestorage.SpaceStorage, error) {
	return createSpaceStorage(s, payload)
}

func (s *storageService) GetSpaceID(objectID string) (spaceID string, err error) {
	err = s.stmt.getBind.QueryRow(objectID).Scan(&spaceID)
	return
}

func (s *storageService) BindSpaceID(spaceID, objectID string) (err error) {
	_, err = s.stmt.upsertBind.Exec(objectID, spaceID, spaceID)
	return
}

func (s *storageService) AllSpaceIds() (ids []string, err error) {
	rows, err := s.stmt.spaceIds.Query()
	if err != nil {
		return nil, err
	}
	defer func() {
		err = rows.Close()
	}()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return
		}
		ids = append(ids, id)
	}
	return
}

func (s *storageService) Close(ctx context.Context) (err error) {
	if s.db != nil {
		return s.db.Close()
	}
	return
}

func isUniqueConstraint(err error) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		if errors.Is(sqliteErr.Code, sqlite3.ErrConstraint) {
			return true
		}
	}
	return false
}
