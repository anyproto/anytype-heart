package anystoremigrate

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// spaceNeededForAll is the free space migrating the v1 databases of sizes needs at its peak, with
// Options.Concurrency of them building at once (each with its WAL allowance and margin). copies
// tells that the volume has no hard links, so a kept v1 file is a full copy, not a second name.
//
// When the v1 files are kept, every new file stays beside its v1 one: all the new files (and all the
// copies), plus the allowances of the largest builds running at once. When they are removed after
// verify, each new file takes the place of its v1 one: the largest builds running at once, each
// with its copy until the v1 file goes.
func (m *Migrator) spaceNeededForAll(sizes []int64, copies bool) uint64 {
	var (
		newFiles uint64
		extras   = make([]uint64, 0, len(sizes))
		builds   = make([]uint64, 0, len(sizes))
	)
	for _, size := range sizes {
		need := spaceNeeded(size)
		src := uint64(max(size, 0))
		newFiles += src
		extras = append(extras, need-src)
		build := need
		if copies {
			build += src
		}
		builds = append(builds, build)
	}
	if m.opts.RemoveV1AfterVerify {
		return sumLargest(builds, m.opts.Concurrency)
	}
	total := newFiles + sumLargest(extras, m.opts.Concurrency)
	if copies {
		total += newFiles
	}
	return total
}

// sumLargest sums the n largest of values.
func sumLargest(values []uint64, n int) uint64 {
	slices.SortFunc(values, func(a, b uint64) int { return cmp.Compare(b, a) })
	var sum uint64
	for _, v := range values[:min(n, len(values))] {
		sum += v
	}
	return sum
}

// checkSpace fails with a *NotEnoughSpaceError when the volume of dir can't hold the migration of
// the v1 databases of sizes.
func (m *Migrator) checkSpace(dir string, sizes []int64) error {
	needed := m.spaceNeededForAll(sizes, !m.opts.linksSupported(dir))
	available, err := m.opts.FreeSpace(dir)
	if err != nil {
		return fmt.Errorf("free disk space of %s: %w", dir, err)
	}
	if available < needed {
		return &NotEnoughSpaceError{Path: dir, Needed: needed, Available: available}
	}
	return nil
}

// linksSupported reports whether the volume of dir supports hard links, by linking a probe file:
// the v1 file a migration keeps is a second name for it then, a full copy otherwise. A probe that
// can't be made counts as no links: the estimate stays on the safe side.
func linksSupported(dir string) bool {
	// a name of its own: the databases of a space migrate at once and probe the same directory
	f, err := os.CreateTemp(dir, ".anystore-migrate-link-probe-*")
	if err != nil {
		return false
	}
	probe := f.Name()
	_ = f.Close()
	defer func() { _ = removeFiles(probe, probe+"-link") }()
	return os.Link(probe, probe+"-link") == nil
}

// CheckSpace fails with a *NotEnoughSpaceError when the volume can't hold the migration of all
// the v1 databases among paths, Options.Concurrency at a time. It is the up-front check of a store whose
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
