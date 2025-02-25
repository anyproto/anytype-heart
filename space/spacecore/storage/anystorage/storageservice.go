package anystorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/globalsign/mgo/bson"
	"github.com/mattn/go-sqlite3"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spacecore/storage/sqliteanystorage"
)

// nolint: unused
var log = logger.NewNamed(spacestorage.CName)

func New(rootPath string) *storageService {
	return &storageService{
		rootPath: rootPath,
	}
}

type storageService struct {
	rootPath             string
	dbPath               string
	store                anystore.DB
	readDb               *sql.DB
	writeDb              *sql.DB
	checkpointAfterWrite time.Duration
	checkpointForce      time.Duration
	lastWrite            atomic.Time
	lastCheckpoint       atomic.Time
	stmt                 struct {
		allSpaces *sql.Stmt
	}
	mu        sync.Mutex
	ctx       context.Context
	ctxCancel context.CancelFunc
}

func (s *storageService) AllSpaceIds() (ids []string, err error) {
	rows, err := s.readDb.QueryContext(context.Background(), `
		SELECT name FROM sqlite_master 
		WHERE type = 'table'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var collName string
		if err := rows.Scan(&collName); err != nil {
			return nil, err
		}
		if strings.Contains(collName, "changes") {
			parts := strings.Split(collName, "_")
			if len(parts) > 0 {
				ids = append(ids, parts[0])
			}
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
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

func (s *storageService) Run(ctx context.Context) (err error) {
	driverName := bson.NewObjectId().Hex()
	sql.Register(driverName,
		&sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				// if s.dbTempPath != "" {
				// 	_, err := conn.Exec("PRAGMA temp_store_directory = '"+s.dbTempPath+"';", nil)
				// 	if err != nil {
				// 		return err
				// 	}
				// }
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

	if s.readDb, err = sql.Open(driverName, connectionUri); err != nil {
		log.With(zap.String("db", "spacestore_sqlite"), zap.String("type", "read"), zap.Error(err)).Error("failed to open db")
		return
	}
	s.readDb.SetMaxOpenConns(10)

	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	go s.checkpointLoop()
	return
}

func (s *storageService) openDb(ctx context.Context, id string) (db anystore.DB, err error) {
	return s.store, nil
}

func (s *storageService) createDb(ctx context.Context, id string) (db anystore.DB, err error) {
	return s.store, nil
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

func (s *storageService) Init(a *app.App) (err error) {
	if _, err = os.Stat(s.rootPath); err != nil {
		err = os.MkdirAll(s.rootPath, 0755)
		if err != nil {
			return err
		}
	}
	s.dbPath = filepath.Join(s.rootPath, "spaceStore.db")
	if s.checkpointAfterWrite == 0 {
		s.checkpointAfterWrite = time.Second
	}
	if s.checkpointForce == 0 {
		s.checkpointForce = time.Minute
	}
	return
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

func (s *storageService) Name() (name string) {
	return spacestorage.CName
}

func (s *storageService) WaitSpaceStorage(ctx context.Context, id string) (spacestorage.SpaceStorage, error) {
	st, err := sqliteanystorage.New(ctx, id, s.readDb, s.writeDb)
	if err != nil {
		fmt.Println("[x] storage wait error: ", err)
		return nil, spacestorage.ErrSpaceStorageMissing
	}
	return NewClientStorage(ctx, st)
}

func (s *storageService) SpaceExists(id string) bool {
	panic("implement me")
}

func (s *storageService) CreateSpaceStorage(ctx context.Context, payload spacestorage.SpaceStorageCreatePayload) (spacestorage.SpaceStorage, error) {
	spaceStorage, err := sqliteanystorage.Create(ctx, s.readDb, s.writeDb, payload)
	if err != nil {
		return nil, err
	}
	return NewClientStorage(ctx, spaceStorage)
}

func (s *storageService) DeleteSpaceStorage(ctx context.Context, spaceId string) error {
	return nil
}
