package anystorehelper

import (
	"context"
	"errors"
	"os"
	"runtime"
	"slices"
	"strings"

	anystore "github.com/anyproto/any-store"
	"go.uber.org/zap"
	"zombiezen.com/go/sqlite"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("objectstore.spaceindex")

// ApplyPlatformPragmas adds platform-specific SQLite connection pragmas.
// On Apple platforms plain fsync() does not flush the drive cache, so a WAL
// checkpoint whose DB sync silently didn't reach stable storage can corrupt the
// main DB file on power loss ("the only time that a failed sync operation can
// cause database corruption is during a checkpoint operation", sqlite.org/wal.html).
// fullfsync=1 makes SQLite use F_FULLFSYNC for all syncs there.
//
// Intended only for source-of-truth dbs (space stores, which may hold the sole
// copy of not-yet-synced data). Derived dbs (object index, crdt state) skip it:
// they are rebuilt by reindex/replay on corruption and checkpoint too often to
// pay F_FULLFSYNC on every one.
func ApplyPlatformPragmas(opts map[string]string) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		opts["fullfsync"] = "1"
	}
}

func IsCorruptedError(err error) (code sqlite.ResultCode, isCorrupted bool) {
	code = sqlite.ErrCode(err)
	isCorrupted = errors.Is(err, anystore.ErrQuickCheckFailed) || errors.Is(err, anystore.ErrIncompatibleVersion) || code == sqlite.ResultCorrupt || code == sqlite.ResultNotADB || code == sqlite.ResultCantOpen
	return
}

func DbStatToZapFields(stat anystore.DBStats) []zap.Field {
	return []zap.Field{
		zap.Bool("dirtyOnOpen", stat.DirtyOnOpen),
		zap.Int64("quickCheckMs", stat.DirtyQuickCheckDuration.Milliseconds()),
		zap.Int64("totalSize", int64(stat.TotalSizeBytes)),
		zap.Int64("dataSize", int64(stat.DataSizeBytes)),
		zap.Int64("indexes", int64(stat.IndexesCount)),
		zap.Int64("collections", int64(stat.CollectionsCount)),
	}
}

func RemoveSqliteFiles(dbPath string) error {
	paths := []string{
		dbPath,
		dbPath + "-shm",
		dbPath + "-wal",
		dbPath + ".lock",
	}
	for _, path := range paths {
		err := os.Remove(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
	}

	return nil
}

func AddIndexes(ctx context.Context, coll anystore.Collection, indexes []anystore.IndexInfo) error {
	gotIndexes := coll.GetIndexes()
	toCreate := indexes[:0]
	var toDrop []string
	for i, idx := range indexes {
		if idx.Name == "" {
			idx.Name = strings.Join(idx.Fields, ",")
			indexes[i].Name = idx.Name
		}
		if !slices.ContainsFunc(gotIndexes, func(i anystore.Index) bool {
			return i.Info().Name == idx.Name
		}) {
			toCreate = append(toCreate, idx)
		}
	}
	for _, idx := range gotIndexes {
		if !slices.ContainsFunc(indexes, func(i anystore.IndexInfo) bool {
			return i.Name == idx.Info().Name
		}) {
			toDrop = append(toDrop, idx.Info().Name)
		}
	}
	if len(toDrop) > 0 {
		for _, indexName := range toDrop {
			if err := coll.DropIndex(ctx, indexName); err != nil {
				return err
			}
		}
	}
	if len(toCreate) > 0 {
		coll.GetIndexes()
		return coll.EnsureIndex(ctx, toCreate...)
	}
	return nil
}
