package anystoremigrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ErrNotEnoughSpace is returned, as a *NotEnoughSpaceError, when the volume can't hold a
// migration. It is returned before anything is written.
var ErrNotEnoughSpace = errors.New("not enough free disk space to migrate the database")

// NotEnoughSpaceError tells how much space a migration needs and how much the volume has.
type NotEnoughSpaceError struct {
	Path              string
	Needed, Available uint64
}

func (e *NotEnoughSpaceError) Error() string {
	return fmt.Sprintf("%s: %s: need %d bytes, %d available", ErrNotEnoughSpace, e.Path, e.Needed, e.Available)
}

func (e *NotEnoughSpaceError) Unwrap() error {
	return ErrNotEnoughSpace
}

const (
	// walFloor is the least WAL a migration is allowed: the build checkpoints once the WAL passes
	// walCheckpointBytes, so it holds that plus one batch or one index build.
	walFloor = 4 * walCheckpointBytes
	// marginFloor is the least headroom kept free on the volume, for everything else the
	// process and the system write meanwhile.
	marginFloor = 64 << 20
)

// spaceNeeded is the free space the migration of a v1 database of srcBytes (file and WAL) needs:
//
//	new file       srcBytes: the v2 file holds the same documents and indexes; measured on real
//	               data it is smaller than the v1 one
//	WAL allowance  max(srcBytes, walFloor): between checkpoints the WAL holds at most a few
//	               checkpoint intervals of copy; an index build is one transaction, so its WAL
//	               holds the whole index, which is smaller than the database
//	margin         marginFloor + srcBytes/20
//
// The v1 file stays until the new one is in place, so nothing is freed during a migration.
func spaceNeeded(srcBytes int64) uint64 {
	src := uint64(max(srcBytes, 0))
	return src + max(src, walFloor) + marginFloor + src/20
}

// spaceNeededForAll is the free space migrating the v1 databases of sizes, one after the other,
// needs at its peak. When the v1 files are kept, every new file stays beside its v1 one: all the
// new files, plus the WAL allowance and margin of the one migrating. When they are removed after
// verify, each new file takes the place of its v1 one: the largest single migration.
func (m *Migrator) spaceNeededForAll(sizes []int64) uint64 {
	var newFiles, largestExtra, largest uint64
	for _, size := range sizes {
		need := spaceNeeded(size)
		src := uint64(max(size, 0))
		newFiles += src
		largestExtra = max(largestExtra, need-src)
		largest = max(largest, need)
	}
	if m.opts.RemoveV1AfterVerify {
		return largest
	}
	return newFiles + largestExtra
}

// checkSpace fails with a *NotEnoughSpaceError when the volume of dir can't hold the migration of
// the v1 databases of sizes.
func (m *Migrator) checkSpace(dir string, sizes []int64) error {
	needed := m.spaceNeededForAll(sizes)
	available, err := m.opts.FreeSpace(dir)
	if err != nil {
		return fmt.Errorf("free disk space of %s: %w", dir, err)
	}
	if available < needed {
		return &NotEnoughSpaceError{Path: dir, Needed: needed, Available: available}
	}
	return nil
}

// CheckSpace fails with a *NotEnoughSpaceError when the volume can't hold the migration of all
// the v1 databases among paths, one after the other. It is the up-front check of a store whose
// databases migrate one by one as they are opened.
func (m *Migrator) CheckSpace(paths []string) error {
	var (
		sizes []int64
		dir   string
	)
	for _, p := range paths {
		isV1, err := IsV1File(p)
		if err != nil {
			return fmt.Errorf("check %s: %w", p, err)
		}
		if !isV1 {
			continue
		}
		size, err := filesSize(p, p+"-wal")
		if err != nil {
			return fmt.Errorf("size of %s: %w", p, err)
		}
		sizes = append(sizes, size)
		dir = filepath.Dir(p)
	}
	if len(sizes) == 0 {
		return nil
	}
	return m.checkSpace(dir, sizes)
}

// syncDir makes a rename in dir durable. Windows can't open a directory for syncing; there the
// rename is durable on its own (MoveFileEx).
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	if err = f.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	return nil
}
