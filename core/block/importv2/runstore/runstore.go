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
	"sync"
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
// not bump it. The frozen compensation core — Manifest.{SchemaVersion,
// State, SpaceId}, entries.{id, objectId, mode, status, rank},
// files.{id, objectId, status, preExisting} — may only ever gain siblings, so
// CompensationInputs stays readable against any past version.
//
// v2 (DM-2 review round): entries mode "derived" whose status is still
// "claimed" must NOT be deleted — the id is deterministic, so the row may
// point at an EARLIER import's object. A v1 reader's unrecognized-mode
// rule would delete exactly those rows, so the writer obligation ("a
// mode that must not be deleted cannot ride an old schemaVersion") applies
// MECHANICALLY: the bump makes v1 binaries see schemaVersion > theirs and
// hand off — the safe direction. The accepted cost: v1 dirs are
// compensated rather than resumed by v2 binaries (resume only
// within a version; compensation forever).
const SchemaVersion = 2

// State is the manifest lifecycle state. The full enum is part of the v1
// schema even though phase A's sweep treats every non-terminal state the
// same way (compensate + delete).
type State string

const (
	StateRunning State = "running"
	// StateFetched marks pass 2 complete: the spool is whole (the
	// fetch-complete marker of DM spec §4.1). StateMaterializing marks
	// pass 3 started. Both land in the sweep's compensate-by-default branch
	// on binaries that predate them — the safe outcome.
	StateFetched       State = "fetched"
	StateMaterializing State = "materializing"
	StateSuspended     State = "suspended"
	StateCancelling    State = "cancelling"
	StateCompensating  State = "compensating"
	StateCompleted     State = "completed"
	StateFailed        State = "failed"
)

// Manifest is the run's self-description. It deliberately does NOT carry the
// serialized import request in phase A: only resume (phase B) needs it, and
// storing it raises the token-at-rest question (spec OQ2) that phase A can
// simply avoid.
type Manifest struct {
	SchemaVersion int
	RunId         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	State         State
	Incarnation   int
	// ResumeAttempts counts pass-3 (materialize) restarts; its exhaustion is
	// the destructive disposition (compensate + drop). CrawlResumeAttempts
	// counts pass-2 (crawl) restarts separately (review P1): a crawl attempt
	// costs ~1 request and must not spend the budget reserved for the
	// expensive materialize phase — and a transient crawl failure (offline
	// laptop) refunds its attempt, so repeated offline starts can never
	// destroy a mid-crawl artifact. Additive field: absent in pre-split dirs,
	// which read as zero — a full fresh budget, the generous direction.
	ResumeAttempts      int
	CrawlResumeAttempts int
	SpaceId             string
	ImportType          int64
	Mode                int64
	UpdateExisting      bool
	NoCollection        bool
	PathIndex           int
	Converter           string
	AppVersion          string
	// MaterializeStarted is STICKY: set the moment the run enters pass 3
	// and never cleared by later transitions (suspend overwrites State, not
	// this). It is the compensation-scope switch — before it, claims are
	// pure intent and nothing exists in the space to delete (A1).
	MaterializeStarted bool
	// Request is the serialized pb.RpcObjectImportRequest — everything a
	// pass-2 crawl resume needs to rebuild its converter (source paths, the
	// Notion token, planner config). Stored AS-IS, no application-level
	// encryption (OQ2, decided 2026-08-14): the run dir already sits in the
	// account repo beside the wallet and the objectstore, and anytype is
	// migrating to an encrypted any-store that will cover this field with
	// the rest of the ledger — building account-derived-key blob encryption
	// now would mean owning key management forever for something the
	// storage layer is about to provide. Until then the exposure is
	// TIME-BOXED mechanically: writeManifest scrubs the field on every
	// transition out of the crawl-resumable states (running/suspended), so
	// only a run actually mid-crawl carries it — and no projection (status
	// RPCs, sweep logs) ever serves it.
	Request []byte
}

// CompensationInputs is the frozen-core view: exactly what compensation
// needs, ordered by rank descending — which is FIRST-WRITE order reversed
// (claim order once claims write rows, effect order otherwise), not strict
// persist order — readable against any schema version by the freeze
// policy.
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
	collKv       = "kv"

	modeMinted  = "minted"
	modeMatched = "matched"
	// modeDerived marks a derived-class create's write-ahead intent (review
	// Class C): derived objects are never claimed in pass 1, so this row is
	// their ONLY pre-effect record — the heal proof and the compensation
	// attribution for a create torn between the tree write and its effect
	// row. Older binaries read it through the rule (an unrecognized
	// mode is DELETABLE), which is exactly the right disposition.
	modeDerived = "derived"

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
	payloads anystore.Collection
	issues   anystore.Collection
	kv       anystore.Collection
	arenas   *anyenc.ArenaPool
	rank     atomic.Int64
	issueSeq atomic.Int64
	// readOnly marks a status-reader store (OpenStatusReader): collections
	// were opened without create, no guard is held, and callers must not
	// write. Spool() honors it by opening rather than creating.
	readOnly bool
	// materializeStarted mirrors the manifest's sticky marker so writers can
	// stamp late claims (claims recorded after materialization began are
	// finalize-stage claims, not converter entities); incarnation mirrors
	// the manifest's counter so rows record which incarnation wrote them.
	// Both are seeded from the manifest on open and advanced by
	// SetState/BeginResume.
	materializeStarted atomic.Bool
	incarnation        atomic.Int64
	// closed makes Close idempotent, so each Store releases its active-dir
	// registry hold exactly once whatever combination of Close/Drop/deferred
	// release paths runs; released tracks the guard separately so Drop can
	// hold it through the unlink (C3).
	closed   atomic.Bool
	released atomic.Bool
}

// activeDirs is the process-global registry of run dirs currently held open
// by a Store. It is the sweep's guard against touching a live run: the db's
// .lock file is a dirty sentinel, not a mutex — a second Open of a live
// run's db succeeds and Drop would unlink the dir under the live writer.
// Process-global (not per component instance) on purpose: the confirmed
// hazard is a same-process account stop/start where Close's 30s grace gave
// up on a run that is still finishing while the NEW component instance
// sweeps. Cross-process exclusivity is already the platform invariant (one
// heart per repo dir).
//
// It is a REFCOUNT, not a set (Invariant 3): a double open of one dir must
// not let the first Close disarm the guard for the still-live holder.
// Store.Close is idempotent, so each Store releases exactly once.
var (
	activeDirsMu sync.Mutex
	activeDirs   = map[string]int{}
)

// ErrActive means another Store in this process holds the run dir.
var ErrActive = errors.New("run dir is held by a live store")

// tryMarkExclusive marks the dir active only when no other holder exists —
// the atomic form of the IsActive-then-Open pair (the gap between the two
// is exactly what a DM-2 resume could slip into).
func tryMarkExclusive(dir string) bool {
	activeDirsMu.Lock()
	defer activeDirsMu.Unlock()
	key := filepath.Clean(dir)
	if activeDirs[key] > 0 {
		return false
	}
	activeDirs[key]++
	return true
}

// OpenExclusive opens a run dir only if no other holder is live, atomically
// with taking the guard. The sweep's entry point.
func OpenExclusive(ctx context.Context, dir string) (*Store, error) {
	if !tryMarkExclusive(dir) {
		return nil, ErrActive
	}
	s, err := open(ctx, dir)
	if err != nil {
		markInactive(dir)
		return nil, err
	}
	m, err := s.Manifest(ctx)
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	s.seedFromManifest(m)
	if err = s.seedRank(ctx); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("seed rank: %w", err)
	}
	issueSeq, err := seedMaxSequenceId(ctx, s.issues)
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("seed issue sequence: %w", err)
	}
	s.issueSeq.Store(issueSeq)
	return s, nil
}

// IsActive reports whether some Store in this process currently holds the
// run dir open.
func IsActive(dir string) bool {
	activeDirsMu.Lock()
	defer activeDirsMu.Unlock()
	return activeDirs[filepath.Clean(dir)] > 0
}

func markActive(dir string) {
	activeDirsMu.Lock()
	defer activeDirsMu.Unlock()
	activeDirs[filepath.Clean(dir)]++
}

func markInactive(dir string) {
	activeDirsMu.Lock()
	defer activeDirsMu.Unlock()
	key := filepath.Clean(dir)
	if activeDirs[key] <= 1 {
		delete(activeDirs, key)
		return
	}
	activeDirs[key]--
}

// RunsRoot is where all run dirs live for an account repo.
func RunsRoot(repoPath string) string {
	return filepath.Join(repoPath, "importv2", "runs")
}

// OpenStatusReader opens a run dir for ADVISORY reads only (the pull
// surface). Three deliberate differences from Open, each closing a
// confirmed hazard of using the full open for status polls (review
// Class E):
//   - NO active-dir guard: a status poll must never make the sweep skip a
//     dir (the sweep runs once per start — a skipped dir waits a whole
//     session);
//   - NO sentinel machinery: Open's quick-check clears the dirty sentinel
//     (MarkCleanAfterCheck), disarming corruption detection on exactly the
//     dirs most likely damaged — the reader must not observe-and-destroy;
//   - NO content writes: the db must already exist (a dir without run.db —
//     a stray directory — is refused, because anystore.Open would CREATE
//     one), collections are OPENED, never created, and no index is
//     ensured. Honesty note (review P2): anystore.Open still runs its own
//     system DDL on open, so the FILE is touched — what this reader
//     guarantees is no run-content mutation and no db materialisation, not
//     byte-identical files.
//
// The returned Store must only be used for reads; Close releases only the
// db handle. Concurrent with a live writer this is a plain extra SQLite
// connection (WAL) — safe, at the documented cost of read-transaction
// pressure on the writer's latency.
func OpenStatusReader(ctx context.Context, dir string) (*Store, error) {
	dbPath := filepath.Join(dir, dbFileName)
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("no run db to read: %w", err)
	}
	db, err := anystore.Open(ctx, dbPath, &anystore.Config{ReadConnections: 1})
	if err != nil {
		return nil, fmt.Errorf("open run db (status): %w", err)
	}
	s := &Store{dir: dir, db: db, arenas: &anyenc.ArenaPool{}, readOnly: true}
	s.released.Store(true) // no guard was taken; Close must not release one
	for _, coll := range []struct {
		name   string
		target *anystore.Collection
	}{
		{collManifest, &s.manifest},
		{collEntries, &s.entries},
		{collFiles, &s.files},
		{collPayloads, &s.payloads},
		{collIssues, &s.issues},
		{collKv, &s.kv},
	} {
		if *coll.target, err = db.OpenCollection(ctx, coll.name); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("open collection %s (status): %w", coll.name, err)
		}
	}
	m, err := s.Manifest(ctx)
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	s.seedFromManifest(m)
	return s, nil
}

// RunIdOfDir maps a run dir back to its runId — the dir is named by the
// runId at creation, so enumeration and by-id lookup need no db open.
func RunIdOfDir(dir string) string {
	return filepath.Base(dir)
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
	// The stop is never corruption (review P0-A): any-store wraps EVERY
	// quick-check failure as ErrQuickCheckFailed — including a shutdown's
	// cancellation and the check's own five-minute cap — and the quick
	// check runs only on dirty-sentinel dirs, i.e. exactly the crashed runs
	// whose ledgers hold uncompensated effects. A ctx-shaped or
	// SQLITE_INTERRUPT cause means "try again next start", never "unlink
	// the ledger".
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if sqlite.ErrCode(err) == sqlite.ResultInterrupt {
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
	// C2: the guard is held from BEFORE the dir exists on disk — the sweep
	// must never observe an existing-but-unguarded dir mid-creation (it
	// would unlink it under the creator, whose ledger writes would then
	// land in an unlinked db).
	markActive(dir)
	if err := os.MkdirAll(filepath.Join(dir, spillDirName), 0o700); err != nil {
		markInactive(dir)
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	s, err := open(ctx, dir)
	if err != nil {
		// Create owns the dir: a failure must not leave sweepable garbage
		// behind (removed while the guard is still held, then released).
		_ = os.RemoveAll(dir)
		markInactive(dir)
		return nil, err
	}
	m.SchemaVersion = SchemaVersion
	m.State = StateRunning
	m.Incarnation = 1
	m.MaterializeStarted = false
	now := nowSecond()
	m.CreatedAt = now
	m.UpdatedAt = now
	if m, err = s.writeManifest(ctx, m); err != nil {
		_ = s.closeDb()
		_ = os.RemoveAll(dir) // Create owns the dir; no garbage on failure
		s.releaseGuard()
		return nil, fmt.Errorf("write manifest: %w", err)
	}
	s.seedFromManifest(m)
	return s, nil
}

func nowSecond() time.Time { return time.Now().Truncate(time.Second) }

// dbOpTimeout bounds one store operation on the detached context below.
// Generous: the largest measured op (a full 5k-row scan) is ~10ms. It is
// deliberately UNDER the adapter's 30-second close grace
// (adapter.go Close) with room to spare: a wedged store (the
// ReadConnections:1 leak this machinery exists for) must fail its op
// VISIBLY inside the grace — leaving the run time to classify the store
// failure and settle on the write path — not silently burn the entire
// grace and be abandoned mid-drain with nothing logged but "did not
// drain". Nested calls SHARE one budget (opCtx below), so the bound holds
// per composite settlement step, not merely per innermost call — a
// multi-write settlement (suspend marker + flush) still fits the grace.
const dbOpTimeout = 15 * time.Second

// opCtx returns the context for ONE database operation: values preserved,
// CANCELLATION DROPPED, bounded — by the enclosing operation's own budget
// when one is already running (an inherited deadline tighter than
// dbOpTimeout), by a fresh dbOpTimeout otherwise.
//
// The detachment is the fix for the review blocker, diagnosed by stack
// capture: any-store v0.4.7 leaks the acquired connection when a query
// fails mid-prepare on a cancelled context (query.go Iter: getReadTx
// acquired, conn.Query returns 'sqlite: prepare: interrupted', the tx is
// never committed) — and with ReadConnections:1 that wedges EVERY later
// read on the handle into GetRead's eternal one-second retry: the resume's
// refund and suspend markers hung forever and Close burned its whole
// grace. A suspend cancels the ctx mid-replay by design, so the trigger is
// our own normal shutdown. The rule, applied to every operation on this
// store: cancellation is honored BETWEEN operations (the sinks and loops
// already check their run contexts); the operation itself runs undisturbed
// on a bounded detached context, exactly like the detached ledger writes
// (P0-1) — this is that rule's read-side twin.
//
// The deadline handling is the review P1 refinement: WithoutCancel drops
// the deadline TOO, so nested ops (SetState → Manifest → writeManifest;
// every composite in resume.go) each minted a fresh full budget — up to
// ~75s of cancellation-immune settlement against a 30s close grace. An
// inherited deadline under dbOpTimeout is re-imposed on the detached
// context (same wall-clock instant: nested calls share, never extend); a
// looser or absent one is capped at dbOpTimeout as before. Cancellation
// stays stripped in every case — a deadline must never smuggle the
// parent's cancel back in (a status poll's dead RPC ctx would wedge the
// live run's single read connection).
func opCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	detached := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		// The remaining budget must be POSITIVE to be worth inheriting
		// (review item 10): `remaining < dbOpTimeout` alone is true for
		// negatives too, so an enclosing budget an earlier nested op had
		// already spent was re-imposed verbatim and the operation began on
		// a context that was dead before its first statement — precisely
		// the input the any-store connection leak this function exists to
		// avoid needs. A spent budget cannot bound anything; the op takes a
		// fresh bounded one, which is still detached from cancellation and
		// still capped at dbOpTimeout.
		if remaining := time.Until(deadline); remaining > 0 && remaining < dbOpTimeout {
			return context.WithDeadline(detached, deadline)
		}
	}
	return context.WithTimeout(detached, dbOpTimeout)
}

// seedFromManifest aligns the store's in-memory markers with the manifest.
func (s *Store) seedFromManifest(m Manifest) {
	s.materializeStarted.Store(m.MaterializeStarted)
	incarnation := int64(m.Incarnation)
	if incarnation < 1 {
		incarnation = 1
	}
	s.incarnation.Store(incarnation)
}

func (s *Store) currentIncarnation() int { return int(s.incarnation.Load()) }

// Open opens an existing run dir. It fails when the db is missing, corrupted
// (see IsCorrupted) or carries no manifest.
func Open(ctx context.Context, dir string) (*Store, error) {
	markActive(dir)
	s, err := open(ctx, dir)
	if err != nil {
		markInactive(dir)
		return nil, err
	}
	m, err := s.Manifest(ctx)
	if err != nil {
		_ = s.Close() // releases the guard
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	s.seedFromManifest(m)
	if err = s.seedRank(ctx); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("seed rank: %w", err)
	}
	issueSeq, err := seedMaxSequenceId(ctx, s.issues)
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("seed issue sequence: %w", err)
	}
	s.issueSeq.Store(issueSeq)
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
		{collPayloads, &s.payloads},
		{collIssues, &s.issues},
		{collKv, &s.kv},
	} {
		if *coll.target, err = db.Collection(ctx, coll.name); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("open collection %s: %w", coll.name, err)
		}
	}
	// The one secondary index the design keeps: compensation and
	// diagnostics look effects up by final object id.
	if err = anystorehelper.AddIndexes(ctx, s.entries, []anystore.IndexInfo{{Fields: []string{"objectId"}}}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure entries index: %w", err)
	}
	return s, nil
}

// storeConfig mirrors the app's objectstore settings plus the durability
// block the spec fixes: WAL with synchronous=normal, idle auto-flush,
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

// MaterializeStarted exposes the sticky compensation-scope marker from the
// in-memory mirror the writers already keep — the SAME switch
// CompensationInputs reads to decide whether a still-claimed row is a
// possible create. Callers deciding a dir's fate need exactly that
// proposition and must not re-derive it: an in-memory oracle (the engine's
// journal was empty) says nothing about what the DURABLE rows still owe
// (review item 3). Lock-free by design — a settlement path must not cost a
// manifest read.
func (s *Store) MaterializeStarted() bool { return s.materializeStarted.Load() }

// SpillDir is where the run's file bytes go (archive-entry spills, notion
// downloads). It shares the run dir's lifetime: bytes survive a crash and
// are collected by Drop.
func (s *Store) SpillDir() string { return filepath.Join(s.dir, spillDirName) }

func (s *Store) Manifest(ctx context.Context) (Manifest, error) {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	doc, err := s.manifest.FindId(ctx, manifestId)
	if err != nil {
		return Manifest{}, fmt.Errorf("find manifest: %w", err)
	}
	return unmarshalManifest(doc.Value()), nil
}

// SetState persists a lifecycle transition and bumps UpdatedAt.
func (s *Store) SetState(ctx context.Context, state State) error {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	m, err := s.Manifest(ctx)
	if err != nil {
		return err
	}
	m.State = state
	if state == StateMaterializing {
		m.MaterializeStarted = true
	}
	m.UpdatedAt = nowSecond()
	if _, err = s.writeManifest(ctx, m); err != nil {
		return err
	}
	if state == StateMaterializing {
		s.materializeStarted.Store(true)
	}
	return nil
}

// writeManifest persists the manifest and returns EXACTLY what it wrote, so
// no caller can hand out a view the disk does not hold.
//
// The request-blob scrub lives at this ONE write site so every writer —
// SetState, MarkFetched, BeginResume, the sweep — obeys it mechanically
// (OQ2 mitigation 1): the blob's useful life is the crawl, and only the
// crawl-resumable states (running, suspended) may carry it. Materialization
// is headless by design (no source, no credentials), so the fetched
// transition is where the token leaves the disk.
func (s *Store) writeManifest(ctx context.Context, m Manifest) (Manifest, error) {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	if m.State != StateRunning && m.State != StateSuspended {
		m.Request = nil
	}
	arena := s.arenas.Get()
	defer func() {
		arena.Reset()
		s.arenas.Put(arena)
	}()
	return m, s.manifest.UpsertOne(ctx, marshalManifest(arena, m))
}

// withWriteTx runs fn inside one write transaction, releasing it on EVERY
// exit including panics. The db has a SINGLE write connection, so a leaked
// transaction wedges every later durable write — suspend markers, attempt
// refunds, effect rows — into an unbounded wait (the review blocker's
// 30s-Close signature is exactly what that wedge produces): explicit-path
// rollback is not release discipline, this is.
func (s *Store) withWriteTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	tx, err := s.db.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("write tx: %w", err)
	}
	finished := false
	defer func() {
		if !finished {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx.Context()); err != nil {
		return err // the deferred rollback releases the connection
	}
	finished = true
	return tx.Commit()
}

// RecordCreated journals one run-created tree object: the
// write-through effect row behind persist's journal seam.
func (s *Store) RecordCreated(ctx context.Context, sourceKey, objectId string) error {
	return s.recordEntry(ctx, sourceKey, objectId, modeMinted, actionCreated)
}

// RecordUpdated journals an updated existing object. Updates are reported as
// uncovered by compensation (design decision §13.3), never rolled back.
func (s *Store) RecordUpdated(ctx context.Context, sourceKey, objectId string) error {
	return s.recordEntry(ctx, sourceKey, objectId, modeMatched, actionUpdated)
}

// recordEntry merges one effect into the row keyed by sourceKey. Three rules
// keep the delete set sound under re-records (a later effect must never
// erase what compensation needs to know):
//
//   - minted is STICKY: once a row says the run created the object, no
//     later effect may downgrade it — deletion supersedes update-rollback
//     for an object this run made;
//   - rank is assigned at the row's FIRST write and never changes (it is a
//     frozen field, §4.4, and compensation ordering depends on it);
//   - a DIFFERENT objectId under the same key is never silently dropped
//     (Invariant 3): the displaced id is preserved under a synthetic key,
//     keeping its own mode (a matched id must never become deletable), and
//     the conflict — an identity violation upstream — is logged loudly.
//
// The primary merge and the displacement preservation commit in ONE
// transaction: a crash between them would lose the displaced
// id — and in the minted-sticky branch the primary write is a no-op, so the
// synthetic row is the only record of the incoming effect.
func (s *Store) recordEntry(ctx context.Context, sourceKey, objectId, mode, action string) error {
	rank := int(s.rank.Add(1))
	var displacedId, displacedMode, displacedStatus, displacedAction, keptId string
	var displacedLate bool
	return s.withWriteTx(ctx, func(txCtx context.Context) error {
		_, err := s.entries.UpsertId(txCtx, sourceKey, query.ModifyFunc(
			func(arena *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
				existingId := string(v.GetStringBytes("objectId"))
				if string(v.GetStringBytes("mode")) == modeMinted {
					if existingId != "" && existingId != objectId {
						// the INCOMING effect would vanish (minted-sticky keeps
						// the row): it persisted, so it carries that status —
						// and the classification a fresh effect row would get
						displacedId, displacedMode, displacedStatus = objectId, mode, statusPersisted
						displacedAction = action
						displacedLate = s.materializeStarted.Load()
						keptId = existingId
						return v, false, nil
					}
					// same id: the effect completes the claim — status/action
					// advance, but mode/rank/objectId (the frozen compensation
					// fields) stay exactly as first written.
					v.Set("status", arena.NewString(statusPersisted))
					v.Set("action", arena.NewString(action))
					return v, true, nil
				}
				if existingId != "" {
					if existingId == objectId {
						// same id over a non-minted row: advance status/action
						// only. Mode is IDENTITY and identity does not change —
						// in particular a matched (pre-existing, user-owned) id
						// must never flip into the delete set (bias: leak,
						// never delete user data).
						v.Set("status", arena.NewString(statusPersisted))
						v.Set("action", arena.NewString(action))
						return v, true, nil
					}
					// the EXISTING row would vanish (this write replaces it):
					// it carries ITS classification into the synthetic row —
					// mode, status, action and late all belong to the origin
					// row, not to the current phase (review Class B)
					displacedId = existingId
					displacedMode = string(v.GetStringBytes("mode"))
					displacedStatus = string(v.GetStringBytes("status"))
					displacedAction = string(v.GetStringBytes("action"))
					displacedLate = v.GetBool("late")
					keptId = objectId
				}
				v.Set("objectId", arena.NewString(objectId))
				v.Set("mode", arena.NewString(mode))
				v.Set("status", arena.NewString(statusPersisted))
				v.Set("action", arena.NewString(action))
				if v.Get("rank") == nil {
					v.Set("rank", arena.NewNumberInt(rank))
				}
				v.Set("incarnation", arena.NewNumberInt(s.currentIncarnation()))
				if v.Get("late") == nil && s.materializeStarted.Load() {
					// A FRESH effect row during pass 3 belongs to an object with
					// no earlier claim row: a finalize-stage object (whose
					// buffered claim flushes only at finish) or a derived-class
					// definition. Same marker, same reason as RecordClaims —
					// the restart's finalize inference depends on it.
					v.Set("late", arena.NewBool(true))
				}
				return v, true, nil
			}))
		if err != nil {
			return err
		}
		if displacedId == "" {
			return nil
		}
		log.With("sourceKey", sourceKey, "kept", keptId, "displaced", displacedId).
			Errorf("conflicting objectId recorded under one source key — identity invariant violation; preserving both in the ledger")
		return s.recordSyntheticEntry(txCtx, sourceKey, syntheticRow{
			objectId: displacedId, mode: displacedMode, status: displacedStatus,
			action: displacedAction, late: displacedLate,
		})
	})
}

// syntheticRow is a displaced row's FULL classification: mode keeps
// matched ids undeletable, status keeps a displaced claim behind the
// MaterializeStarted gate, and late/action belong to the origin row —
// dropping any of them re-classifies the row (review Class B: an unmarked
// displaced finalize claim read as an ordinary stream row and broke both
// the finalize inference and reconciliation).
type syntheticRow struct {
	objectId string
	mode     string
	status   string
	action   string
	late     bool
}

// recordSyntheticEntry preserves a displaced id under a synthetic key,
// marked synthetic: a synthetic row can never have a spool row, so
// rehydration, reconciliation, the finalize inference and the counters all
// exclude it — it exists for compensation alone. Placement goes through
// placeRow: never over a different occupant.
func (s *Store) recordSyntheticEntry(ctx context.Context, sourceKey string, row syntheticRow) error {
	rank := int(s.rank.Add(1))
	return placeRow(ctx, s.entries, sourceKey+"#dup-"+row.objectId,
		func(v *anyenc.Value) bool {
			return string(v.GetStringBytes("objectId")) == row.objectId &&
				string(v.GetStringBytes("mode")) == row.mode &&
				string(v.GetStringBytes("status")) == row.status
		},
		func(a *anyenc.Arena, v *anyenc.Value) {
			v.Set("objectId", a.NewString(row.objectId))
			v.Set("mode", a.NewString(row.mode))
			v.Set("status", a.NewString(row.status))
			if row.action != "" {
				v.Set("action", a.NewString(row.action))
			}
			if row.late {
				v.Set("late", a.NewBool(true))
			}
			v.Set("synthetic", a.NewBool(true))
			if v.Get("rank") == nil { // frozen at first write (recordEntry's rule)
				v.Set("rank", a.NewNumberInt(rank))
			}
			v.Set("incarnation", a.NewNumberInt(s.currentIncarnation()))
		})
}

// RecordCreateIntent writes the write-ahead intent for a derived-class
// create (mode "derived", status claimed): recorded BEFORE the tree write,
// so a tear between the create and its effect row leaves attribution and
// heal proof instead of nothing. Merge rule: an existing row is kept whole
// (a replayed derived create re-records intent over its terminal row — a
// no-op; a conflicting id would be an identity violation and is preserved
// via the entries displacement machinery on the effect write).
func (s *Store) RecordCreateIntent(ctx context.Context, sourceKey, objectId string) error {
	rank := int(s.rank.Add(1))
	var displaced bool
	err := s.withWriteTx(ctx, func(txCtx context.Context) error {
		_, err := s.entries.UpsertId(txCtx, sourceKey, query.ModifyFunc(
			func(arena *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
				if existing := string(v.GetStringBytes("objectId")); existing != "" {
					// never downgrade an existing row (E1) — but a DIFFERENT
					// id must not vanish either (review P2: this was the one
					// ledger writer with no displacement preservation)
					displaced = existing != objectId
					return v, false, nil
				}
				v.Set("objectId", arena.NewString(objectId))
				v.Set("mode", arena.NewString(modeDerived))
				v.Set("status", arena.NewString(statusClaimed))
				v.Set("rank", arena.NewNumberInt(rank))
				v.Set("incarnation", arena.NewNumberInt(s.currentIncarnation()))
				if s.materializeStarted.Load() {
					v.Set("late", arena.NewBool(true))
				}
				return v, true, nil
			}))
		if err != nil {
			return err
		}
		if !displaced {
			return nil
		}
		log.With("sourceKey", sourceKey, "displaced", objectId).
			Errorf("derived intent conflicts with an existing ledger id — preserving both")
		return s.recordSyntheticEntry(txCtx, sourceKey, syntheticRow{
			objectId: objectId, mode: modeDerived, status: statusClaimed,
			late: s.materializeStarted.Load(),
		})
	})
	return err
}

// RecordDerivedMatched resolves a derived intent row against a
// PRE-EXISTING tree: mode flips derived → matched (the one sanctioned mode
// transition — deletability strictly DECREASES, the leak-bias direction)
// and the row turns terminal. Any other row shape is left alone.
func (s *Store) RecordDerivedMatched(ctx context.Context, sourceKey, objectId string) error {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	_, err := s.entries.UpsertId(ctx, sourceKey, query.ModifyFunc(
		func(arena *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
			if string(v.GetStringBytes("mode")) != modeDerived ||
				string(v.GetStringBytes("objectId")) != objectId {
				return v, false, nil
			}
			v.Set("mode", arena.NewString(modeMatched))
			v.Set("status", arena.NewString(statusPersisted))
			return v, true, nil
		}))
	return err
}

// RecordFile journals a file-upload outcome. preExisting marks a
// content-dedup hit on an object that already lived in the space — those are
// never compensation-deleted (the classification cannot be reconstructed
// later; see persist/journal.go). The FIRST record wins entirely: a
// re-recorded file looks pre-existing only because the first upload indexed
// it, so the first classification is the honest one. Primary and
// displacement commit in one transaction (§9.1 item 2, as recordEntry).
func (s *Store) RecordFile(ctx context.Context, sourceKey, objectId string, preExisting bool) error {
	rank := int(s.rank.Add(1))
	var displacedId string
	var displacedPreExisting bool
	return s.withWriteTx(ctx, func(txCtx context.Context) error {
		_, err := s.files.UpsertId(txCtx, sourceKey, query.ModifyFunc(
			func(arena *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
				if existingId := string(v.GetStringBytes("objectId")); existingId != "" {
					if existingId != objectId {
						// first-record-wins keeps the row; the incoming id must
						// not vanish from the ledger (Invariant 3)
						displacedId, displacedPreExisting = objectId, preExisting
					}
					return v, false, nil
				}
				v.Set("objectId", arena.NewString(objectId))
				v.Set("status", arena.NewString(statusDone))
				v.Set("preExisting", arena.NewBool(preExisting))
				v.Set("rank", arena.NewNumberInt(rank))
				v.Set("incarnation", arena.NewNumberInt(s.currentIncarnation()))
				return v, true, nil
			}))
		if err != nil {
			return err
		}
		if displacedId == "" {
			return nil
		}
		log.With("sourceKey", sourceKey, "displaced", displacedId).
			Errorf("conflicting file objectId recorded under one source key — preserving both in the ledger")
		displacedRank := int(s.rank.Add(1))
		return placeRow(txCtx, s.files, sourceKey+"#dup-"+displacedId,
			func(v *anyenc.Value) bool {
				return string(v.GetStringBytes("objectId")) == displacedId &&
					v.GetBool("preExisting") == displacedPreExisting
			},
			func(a *anyenc.Arena, v *anyenc.Value) {
				v.Set("objectId", a.NewString(displacedId))
				v.Set("status", a.NewString(statusDone))
				v.Set("preExisting", a.NewBool(displacedPreExisting))
				// same rule as entries synthetics (Class B, own-audit sibling):
				// a displaced row exists for compensation alone — rehydration
				// and the counters must be able to exclude it
				v.Set("synthetic", a.NewBool(true))
				if v.Get("rank") == nil { // frozen at first write (recordEntry's rule)
					v.Set("rank", a.NewNumberInt(displacedRank))
				}
				v.Set("incarnation", a.NewNumberInt(s.currentIncarnation()))
			})
	})
}

type rankedId struct {
	id   string
	rank int
}

// CompensationInputs reads the frozen-core view: only the
// version-frozen fields, tolerantly — an undecodable row is logged and
// skipped, never fatal (a damaged row must not block cleaning up the rest).
func (s *Store) CompensationInputs(ctx context.Context) (CompensationInputs, error) {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	var inputs CompensationInputs
	// A1: pass-1 claims are pure intent. Before materialization begins,
	// nothing exists in the space — rows still in the claimed status must
	// not enter the delete set (a suspended 20k-page crawl must sweep to
	// ZERO deletes). Once pass 3 has started (the sticky manifest marker),
	// a still-claimed row IS the crash window of a possible create and is
	// deleted with not-found tolerance.
	manifest, err := s.Manifest(ctx)
	if err != nil {
		return CompensationInputs{}, fmt.Errorf("read manifest for compensation scope: %w", err)
	}
	deleteClaimed := manifest.MaterializeStarted
	// neverDelete is ID-scoped (review P1-C): placeRow preserves displaced
	// ROWS, but this reader returns IDS — so an id that ANY row classifies
	// matched (pre-existing object) or pre-existing (deduped file) must be
	// subtracted from the delete sets however many other rows name it as
	// minted/owned. Collected from every row, gates ignored: a protection
	// is a protection.
	neverDelete := map[string]struct{}{}
	err = s.scan(ctx, s.entries, func(v *anyenc.Value) error {
		if string(v.GetStringBytes("mode")) == modeMatched {
			if id := string(v.GetStringBytes("objectId")); id != "" {
				neverDelete[id] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return CompensationInputs{}, err
	}
	err = s.scan(ctx, s.files, func(v *anyenc.Value) error {
		if v.GetBool("preExisting") {
			if id := string(v.GetStringBytes("objectId")); id != "" {
				neverDelete[id] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return CompensationInputs{}, err
	}
	var created []rankedId
	err = s.scan(ctx, s.entries, func(v *anyenc.Value) error {
		objectId := string(v.GetStringBytes("objectId"))
		mode := string(v.GetStringBytes("mode"))
		if objectId == "" || mode == "" {
			return fmt.Errorf("row %q: missing objectId or mode", v.GetStringBytes("id"))
		}
		if _, protected := neverDelete[objectId]; protected && mode != modeMatched {
			return nil // id-scoped protection wins over this row's own class
		}
		if string(v.GetStringBytes("status")) == statusClaimed && !deleteClaimed {
			return nil
		}
		if mode == modeDerived && string(v.GetStringBytes("status")) == statusClaimed {
			// A derived intent that never completed: the id is DETERMINISTIC,
			// so the tree — if it exists at all — may belong to an EARLIER
			// import (the create collided and the resolution record was the
			// thing the crash ate). Leak-bias: never delete what may be
			// another run's object. A COMPLETED derived create (persisted)
			// is proven ours and stays deletable below. The skip is REPORTED
			// (review P2): the id joins the uncovered list — a possible real
			// object must never leave the record without a word.
			inputs.Updated = append(inputs.Updated, objectId)
			return nil
		}
		switch mode {
		case modeMatched:
			inputs.Updated = append(inputs.Updated, objectId)
		default:
			// minted — and, by the reader rule, any mode this binary
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
		if _, protected := neverDelete[objectId]; protected {
			// id-scoped: some row says this file pre-existed. When THIS row
			// claims ownership — a fresh upload whose id another row then
			// classified pre-existing (two identical source files deduping
			// onto one object) — the protection drops the run's OWN file.
			// The drop stands: the same ledger shape is left by a
			// crashed-then-resumed re-upload of a GENUINE user file (the
			// resumed checker sees the earlier incarnation's object), so
			// deleting on cross-row reconstruction would be a guess, and the
			// bias is leak-never-guess. But never wordlessly (the
			// derived-claimed rule above): the id joins the uncovered list.
			if !v.GetBool("preExisting") {
				inputs.Updated = append(inputs.Updated, objectId)
			}
			return nil
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
	ctx, opDone := opCtx(ctx)
	defer opDone()

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
// point (measured 4–9 ms, §8).
func (s *Store) Flush(ctx context.Context) error {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	return s.db.Flush(ctx, 0, anystore.FlushModeCheckpointPassive)
}

// closeDb closes the database exactly once; the guard release is separate
// so Drop can keep the guard alive THROUGH the unlink (C3).
func (s *Store) closeDb() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	return s.db.Close()
}

// releaseGuard releases this Store's registry hold exactly once.
func (s *Store) releaseGuard() {
	if s.released.CompareAndSwap(false, true) {
		markInactive(s.dir)
	}
}

// Close is idempotent: exactly one call releases the active-dir registry
// hold, whatever combination of Close/Drop/deferred-release paths runs.
// The release is deferred so even a panicking db close cannot leak the
// guard.
func (s *Store) Close() error {
	defer s.releaseGuard()
	return s.closeDb()
}

// Drop closes the store and deletes the whole run dir — the disposal the
// per-run-DB layout exists for: O(1), no tombstones, no vacuum. The
// guard is released only AFTER the unlink: the dir must never be observable
// as existing-but-unguarded (C3).
func (s *Store) Drop() error {
	defer s.releaseGuard() // after RemoveAll (C3), panic-safe
	err := s.closeDb()
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
	obj.Set("crawlResumeAttempts", arena.NewNumberInt(m.CrawlResumeAttempts))
	obj.Set("spaceId", arena.NewString(m.SpaceId))
	obj.Set("importType", arena.NewNumberInt(int(m.ImportType)))
	obj.Set("mode", arena.NewNumberInt(int(m.Mode)))
	obj.Set("updateExisting", arena.NewBool(m.UpdateExisting))
	obj.Set("noCollection", arena.NewBool(m.NoCollection))
	obj.Set("pathIndex", arena.NewNumberInt(m.PathIndex))
	obj.Set("converter", arena.NewString(m.Converter))
	obj.Set("appVersion", arena.NewString(m.AppVersion))
	obj.Set("materializeStarted", arena.NewBool(m.MaterializeStarted))
	if len(m.Request) > 0 {
		obj.Set("request", arena.NewBinary(m.Request))
	}
	return obj
}

func unmarshalManifest(v *anyenc.Value) Manifest {
	return Manifest{
		SchemaVersion:       v.GetInt("schemaVersion"),
		RunId:               string(v.GetStringBytes("runId")),
		CreatedAt:           time.Unix(int64(v.GetInt("createdAt")), 0).UTC(),
		UpdatedAt:           time.Unix(int64(v.GetInt("updatedAt")), 0).UTC(),
		State:               State(v.GetStringBytes("state")),
		Incarnation:         v.GetInt("incarnation"),
		ResumeAttempts:      v.GetInt("resumeAttempts"),
		CrawlResumeAttempts: v.GetInt("crawlResumeAttempts"),
		SpaceId:             string(v.GetStringBytes("spaceId")),
		ImportType:          int64(v.GetInt("importType")),
		Mode:                int64(v.GetInt("mode")),
		UpdateExisting:      v.GetBool("updateExisting"),
		NoCollection:        v.GetBool("noCollection"),
		PathIndex:           v.GetInt("pathIndex"),
		Converter:           string(v.GetStringBytes("converter")),
		AppVersion:          string(v.GetStringBytes("appVersion")),

		MaterializeStarted: v.GetBool("materializeStarted"),
		Request:            bytesCopy(v.GetBytes("request")),
	}
}

// bytesCopy detaches a value from its arena; nil stays nil.
func bytesCopy(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return append([]byte(nil), b...)
}
