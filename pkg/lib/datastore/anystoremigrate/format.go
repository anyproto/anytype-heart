package anystoremigrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	anystore "github.com/anyproto/any-store/v2"
	"go.uber.org/zap"
)

// formatTempSuffix names the file a format upgrade copies a database into.
const formatTempSuffix = ".format-upgrade"

// UpgradeFormat brings the v2 database db, open at path, onto anystore.LatestSchemaFormat, waiting
// for its turn, and returns the database to use from then on. A database already in that format is
// returned as it is. config is the configuration path is opened with.
//
// A database the caller rebuilds from elsewhere (reset) is replaced by an empty one. Any other is
// copied with anystore.CopyToLatestFormat, and the copy takes its name, as any-store's
// docs/migration-format-5-to-6.md lays out.
//
// The caller holds LockPath(path); db is the only handle on path, and nothing writes through it
// until UpgradeFormat returns.
//
// A canceled ctx stops the upgrade at the next of its checks: before it starts, after the copy,
// after the database is checkpointed, and after it is closed. Stopped by the last one, it returns
// nil with the files untouched: the old database, checkpointed, for the next open. Past that check
// the files are replaced without another look at ctx.
//
// On another error it still returns the database to go on with: db itself, in its old format, when
// the upgrade failed before closing it; otherwise path opened again, which holds the old file or,
// when the failure came after the replacement, the new one. It returns nil when that open failed
// too.
func (m *Migrator) UpgradeFormat(ctx context.Context, db anystore.DB, path string, config *anystore.Config, reset bool) (anystore.DB, error) {
	format, err := anystore.SchemaFormat(db)
	if err != nil {
		return db, fmt.Errorf("read schema format: %w", err)
	}
	if format >= anystore.LatestSchemaFormat {
		return db, nil
	}
	select {
	case m.sem <- struct{}{}:
	case <-ctx.Done():
		return db, fmt.Errorf("wait for format upgrade turn: %w", ctx.Err())
	}
	defer func() { <-m.sem }()

	start := time.Now()
	if reset {
		db, err = m.resetDatabase(ctx, db, path, config)
	} else {
		db, err = m.replaceWithCopy(ctx, db, path, config)
	}
	if err != nil {
		return db, fmt.Errorf("upgrade to format %d: %w", anystore.LatestSchemaFormat, err)
	}
	log.Info("database format upgraded",
		zap.String("path", path),
		zap.Int("from", format),
		zap.Int("to", anystore.LatestSchemaFormat),
		zap.Bool("reset", reset),
		zap.Duration("took", time.Since(start)))
	return db, nil
}

// resetDatabase replaces the database at path with an empty one in the latest format.
func (m *Migrator) resetDatabase(ctx context.Context, db anystore.DB, path string, config *anystore.Config) (anystore.DB, error) {
	if err := ctx.Err(); err != nil {
		return db, fmt.Errorf("reset: %w", err)
	}
	// Close drops a checkpoint error, and the log goes below: checkpoint first, so that a failure
	// after the log is removed leaves a consistent main file to open again, never a partial one
	if err := db.Flush(ctx, 0, anystore.FlushModeCheckpointTruncate); err != nil {
		return db, fmt.Errorf("checkpoint: %w", err)
	}
	m.step(formatStepCheckpointed)
	if err := ctx.Err(); err != nil {
		return db, fmt.Errorf("reset: %w", err)
	}
	if err := db.Close(); err != nil {
		return reopen(ctx, path, config, fmt.Errorf("close: %w", err))
	}
	m.step(formatStepClosed)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("reset: %w", err)
	}
	// the sidecars first: a crash in between leaves the main file, which the next open resets
	// again, never an old log beside a new file
	if err := removeFiles(path+"-wal", path+"-wal-shm", path+".lock"); err != nil {
		return reopen(ctx, path, config, fmt.Errorf("remove sidecars: %w", err))
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return reopen(ctx, path, config, fmt.Errorf("sync directory: %w", err))
	}
	if err := removeFiles(path); err != nil {
		return reopen(ctx, path, config, fmt.Errorf("remove database: %w", err))
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return reopen(ctx, path, config, fmt.Errorf("sync directory: %w", err))
	}
	return reopen(ctx, path, config, nil)
}

// replaceWithCopy copies the database at path into the latest format and puts the copy in its
// place, in the order of any-store's docs/migration-format-5-to-6.md: a crash at any step leaves
// either the old file with its log, or the copy.
func (m *Migrator) replaceWithCopy(ctx context.Context, db anystore.DB, path string, config *anystore.Config) (anystore.DB, error) {
	dir := filepath.Dir(path)
	tmp := path + formatTempSuffix
	// what an earlier run left goes first: it takes space the check below counts against the copy,
	// and a copy it left can be older than the database, never to be put in place
	if err := removeFiles(formatLeftovers(tmp)...); err != nil {
		return db, fmt.Errorf("remove an earlier copy: %w", err)
	}
	size, err := filesSize(path, path+"-wal")
	if err != nil {
		return db, fmt.Errorf("size of database: %w", err)
	}
	if err = m.checkSpace(dir, []int64{size}); err != nil {
		return db, fmt.Errorf("check free space: %w", err)
	}
	removeCopy := func() {
		// what stays is removed before the next upgrade of path copies again
		if err := removeFiles(formatLeftovers(tmp)...); err != nil {
			log.Warn("remove the copy of a failed format upgrade", zap.String("path", path), zap.Error(err))
		}
	}
	// ErrCopyPublished: the copy is complete and verified at tmp, and only a step after its
	// publication failed (the directory sync, or removing a staging file, which the cleanup of
	// leftovers takes care of); the sync before the rename below is checked
	if _, err = anystore.CopyToLatestFormat(ctx, db, tmp, config); err != nil && !errors.Is(err, anystore.ErrCopyPublished) {
		removeCopy()
		return db, fmt.Errorf("copy to the latest format: %w", err)
	}
	if err = ctx.Err(); err != nil {
		removeCopy()
		return db, fmt.Errorf("copy to the latest format: %w", err)
	}
	// Close drops a checkpoint error: checkpoint first, so the old file holds everything before its
	// log goes
	if err = db.Flush(ctx, 0, anystore.FlushModeCheckpointTruncate); err != nil {
		removeCopy()
		return db, fmt.Errorf("checkpoint: %w", err)
	}
	m.step(formatStepCheckpointed)
	if err = ctx.Err(); err != nil {
		removeCopy()
		return db, fmt.Errorf("copy to the latest format: %w", err)
	}
	if err = db.Close(); err != nil {
		removeCopy()
		return reopen(ctx, path, config, fmt.Errorf("close: %w", err))
	}
	m.step(formatStepClosed)
	// the last point to stop without replacing anything: the closed database stays as it is
	if err = ctx.Err(); err != nil {
		removeCopy()
		return nil, fmt.Errorf("copy to the latest format: %w", err)
	}
	if err = removeFiles(path+"-wal", path+"-wal-shm"); err != nil {
		removeCopy()
		return reopen(ctx, path, config, fmt.Errorf("remove log: %w", err))
	}
	if err = syncDir(dir); err != nil {
		removeCopy()
		return reopen(ctx, path, config, fmt.Errorf("sync directory: %w", err))
	}
	if err = os.Rename(tmp, path); err != nil {
		removeCopy()
		return reopen(ctx, path, config, fmt.Errorf("put the copy in place: %w", err))
	}
	if err = syncDir(dir); err != nil {
		return reopen(ctx, path, config, fmt.Errorf("sync directory: %w", err))
	}
	return reopen(ctx, path, config, nil)
}

// The steps of a format upgrade a test can act at (Options.formatStep).
const (
	formatStepCheckpointed = "checkpointed"
	formatStepClosed       = "closed"
)

func (m *Migrator) step(name string) {
	if m.opts.formatStep != nil {
		m.opts.formatStep(name)
	}
}

// formatLeftovers are the files a format upgrade copying into tmp can leave: the copy and its
// sidecars, and CopyToLatestFormat's lock and staging files.
func formatLeftovers(tmp string) []string {
	return append(tempFiles(tmp), append(tempFiles(tmp+".copy-staging"), tmp+".copy-lock")...)
}

// reopen opens path after the database was closed, and returns it with cause. When the open fails
// too, it returns nil and both errors.
func reopen(ctx context.Context, path string, config *anystore.Config, cause error) (anystore.DB, error) {
	db, err := anystore.Open(ctx, path, config)
	if err != nil {
		return nil, errors.Join(cause, fmt.Errorf("open again: %w", err))
	}
	return db, cause
}
