package sqlitestorage

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"github.com/globalsign/mgo/bson"
	"github.com/mattn/go-sqlite3"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
)

var ErrLocked = errors.New("space storage locked")

var log = logger.NewNamed("sqlitestore")

var (
	ErrSpaceNotFound = errors.New("space not found")
	ErrTreeNotFound  = treestorage.ErrUnknownTreeId
)

type configGetter interface {
	GetSpaceStorePath() string
	GetTempDirPath() string
}

type storageService struct {
	writeDb *sql.DB
	readDb  *sql.DB
	stmt    struct {
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
		allTreeDelStatus,
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
	dbTempPath   string
	lockedSpaces map[string]*lockSpace

	ctx       context.Context
	ctxCancel context.CancelFunc

	checkpointAfterWrite time.Duration
	checkpointForce      time.Duration
	lastWrite            atomic.Time
	lastCheckpoint       atomic.Time
	mu                   sync.Mutex
}

type lockSpace struct {
	ch chan struct{}

	err error
}

func New() *storageService {
	return &storageService{}
}

func (s *storageService) Init(a *app.App) (err error) {
	s.dbPath = a.MustComponent("config").(configGetter).GetSpaceStorePath()
	s.dbTempPath = a.MustComponent("config").(configGetter).GetTempDirPath()
	s.lockedSpaces = map[string]*lockSpace{}
	if s.checkpointAfterWrite == 0 {
		s.checkpointAfterWrite = time.Second
	}
	if s.checkpointForce == 0 {
		s.checkpointForce = time.Minute
	}
	return
}

func (s *storageService) Run(ctx context.Context) (err error) {
	driverName := bson.NewObjectId().Hex()
	sql.Register(driverName,
		&sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if s.dbTempPath != "" {
					_, err := conn.Exec("PRAGMA temp_store_directory = '"+s.dbTempPath+"';", nil)
					if err != nil {
						return err
					}
				}
				conn.RegisterUpdateHook(func(op int, db string, table string, rowid int64) {
					s.lastWrite.Store(time.Now())
				})
				return nil
			},
		})

	connectionUrlParams := make(url.Values)
	connectionUrlParams.Add("_txlock", "immediate")
	connectionUrlParams.Add("_journal_mode", "WAL")
	connectionUrlParams.Add("_busy_timeout", "5000")
	connectionUrlParams.Add("_synchronous", "NORMAL")
	connectionUrlParams.Add("_cache_size", "10000000")
	connectionUrlParams.Add("_foreign_keys", "true")
	connectionUri := s.dbPath + "?" + connectionUrlParams.Encode()
	if s.writeDb, err = sql.Open(driverName, connectionUri); err != nil {
		log.With(zap.String("db", "spacestore_sqlite"), zap.String("type", "write"), zap.Error(err)).Error("failed to open db")
		return
	}
	s.writeDb.SetMaxOpenConns(1)

	if _, err = s.writeDb.Exec(sqlCreateTables); err != nil {
		log.With(zap.String("db", "spacestore_sqlite"), zap.String("type", "createtable"), zap.Error(err)).Error("failed to open db")
		return
	}

	if s.readDb, err = sql.Open(driverName, connectionUri); err != nil {
		log.With(zap.String("db", "spacestore_sqlite"), zap.String("type", "read"), zap.Error(err)).Error("failed to open db")
		return
	}
	s.readDb.SetMaxOpenConns(10)

	if err = initStmts(s); err != nil {
		log.With(zap.String("db", "spacestore_sqlite"), zap.String("type", "init"), zap.Error(err)).Error("failed to open db")
		return
	}
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	go s.checkpointLoop()
	return
}

func (s *storageService) Name() (name string) {
	return spacestorage.CName
}

func (s *storageService) WaitSpaceStorage(ctx context.Context, id string) (store oldstorage.SpaceStorage, err error) {
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
	return replaceNoRowsErr(err, ErrSpaceNotFound)
}

func (s *storageService) UnmarkSpaceCreated(id string) (err error) {
	_, err = s.stmt.updateSpaceIsCreated.Exec(false, id)
	return replaceNoRowsErr(err, ErrSpaceNotFound)
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
	tx, err := s.writeDb.Begin()
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

func (s *storageService) CreateSpaceStorage(payload spacestorage.SpaceStorageCreatePayload) (ss oldstorage.SpaceStorage, err error) {
	_, err = s.checkLock(payload.SpaceHeaderWithId.Id, func() error {
		ss, err = createSpaceStorage(s, payload)
		return err
	})
	return
}

func (s *storageService) GetSpaceID(objectID string) (spaceID string, err error) {
	err = s.stmt.getBind.QueryRow(objectID).Scan(&spaceID)
	err = replaceNoRowsErr(err, domain.ErrObjectNotFound)
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
		err = errors.Join(rows.Close())
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

func (s *storageService) checkpointLoop() {
	for {
		select {
		case <-time.After(s.checkpointAfterWrite):
		case <-s.ctx.Done():
			return
		}
		if s.needCheckpoint() {
			st := time.Now()
			if err := s.checkpoint(); err != nil {
				log.Warn("checkpoint error", zap.Error(err))
			}
			log.Debug("checkpoint", zap.Duration("dur", time.Since(st)))
		}
	}
}

func (s *storageService) needCheckpoint() bool {
	now := time.Now()
	lastWrite := s.lastWrite.Load()
	lastCheckpoint := s.lastCheckpoint.Load()

	if lastCheckpoint.Before(lastWrite) && now.Sub(lastWrite) > s.checkpointAfterWrite {
		return true
	}

	if now.Sub(lastCheckpoint) > s.checkpointForce {
		return true
	}
	return false
}

func (s *storageService) checkpoint() (err error) {
	_, err = s.writeDb.ExecContext(s.ctx, `PRAGMA wal_checkpoint(PASSIVE)`)
	s.lastCheckpoint.Store(time.Now())
	return err
}

func (s *storageService) Close(ctx context.Context) (err error) {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	if s.writeDb != nil {
		err = errors.Join(err, s.writeDb.Close())
	}
	if s.readDb != nil {
		err = errors.Join(err, s.readDb.Close())
	}
	return
}

func isUniqueConstraint(err error) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && errors.Is(sqliteErr.Code, sqlite3.ErrConstraint) {
		return true
	}
	return false
}

func replaceNoRowsErr(err, rErr error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return rErr
	}
	return err
}
