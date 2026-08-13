// Package runstore is the durable per-run state of an import: a dedicated
// any-store database plus the run's file-spill directory, living together in
// one directory whose deletion is the run's entire disposal
// (docs/superpowers/specs/2026-08-13-importv2-durable-queue-design.md §4).
//
// Phase A scope: the manifest, the effect ledger (entries + files) and the
// frozen-core compensation reader. The identity ledger (payloads, derived,
// claims) is phase B.
//
// Serialization follows the filequeue storage idiom
// (core/files/filesync/filequeue/fileinfo.go): hand-written anyenc
// marshal/unmarshal pairs, tolerant reads that skip undecodable rows.
package runstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	sqlite "github.com/anyproto/go-sqlite"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("import-v2-runstore")

// SchemaVersion is bumped only for incompatible changes; additive fields do
// not bump it. The frozen compensation core (§4.4) — Manifest.{SchemaVersion,
// State, SpaceId}, entries.{id, objectId, mode, status, rank},
// files.{id, objectId, status, preExisting} — may only ever gain siblings, so
// CompensationInputs stays readable against any past version.
const SchemaVersion = 1

// State is the manifest lifecycle state. The full enum is part of the v1
// schema even though phase A's sweep treats every non-terminal state the
// same way (compensate + delete).
type State string

const (
	StateRunning      State = "running"
	StateSuspended    State = "suspended"
	StateCancelling   State = "cancelling"
	StateCompensating State = "compensating"
	StateCompleted    State = "completed"
	StateFailed       State = "failed"
)

// Manifest is the run's self-description. It deliberately does NOT carry the
// serialized import request in phase A: only resume (phase B) needs it, and
// storing it raises the token-at-rest question (spec OQ2) that phase A can
// simply avoid.
type Manifest struct {
	SchemaVersion  int
	RunId          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	State          State
	Incarnation    int
	ResumeAttempts int
	SpaceId        string
	ImportType     int64
	Mode           int64
	UpdateExisting bool
	NoCollection   bool
	PathIndex      int
	Converter      string
	AppVersion     string
}

// CompensationInputs is the frozen-core view: exactly what compensation
// needs, ordered newest-first (matching the in-memory journal's delete
// order), readable against any schema version by the §4.4 freeze policy.
type CompensationInputs struct {
	Created    []string // run-created object ids, newest first
	OwnedFiles []string // run-uploaded file object ids (pre-existing excluded), newest first
	Updated    []string // updated existing objects — reported uncovered, never deleted
}

const (
	dbFileName   = "run.db"
	spillDirName = "spill"
	manifestId   = "manifest"

	collManifest = "manifest"
	collEntries  = "entries"
	collFiles    = "files"

	modeMinted  = "minted"
	modeMatched = "matched"

	statusPersisted = "persisted"
	statusDone      = "done"

	actionCreated = "created"
	actionUpdated = "updated"
)

// Store is one run's durable state. Record methods are safe for concurrent
// worker use (any-store serializes writers; the arena pool guards encoding).
type Store struct {
	dir      string
	db       anystore.DB
	manifest anystore.Collection
	entries  anystore.Collection
	files    anystore.Collection
	arenas   *anyenc.ArenaPool
	rank     atomic.Int64
}

// RunsRoot is where all run dirs live for an account repo.
func RunsRoot(repoPath string) string {
	return filepath.Join(repoPath, "importv2", "runs")
}

// ListRunDirs enumerates run directories under root. A missing root is an
// empty listing, not an error (no import has ever run).
func ListRunDirs(root string) ([]string, error) {
	dirEntries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read runs root: %w", err)
	}
	var dirs []string
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			dirs = append(dirs, filepath.Join(root, dirEntry.Name()))
		}
	}
	return dirs, nil
}

// IsMissingManifest reports an Open failure meaning the db opened fine but
// never got its manifest — a dir whose creation crashed before the first
// write. No manifest means no recorded effects: safe for the sweep to
// delete outright.
func IsMissingManifest(err error) bool {
	return errors.Is(err, anystore.ErrDocNotFound)
}

// IsCorrupted reports whether an Open failure means the db file is damaged
// (as opposed to transient IO trouble) — the sweep's delete-dir criterion,
// so it must never fire on a recoverable condition.
//
// Classification is deliberately narrow:
//   - the fork's codes are checked directly (anystorehelper classifies by
//     zombiezen.com/go/sqlite codes, which cannot unwrap the
//     github.com/anyproto/go-sqlite error type any-store v0.4.7 emits);
//   - only CORRUPT and NOTADB count — CANTOPEN is excluded on purpose: it
//     also means EACCES, fd exhaustion and some disk-full paths, none of
//     which is a damaged file (the provider's reinit path counts CANTOPEN
//     as corruption; deleting a run ledger on a permission hiccup is the
//     wrong trade here).
func IsCorrupted(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, anystore.ErrQuickCheckFailed) || errors.Is(err, anystore.ErrIncompatibleVersion) {
		return true
	}
	switch sqlite.ErrCode(err) {
	case sqlite.ResultCorrupt, sqlite.ResultNotADB:
		return true
	}
	return false
}

// Create makes the run dir (with its spill subdir), opens a fresh store and
// writes the manifest in StateRunning at SchemaVersion.
func Create(ctx context.Context, dir string, m Manifest) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, spillDirName), 0o700); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	s, err := open(ctx, dir)
	if err != nil {
		return nil, err
	}
	m.SchemaVersion = SchemaVersion
	m.State = StateRunning
	m.Incarnation = 1
	now := time.Now().Truncate(time.Second)
	m.CreatedAt = now
	m.UpdatedAt = now
	if err = s.writeManifest(ctx, m); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("write manifest: %w", err)
	}
	return s, nil
}

// Open opens an existing run dir. It fails when the db is missing, corrupted
// (see IsCorrupted) or carries no manifest.
func Open(ctx context.Context, dir string) (*Store, error) {
	s, err := open(ctx, dir)
	if err != nil {
		return nil, err
	}
	if _, err = s.Manifest(ctx); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if err = s.seedRank(ctx); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("seed rank: %w", err)
	}
	return s, nil
}

func open(ctx context.Context, dir string) (*Store, error) {
	db, err := anystore.Open(ctx, filepath.Join(dir, dbFileName), storeConfig())
	if err != nil {
		return nil, fmt.Errorf("open run db: %w", err)
	}
	s := &Store{dir: dir, db: db, arenas: &anyenc.ArenaPool{}}
	for _, coll := range []struct {
		name   string
		target *anystore.Collection
	}{
		{collManifest, &s.manifest},
		{collEntries, &s.entries},
		{collFiles, &s.files},
	} {
		if *coll.target, err = db.Collection(ctx, coll.name); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("open collection %s: %w", coll.name, err)
		}
	}
	// The one secondary index the design keeps (spec §4.3): compensation and
	// diagnostics look effects up by final object id.
	if err = anystorehelper.AddIndexes(ctx, s.entries, []anystore.IndexInfo{{Fields: []string{"objectId"}}}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure entries index: %w", err)
	}
	return s, nil
}

// storeConfig mirrors the app's objectstore settings plus the durability
// block the spec fixes (§4.1): WAL with synchronous=normal, idle auto-flush,
// sentinel dirty-detection for quick-check on reopen.
func storeConfig() *anystore.Config {
	return &anystore.Config{
		ReadConnections: 1,
		SQLiteConnectionOptions: map[string]string{
			"synchronous":        "normal",
			"wal_autocheckpoint": "10000",
		},
		Durability: anystore.DurabilityConfig{
			AutoFlush: true,
			IdleAfter: 20 * time.Second,
			FlushMode: anystore.FlushModeCheckpointPassive,
			Sentinel:  true,
		},
	}
}

func (s *Store) Dir() string { return s.dir }

// SpillDir is where the run's file bytes go (archive-entry spills, notion
// downloads). It shares the run dir's lifetime: bytes survive a crash and
// are collected by Drop.
func (s *Store) SpillDir() string { return filepath.Join(s.dir, spillDirName) }

func (s *Store) Manifest(ctx context.Context) (Manifest, error) {
	doc, err := s.manifest.FindId(ctx, manifestId)
	if err != nil {
		return Manifest{}, fmt.Errorf("find manifest: %w", err)
	}
	return unmarshalManifest(doc.Value()), nil
}

// SetState persists a lifecycle transition and bumps UpdatedAt.
func (s *Store) SetState(ctx context.Context, state State) error {
	m, err := s.Manifest(ctx)
	if err != nil {
		return err
	}
	m.State = state
	m.UpdatedAt = time.Now().Truncate(time.Second)
	return s.writeManifest(ctx, m)
}

func (s *Store) writeManifest(ctx context.Context, m Manifest) error {
	arena := s.arenas.Get()
	defer func() {
		arena.Reset()
		s.arenas.Put(arena)
	}()
	return s.manifest.UpsertOne(ctx, marshalManifest(arena, m))
}

// RecordCreated journals one run-created tree object (spec §5.3): the
// write-through effect row behind persist's journal seam.
func (s *Store) RecordCreated(ctx context.Context, sourceKey, objectId string) error {
	return s.recordEntry(ctx, sourceKey, objectId, modeMinted, actionCreated)
}

// RecordUpdated journals an updated existing object. Updates are reported as
// uncovered by compensation (design decision §13.3), never rolled back.
func (s *Store) RecordUpdated(ctx context.Context, sourceKey, objectId string) error {
	return s.recordEntry(ctx, sourceKey, objectId, modeMatched, actionUpdated)
}

// recordEntry merges one effect into the row keyed by sourceKey. Two rules
// keep the delete set sound under re-records (a later effect must never
// erase what compensation needs to know):
//
//   - minted is STICKY: once a row says the run created the object, no
//     later effect may downgrade it — deletion supersedes update-rollback
//     for an object this run made;
//   - rank is assigned at the row's FIRST write and never changes (it is a
//     frozen field, §4.4, and compensation ordering depends on it).
func (s *Store) recordEntry(ctx context.Context, sourceKey, objectId, mode, action string) error {
	rank := int(s.rank.Add(1))
	_, err := s.entries.UpsertId(ctx, sourceKey, query.ModifyFunc(
		func(arena *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
			if string(v.GetStringBytes("mode")) == modeMinted {
				return v, false, nil
			}
			v.Set("objectId", arena.NewString(objectId))
			v.Set("mode", arena.NewString(mode))
			v.Set("status", arena.NewString(statusPersisted))
			v.Set("action", arena.NewString(action))
			if v.Get("rank") == nil {
				v.Set("rank", arena.NewNumberInt(rank))
			}
			v.Set("incarnation", arena.NewNumberInt(1))
			return v, true, nil
		}))
	return err
}

// RecordFile journals a file-upload outcome. preExisting marks a
// content-dedup hit on an object that already lived in the space — those are
// never compensation-deleted (the classification cannot be reconstructed
// later; see persist/journal.go). The FIRST record wins entirely: a
// re-recorded file looks pre-existing only because the first upload indexed
// it, so the first classification is the honest one.
func (s *Store) RecordFile(ctx context.Context, sourceKey, objectId string, preExisting bool) error {
	rank := int(s.rank.Add(1))
	_, err := s.files.UpsertId(ctx, sourceKey, query.ModifyFunc(
		func(arena *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
			if len(v.GetStringBytes("objectId")) > 0 {
				return v, false, nil
			}
			v.Set("objectId", arena.NewString(objectId))
			v.Set("status", arena.NewString(statusDone))
			v.Set("preExisting", arena.NewBool(preExisting))
			v.Set("rank", arena.NewNumberInt(rank))
			v.Set("incarnation", arena.NewNumberInt(1))
			return v, true, nil
		}))
	return err
}

type rankedId struct {
	id   string
	rank int
}

// CompensationInputs reads the frozen-core view (§4.4): only the
// version-frozen fields, tolerantly — an undecodable row is logged and
// skipped, never fatal (a damaged row must not block cleaning up the rest).
func (s *Store) CompensationInputs(ctx context.Context) (CompensationInputs, error) {
	var inputs CompensationInputs
	var created []rankedId
	err := s.scan(ctx, s.entries, func(v *anyenc.Value) error {
		objectId := string(v.GetStringBytes("objectId"))
		mode := string(v.GetStringBytes("mode"))
		if objectId == "" || mode == "" {
			return fmt.Errorf("row %q: missing objectId or mode", v.GetStringBytes("id"))
		}
		switch mode {
		case modeMatched:
			inputs.Updated = append(inputs.Updated, objectId)
		default:
			// minted — and, by the §4.4 reader rule, any mode this binary
			// does not know: an unrecognized mode is treated as DELETABLE
			// (a phase-B "derived" row read by an older binary must still
			// be compensated; a future non-deletable mode must bump
			// schemaVersion instead).
			created = append(created, rankedId{id: objectId, rank: v.GetInt("rank")})
		}
		return nil
	})
	if err != nil {
		return CompensationInputs{}, err
	}
	var ownedFiles []rankedId
	err = s.scan(ctx, s.files, func(v *anyenc.Value) error {
		objectId := string(v.GetStringBytes("objectId"))
		if objectId == "" {
			return fmt.Errorf("row %q: missing objectId", v.GetStringBytes("id"))
		}
		if !v.GetBool("preExisting") {
			ownedFiles = append(ownedFiles, rankedId{id: objectId, rank: v.GetInt("rank")})
		}
		return nil
	})
	if err != nil {
		return CompensationInputs{}, err
	}
	inputs.Created = newestFirst(created)
	inputs.OwnedFiles = newestFirst(ownedFiles)
	return inputs, nil
}

func newestFirst(rows []rankedId) []string {
	sort.Slice(rows, func(a, b int) bool { return rows[a].rank > rows[b].rank })
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.id)
	}
	return ids
}

// scan iterates a collection, applying read tolerantly per row: a row the
// reader rejects is logged and skipped (the filequeue unmarshalOrSkip idiom,
// filequeue/storage.go).
func (s *Store) scan(ctx context.Context, coll anystore.Collection, read func(v *anyenc.Value) error) error {
	iter, err := coll.Find(nil).Iter(ctx)
	if err != nil {
		return fmt.Errorf("iterate %s: %w", coll.Name(), err)
	}
	defer iter.Close()
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return fmt.Errorf("read %s doc: %w", coll.Name(), err)
		}
		if err = read(doc.Value()); err != nil {
			log.With("collection", coll.Name()).Warnf("skipping undecodable row: %s", err)
		}
	}
	return nil
}

func (s *Store) seedRank(ctx context.Context) error {
	maxRank := 0
	for _, coll := range []anystore.Collection{s.entries, s.files} {
		err := s.scan(ctx, coll, func(v *anyenc.Value) error {
			if rank := v.GetInt("rank"); rank > maxRank {
				maxRank = rank
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	s.rank.Store(int64(maxRank))
	return nil
}

// Flush forces a WAL checkpoint + fsync — the suspend path's durability
// point (spec §6.4; measured 4–9 ms, §8).
func (s *Store) Flush(ctx context.Context) error {
	return s.db.Flush(ctx, 0, anystore.FlushModeCheckpointPassive)
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Drop closes the store and deletes the whole run dir — the disposal the
// per-run-DB layout exists for (§4.1): O(1), no tombstones, no vacuum.
func (s *Store) Drop() error {
	err := s.db.Close()
	if removeErr := os.RemoveAll(s.dir); removeErr != nil {
		return errors.Join(err, fmt.Errorf("remove run dir: %w", removeErr))
	}
	return err
}

func marshalManifest(arena *anyenc.Arena, m Manifest) *anyenc.Value {
	obj := arena.NewObject()
	obj.Set("id", arena.NewString(manifestId))
	obj.Set("schemaVersion", arena.NewNumberInt(m.SchemaVersion))
	obj.Set("runId", arena.NewString(m.RunId))
	obj.Set("createdAt", arena.NewNumberInt(int(m.CreatedAt.UTC().Unix())))
	obj.Set("updatedAt", arena.NewNumberInt(int(m.UpdatedAt.UTC().Unix())))
	obj.Set("state", arena.NewString(string(m.State)))
	obj.Set("incarnation", arena.NewNumberInt(m.Incarnation))
	obj.Set("resumeAttempts", arena.NewNumberInt(m.ResumeAttempts))
	obj.Set("spaceId", arena.NewString(m.SpaceId))
	obj.Set("importType", arena.NewNumberInt(int(m.ImportType)))
	obj.Set("mode", arena.NewNumberInt(int(m.Mode)))
	obj.Set("updateExisting", arena.NewBool(m.UpdateExisting))
	obj.Set("noCollection", arena.NewBool(m.NoCollection))
	obj.Set("pathIndex", arena.NewNumberInt(m.PathIndex))
	obj.Set("converter", arena.NewString(m.Converter))
	obj.Set("appVersion", arena.NewString(m.AppVersion))
	return obj
}

func unmarshalManifest(v *anyenc.Value) Manifest {
	return Manifest{
		SchemaVersion:  v.GetInt("schemaVersion"),
		RunId:          string(v.GetStringBytes("runId")),
		CreatedAt:      time.Unix(int64(v.GetInt("createdAt")), 0).UTC(),
		UpdatedAt:      time.Unix(int64(v.GetInt("updatedAt")), 0).UTC(),
		State:          State(v.GetStringBytes("state")),
		Incarnation:    v.GetInt("incarnation"),
		ResumeAttempts: v.GetInt("resumeAttempts"),
		SpaceId:        string(v.GetStringBytes("spaceId")),
		ImportType:     int64(v.GetInt("importType")),
		Mode:           int64(v.GetInt("mode")),
		UpdateExisting: v.GetBool("updateExisting"),
		NoCollection:   v.GetBool("noCollection"),
		PathIndex:      v.GetInt("pathIndex"),
		Converter:      string(v.GetStringBytes("converter")),
		AppVersion:     string(v.GetStringBytes("appVersion")),
	}
}
