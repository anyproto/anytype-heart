package anystorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/space/spacedomain"
)

const backupSuffix = "_backup"

// CorruptedBackup represents a backup of a corrupted space storage
type CorruptedBackup struct {
	SpaceId    string
	BackupPath string
}

// nolint: unused
var log = logger.NewNamed(spacestorage.CName)

func New(rootPath string, anyStoreConfig *anystore.Config) *storageService {
	return &storageService{
		rootPath: rootPath,
		config:   anyStoreConfig,
	}
}

type storageService struct {
	rootPath string
	config   *anystore.Config
	sync.Mutex
}

func (s *storageService) AllSpaceIds() (ids []string, err error) {
	var files []string
	fileInfo, err := os.ReadDir(s.rootPath)
	if err != nil {
		return files, fmt.Errorf("can't read datadir '%v': %w", s.rootPath, err)
	}
	for _, file := range fileInfo {
		if !strings.HasPrefix(file.Name(), ".") {
			files = append(files, file.Name())
		}
	}
	return files, nil
}

func (s *storageService) Run(ctx context.Context) (err error) {
	return nil
}

func (s *storageService) openDb(ctx context.Context, id string) (db anystore.DB, err error) {
	dbPath := path.Join(s.rootPath, id, "store.db")
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, spacestorage.ErrSpaceStorageMissing
		}
		return nil, err
	}

	db, err = anystore.Open(ctx, dbPath, s.anyStoreConfig())
	if err != nil {
		code, isCorrupted := anystorehelper.IsCorruptedError(err)
		if isCorrupted {
			log.With(zap.String("spaceId", id), zap.String("code", code.String())).
				Warn("space store corrupted, backing up")
			if backupPath, backupErr := s.backupCorruptedSpace(id); backupErr != nil {
				log.With(zap.Error(backupErr)).Error("failed to backup corrupted space")
			} else {
				log.With(zap.String("backupPath", backupPath)).Warn("corrupted space backed up")
			}
			return nil, spacestorage.ErrSpaceStorageMissing
		}
		return nil, err
	}
	return db, nil
}

func (s *storageService) createDb(ctx context.Context, id string) (db anystore.DB, err error) {
	dirPath := path.Join(s.rootPath, id)
	err = os.MkdirAll(dirPath, 0755)
	if err != nil {
		return nil, err
	}
	dbPath := path.Join(dirPath, "store.db")
	return anystore.Open(ctx, dbPath, s.anyStoreConfig())
}

func (s *storageService) Close(ctx context.Context) (err error) {
	return nil
}

func (s *storageService) Init(a *app.App) (err error) {
	if _, err = os.Stat(s.rootPath); err != nil {
		err = os.MkdirAll(s.rootPath, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *storageService) Name() (name string) {
	return spacestorage.CName
}

func (s *storageService) WaitSpaceStorage(ctx context.Context, id string) (spacestorage.SpaceStorage, error) {
	start := time.Now()
	db, err := s.openDb(ctx, id)
	if err != nil {
		code, isCorrupted := anystorehelper.IsCorruptedError(err)
		log.With(zap.Bool("isCorrupted", isCorrupted), zap.String("code", code.String()), zap.Error(err)).Error("failed to open spacestore")
		return nil, err
	}
	if time.Since(start) > time.Second {
		ctxStat, cancel := context.WithTimeout(ctx, time.Second*2)
		defer cancel()

		logger := log.With(zap.String("spaceId", id)).With(zap.Int64("tookMs", time.Since(start).Milliseconds()))
		stat, err := db.Stats(ctxStat)
		if err != nil {
			logger = logger.With(zap.Error(err))
		} else {
			logger = logger.With(anystorehelper.DbStatToZapFields(stat)...)
		}
		logger.Warn("spacestore db open took too long")
	}
	st, err := spacestorage.New(ctx, id, db)
	if err != nil {
		return nil, err
	}
	return NewClientStorage(ctx, st)
}

func (s *storageService) SpaceExists(id string) bool {
	if id == "" {
		return false
	}
	dbPath := path.Join(s.rootPath, id)
	if _, err := os.Stat(dbPath); err != nil {
		return false
	}
	return true
}

func (s *storageService) CreateSpaceStorage(ctx context.Context, payload spacestorage.SpaceStorageCreatePayload) (spacestorage.SpaceStorage, error) {
	if err := validateSpaceType(payload.SpaceHeaderWithId); err != nil {
		return nil, err
	}
	db, err := s.createDb(ctx, payload.SpaceHeaderWithId.Id)
	if err != nil {
		err = fmt.Errorf("failed to create db: %w", err)
		return nil, err
	}
	st, err := spacestorage.Create(ctx, db, payload)
	if err != nil {
		err = fmt.Errorf("failed to create spacestorage: %w", err)
		return nil, err
	}
	return NewClientStorage(ctx, st)
}

func (s *storageService) DeleteSpaceStorage(ctx context.Context, spaceId string) error {
	dbPath := path.Join(s.rootPath, spaceId)
	return os.RemoveAll(dbPath)
}

func (s *storageService) anyStoreConfig() *anystore.Config {
	s.Lock()
	defer s.Unlock()
	opts := maps.Clone(s.config.SQLiteConnectionOptions)
	if opts == nil {
		opts = make(map[string]string)
	}
	opts["synchronous"] = "off"
	return &anystore.Config{
		ReadConnections:                           4,
		SQLiteConnectionOptions:                   opts,
		SQLiteGlobalPageCachePreallocateSizeBytes: 1 << 26,

		StalledConnectionsPanicOnClose:    time.Second * 45,
		StalledConnectionsDetectorEnabled: true,
		Durability: anystore.DurabilityConfig{
			AutoFlush: true,
			IdleAfter: time.Second * 20,
			FlushMode: anystore.FlushModeCheckpointPassive,
			Sentinel:  true,
		},
	}
}

func validateSpaceType(headerWithId *spacesyncproto.RawSpaceHeaderWithId) error {
	var rawHeader = &spacesyncproto.RawSpaceHeader{}
	if err := rawHeader.UnmarshalVT(headerWithId.RawHeader); err != nil {
		return err
	}

	var header = &spacesyncproto.SpaceHeader{}
	if err := header.UnmarshalVT(rawHeader.SpaceHeader); err != nil {
		return err
	}

	switch spacedomain.SpaceType(header.SpaceType) {
	case "":
	case spacedomain.SpaceTypeTech:
	case spacedomain.SpaceTypeRegular:
	case spacedomain.SpaceTypeChat:
	case spacedomain.SpaceTypeOneToOne:
	default:
		return fmt.Errorf("%w: type: %v", spacedomain.ErrUnexpectedSpaceType, header.SpaceType)
	}
	return nil
}

// backupCorruptedSpace renames the corrupted space folder to {spaceId}_backup_{timestamp}
func (s *storageService) backupCorruptedSpace(spaceId string) (string, error) {
	srcPath := filepath.Join(s.rootPath, spaceId)
	timestamp := time.Now().Unix()
	backupPath := filepath.Join(s.rootPath, fmt.Sprintf("%s%s%d", spaceId, backupSuffix, timestamp))

	if err := os.Rename(srcPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to rename corrupted space: %w", err)
	}
	return backupPath, nil
}

// ListCorruptedBackups returns all corrupted backup folders in the storage root
// Backup folders are named {spaceId}_backup_{timestamp}
func (s *storageService) ListCorruptedBackups() ([]CorruptedBackup, error) {
	entries, err := os.ReadDir(s.rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read storage root: %w", err)
	}

	var backups []CorruptedBackup
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Find _backup_ in the folder name
		idx := strings.Index(entry.Name(), backupSuffix)
		if idx == -1 {
			continue
		}
		spaceId := entry.Name()[:idx]
		if spaceId == "" {
			continue
		}
		backups = append(backups, CorruptedBackup{
			SpaceId:    spaceId,
			BackupPath: filepath.Join(s.rootPath, entry.Name()),
		})
	}
	return backups, nil
}

// DeleteBackup removes a corrupted backup folder after validating the path
func (s *storageService) DeleteBackup(backupPath string) error {
	absRoot, err := filepath.Abs(s.rootPath)
	if err != nil {
		return fmt.Errorf("failed to resolve root path: %w", err)
	}
	absBackup, err := filepath.Abs(backupPath)
	if err != nil {
		return fmt.Errorf("failed to resolve backup path: %w", err)
	}

	// Security: ensure backup path is within storage root
	if !strings.HasPrefix(absBackup, absRoot+string(filepath.Separator)) {
		return fmt.Errorf("invalid backup path: outside storage root")
	}

	// Ensure this is actually a backup folder (contains _backup_ pattern)
	if !strings.Contains(filepath.Base(backupPath), backupSuffix) {
		return fmt.Errorf("invalid backup path: not a backup folder")
	}

	return os.RemoveAll(backupPath)
}
