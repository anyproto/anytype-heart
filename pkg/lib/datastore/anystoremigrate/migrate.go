// Package anystoremigrate migrates the heart-owned databases (the object store, the space index
// and the per-space CRDT databases) from any-store v1 (SQLite) files to any-store v2 files, one
// database at a time, following any-store's docs/migration-v1-to-v2.md:
//
//   - documents are carried over as raw anyenc bytes, collection by collection, in batches of
//     write transactions; indexes are created after the bulk load;
//   - the new database is built in a temp file next to the old one and verified (document
//     counts, index definitions, QuickCheck) before it takes the old file's name;
//   - the v1 file is kept, as <name>.v1, unless the migrator is told to remove it once the
//     verified v2 file is in place (Options.RemoveV1AfterVerify).
//
// Chat messages collections (chatmodel.IsMessagesCollection) change layout on the way: they are
// keyed by the message storage key (chatmodel.StorageKey) and get the chat repository's indexes;
// legacy stubs without an order id are dropped.
package anystoremigrate

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	anystorev1 "github.com/anyproto/any-store"
	anystore "github.com/anyproto/any-store/v2"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/freespace"
)

var log = logging.LoggerNotSugared("anystore-migrate")

const (
	// tempSuffix names the file a migration builds the v2 database in.
	tempSuffix = ".v2tmp"
	// V1BackupSuffix names the v1 file a migration keeps beside the migrated database.
	V1BackupSuffix = ".v1"
)

// Options configure a Migrator.
type Options struct {
	// Concurrency is how many databases migrate at once; others wait. Default 1.
	Concurrency int
	// RemoveV1AfterVerify removes the v1 file once the verified v2 database has taken its name.
	// When false (the default) it is kept beside it as <name>.v1.
	RemoveV1AfterVerify bool
	// DstConfig is the v2 configuration the new database is built with. Its durability options
	// are replaced: the build checkpoints the WAL itself and writes no sentinel.
	DstConfig *anystore.Config
	// SrcConfig is the v1 configuration the source is opened with.
	SrcConfig *anystorev1.Config
	// FreeSpace returns the bytes available on the volume of path. Default
	// freespace.GetFreeDiskSpace.
	FreeSpace func(path string) (uint64, error)

	// failAfterDocs makes the copy fail once that many documents were copied (tests).
	failAfterDocs int
}

// Migrator migrates v1 database files to v2, at most Options.Concurrency at a time, and one
// path only once at a time.
type Migrator struct {
	opts Options
	sem  chan struct{}

	pathLocksMu sync.Mutex
	pathLocks   map[string]*sync.Mutex
}

func New(opts Options) *Migrator {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 1
	}
	if opts.FreeSpace == nil {
		opts.FreeSpace = freespace.GetFreeDiskSpace
	}
	return &Migrator{opts: opts, sem: make(chan struct{}, opts.Concurrency), pathLocks: make(map[string]*sync.Mutex)}
}

// Report summarises the migration of one database.
type Report struct {
	Path string
	// SrcBytes is the size of the v1 file and its WAL; DstBytes the size of the v2 file.
	SrcBytes, DstBytes int64
	Collections        int
	Documents          int
	ChatCollections    int
	// StubsDropped counts the legacy chat stubs (documents without an order id) left out.
	StubsDropped int
	// PeakWalBytes is the largest size the v2 WAL was seen at during the build.
	PeakWalBytes int64
	// Waited is how long the migration waited for its turn; Duration how long it took after.
	Waited, Duration time.Duration
}

// IsV1File reports whether path holds an any-store v1 (SQLite) database.
func IsV1File(path string) (bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	header := make([]byte, len(sqliteHeader))
	n, err := f.Read(header)
	if err != nil && n == 0 {
		// an empty file is no v1 database
		return false, nil
	}
	return n == len(sqliteHeader) && string(header) == sqliteHeader, nil
}

const sqliteHeader = "SQLite format 3\x00"

// RemoveLeftovers removes what an interrupted migration of path left: the temp database and its
// sidecars. It never touches path itself or its v1 backup.
func RemoveLeftovers(path string) error {
	return removeFiles(tempFiles(path + tempSuffix)...)
}

// MigrateFile migrates the v1 database at path to v2 in place, waiting for its turn. A path that
// no longer holds a v1 database (migrated meanwhile, or never v1) is left as it is: the returned
// report has no Path. When it returns an error, path still holds the v1 database, unchanged.
func (m *Migrator) MigrateFile(ctx context.Context, path string) (rep Report, err error) {
	start := time.Now()
	unlock := m.lockPath(path)
	defer unlock()
	select {
	case m.sem <- struct{}{}:
	case <-ctx.Done():
		return rep, fmt.Errorf("wait for migration turn: %w", ctx.Err())
	}
	defer func() { <-m.sem }()

	if err = RemoveLeftovers(path); err != nil {
		return rep, fmt.Errorf("remove leftovers of an earlier migration: %w", err)
	}
	isV1, err := IsV1File(path)
	if err != nil {
		return rep, fmt.Errorf("check v1 database: %w", err)
	}
	if !isV1 {
		return rep, nil
	}
	rep.Path = path
	rep.Waited = time.Since(start)
	start = time.Now()
	defer func() {
		rep.Duration = time.Since(start)
	}()
	if rep.SrcBytes, err = filesSize(path, path+"-wal"); err != nil {
		return rep, fmt.Errorf("size of v1 database: %w", err)
	}
	if err = m.checkSpace(filepath.Dir(path), []int64{rep.SrcBytes}); err != nil {
		return rep, err
	}

	tmp := path + tempSuffix
	defer func() {
		if err != nil {
			_ = removeFiles(tempFiles(tmp)...)
		}
	}()
	if err = m.buildAndVerify(ctx, path, tmp, &rep); err != nil {
		return rep, err
	}
	if rep.DstBytes, err = filesSize(tmp); err != nil {
		return rep, fmt.Errorf("size of v2 database: %w", err)
	}
	if err = m.swap(path, tmp); err != nil {
		return rep, fmt.Errorf("swap in the v2 database: %w", err)
	}
	log.Info("database migrated to any-store v2",
		zap.String("path", path),
		zap.Int64("srcBytes", rep.SrcBytes),
		zap.Int64("dstBytes", rep.DstBytes),
		zap.Int("collections", rep.Collections),
		zap.Int("documents", rep.Documents),
		zap.Int("chatCollections", rep.ChatCollections),
		zap.Int("chatStubsDropped", rep.StubsDropped),
		zap.Int64("peakWalBytes", rep.PeakWalBytes),
		zap.Duration("took", time.Since(start)),
		zap.Bool("v1Kept", !m.opts.RemoveV1AfterVerify))
	return rep, nil
}

// lockPath serializes the migrations of one path: two openers of one database never build it
// twice, nor into one temp file.
func (m *Migrator) lockPath(path string) (unlock func()) {
	m.pathLocksMu.Lock()
	l, ok := m.pathLocks[path]
	if !ok {
		l = &sync.Mutex{}
		m.pathLocks[path] = l
	}
	m.pathLocksMu.Unlock()
	l.Lock()
	return l.Unlock
}

// buildAndVerify builds the v2 database in tmp from the v1 one at path and verifies it against
// the source, reopened from disk.
func (m *Migrator) buildAndVerify(ctx context.Context, path, tmp string, rep *Report) error {
	// Open fills defaults into the config it gets, including a map, so every open needs its own copy.
	var srcConfig anystorev1.Config
	if m.opts.SrcConfig != nil {
		srcConfig = *m.opts.SrcConfig
	}
	srcConfig.SQLiteConnectionOptions = maps.Clone(srcConfig.SQLiteConnectionOptions)
	src, err := anystorev1.Open(ctx, path, &srcConfig)
	if err != nil {
		return fmt.Errorf("open v1 database: %w", err)
	}
	srcClosed := false
	defer func() {
		if !srcClosed {
			_ = src.Close()
		}
	}()

	stubs, err := m.build(ctx, src, tmp, rep)
	if err != nil {
		return fmt.Errorf("build v2 database: %w", err)
	}
	dst, err := anystore.Open(ctx, tmp, m.dstConfig())
	if err != nil {
		return fmt.Errorf("reopen v2 database: %w", err)
	}
	if err = verify(ctx, src, dst, stubs); err != nil {
		_ = dst.Close()
		return fmt.Errorf("verify v2 database: %w", err)
	}
	if err = dst.Close(); err != nil {
		return fmt.Errorf("close v2 database: %w", err)
	}
	srcClosed = true
	if err = src.Close(); err != nil {
		return fmt.Errorf("close v1 database: %w", err)
	}
	return nil
}

// dstConfig is the configuration the v2 database is built and verified with: no sentinel (no
// lock file to carry over), no auto-flush (the build checkpoints itself).
func (m *Migrator) dstConfig() *anystore.Config {
	var cfg anystore.Config
	if m.opts.DstConfig != nil {
		cfg = *m.opts.DstConfig
	}
	cfg.Durability = anystore.DurabilityConfig{}
	return &cfg
}

// swap gives the verified v2 database at tmp the name of the v1 one at path, keeping the v1 file as
// path.v1 (a hard link, or a copy where links are not supported) until it is removed on request.
// path never goes missing: the rename replaces it atomically.
func (m *Migrator) swap(path, tmp string) error {
	// SQLite checkpointed and removed the WAL when the last connection closed; a WAL with frames
	// still in it belongs to the v1 file and can't go anywhere but with it
	if size, err := filesSize(path + "-wal"); err != nil {
		return fmt.Errorf("size of v1 wal: %w", err)
	} else if size > 0 {
		return fmt.Errorf("v1 wal %s still holds %d bytes", path+"-wal", size)
	}
	// so must the v2 one: the build checkpointed it into the main file
	if size, err := filesSize(tmp + "-wal"); err != nil {
		return fmt.Errorf("size of v2 wal: %w", err)
	} else if size > 0 {
		return fmt.Errorf("v2 wal %s still holds %d bytes", tmp+"-wal", size)
	}

	backup := path + V1BackupSuffix
	if err := removeFiles(backup, backup+".lock"); err != nil {
		return fmt.Errorf("remove stale v1 backup: %w", err)
	}
	if err := os.Link(path, backup); err != nil {
		if err = copyFile(path, backup); err != nil {
			return fmt.Errorf("keep v1 file: %w", err)
		}
	}
	// the v1 sentinel says the v1 file was not closed cleanly: it stays with the v1 file
	if err := os.Rename(path+".lock", backup+".lock"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("move v1 sentinel: %w", err)
	}
	// empty sidecars of either version: a WAL left beside path would be replayed into the new file
	if err := removeFiles(path+"-wal", path+"-shm", path+"-wal-shm", tmp+"-wal", tmp+"-wal-shm", tmp+".lock"); err != nil {
		return fmt.Errorf("remove empty sidecars: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = removeFiles(backup)
		return fmt.Errorf("rename v2 database: %w", err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	if m.opts.RemoveV1AfterVerify {
		if err := removeFiles(backup, backup+".lock"); err != nil {
			return fmt.Errorf("remove v1 file: %w", err)
		}
	}
	return nil
}

// tempFiles are the files of the v2 database at tmp.
func tempFiles(tmp string) []string {
	return []string{tmp, tmp + "-wal", tmp + "-wal-shm", tmp + ".lock"}
}

func removeFiles(paths ...string) error {
	var errs []error
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove %s: %w", p, err))
		}
	}
	return errors.Join(errs...)
}

// filesSize sums the sizes of the files that exist.
func filesSize(paths ...string) (int64, error) {
	var total int64
	for _, p := range paths {
		fi, err := os.Stat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("stat %s: %w", p, err)
		}
		total += fi.Size()
	}
	return total, nil
}

func copyFile(from, to string) (err error) {
	data, err := os.Open(from)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer data.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(to)
		}
	}()
	if _, err = out.ReadFrom(data); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err = out.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	return nil
}
