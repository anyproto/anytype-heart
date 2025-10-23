package anystorehelper

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"

	anystore "github.com/anyproto/any-store"
	"go.uber.org/zap"
	"zombiezen.com/go/sqlite"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("objectstore.spaceindex")

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
