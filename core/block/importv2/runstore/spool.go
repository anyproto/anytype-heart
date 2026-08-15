package runstore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	collSpool     = "spool"
	spoolDbName   = "spool.db"
	statusSpooled = "spooled"
)

// Spool is the pass-2 → pass-3 absorbing queue (deferred-materialization
// spec §6.1): converter output serialized in emission order, replayed to
// materialization. It deliberately carries no cross-version promise — an
// incompatible spool is dropped with its run dir, never migrated (§6.3).
//
// Two homes, one type: inside a run's own db (Store.Spool — the durable
// mode) or a standalone throwaway db in the volatile spill dir
// (OpenStandaloneSpool), so the serialization round-trip is exercised by
// every run including the golden harness.
type Spool struct {
	coll   anystore.Collection
	arenas *anyenc.ArenaPool
	seq    atomic.Int64
	ownDb  anystore.DB // standalone mode only; nil when backed by a Store
	// absent marks a read-only handle on a dir whose spool collection was
	// never created — a run killed during pass 1, before its first append.
	// That is an EMPTY spool, not a broken one (see Store.Spool).
	absent bool
}

// Spool opens the run db's spool collection, continuing the sequence from
// any existing rows — E2: a per-instance counter restarting at zero let a
// second handle overwrite rows and reorder the replay.
func (s *Store) Spool(ctx context.Context) (*Spool, error) {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	var coll anystore.Collection
	var err error
	if s.readOnly {
		// A status reader never creates (review Class E: the pull surface
		// must not write on a read path).
		coll, err = s.db.OpenCollection(ctx, collSpool)
		if errors.Is(err, anystore.ErrCollectionNotFound) {
			// A run killed during pass 1 — before its first append — has no
			// spool collection at all. That is an EMPTY spool, not a broken
			// dir, and erroring here made exactly such a run fail its own
			// status poll and vanish silently from the listing: the Class-E
			// symptom through a third door. Read methods answer zero; a
			// write would be a caller bug and says so.
			return &Spool{arenas: s.arenas, absent: true}, nil
		}
	} else {
		coll, err = s.db.Collection(ctx, collSpool)
	}
	if err != nil {
		return nil, fmt.Errorf("open spool collection: %w", err)
	}
	sp := &Spool{coll: coll, arenas: s.arenas}
	if err = sp.seedSeq(ctx); err != nil {
		return nil, err
	}
	return sp, nil
}

// seedSeq continues the append sequence after the highest existing row id
// (the shared seeding rule, ledgerwrite.go).
func (sp *Spool) seedSeq(ctx context.Context) error {
	last, err := seedMaxSequenceId(ctx, sp.coll)
	if err != nil {
		return fmt.Errorf("seed spool seq: %w", err)
	}
	sp.seq.Store(last)
	return nil
}

// OpenStandaloneSpool creates a throwaway spool db under dir (volatile
// runs: no run store, but the memory invariant still demands a disk-backed
// queue). The caller Closes it; the dir's removal disposes the file.
func OpenStandaloneSpool(ctx context.Context, dir string) (*Spool, error) {
	db, err := anystore.Open(ctx, filepath.Join(dir, spoolDbName), storeConfig())
	if err != nil {
		return nil, fmt.Errorf("open standalone spool: %w", err)
	}
	coll, err := db.Collection(ctx, collSpool)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open spool collection: %w", err)
	}
	sp := &Spool{coll: coll, arenas: &anyenc.ArenaPool{}, ownDb: db}
	if err = sp.seedSeq(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return sp, nil
}

// Close releases the standalone db; a Store-backed spool closes with its
// Store.
func (sp *Spool) Close() error {
	if sp.ownDb != nil {
		return sp.ownDb.Close()
	}
	return nil
}

// Append serializes one converted object in emission order. The engine must
// have drained any Open closure to the spill dir first — a closure reaching
// the spool is a contract violation, reported loudly, never a silent loss.
func (sp *Spool) Append(ctx context.Context, o *importv2.Object) error {
	if sp.absent {
		return fmt.Errorf("spool %q: this handle is read-only", o.SourceKey)
	}
	ctx, opDone := opCtx(ctx)
	defer opDone()

	if o.File != nil && o.File.Open != nil {
		return fmt.Errorf("spool %q: file source carries an undrained Open closure", o.SourceKey)
	}
	snapshot, err := o.Payload.ToProto().Marshal()
	if err != nil {
		return fmt.Errorf("spool %q: marshal snapshot: %w", o.SourceKey, err)
	}
	seq := sp.seq.Add(1)
	arena := sp.arenas.Get()
	defer func() {
		arena.Reset()
		sp.arenas.Put(arena)
	}()
	row := arena.NewObject()
	row.Set("id", arena.NewString(fmt.Sprintf("%012d", seq)))
	row.Set("sourceKey", arena.NewString(o.SourceKey))
	row.Set("sbType", arena.NewNumberInt(int(o.SbType)))
	row.Set("snapshot", arena.NewBinary(snapshot))
	row.Set("isRootCandidate", arena.NewBool(o.IsRootCandidate))
	row.Set("favorite", arena.NewBool(o.Favorite))
	row.Set("archived", arena.NewBool(o.Archived))
	row.Set("status", arena.NewString(statusSpooled))
	if o.File != nil {
		file := arena.NewObject()
		file.Set("path", arena.NewString(o.File.Path))
		file.Set("name", arena.NewString(o.File.Name))
		file.Set("url", arena.NewString(o.File.URL))
		file.Set("imageKind", arena.NewNumberInt(int(o.File.ImageKind)))
		if len(o.File.EncryptionKeys) > 0 {
			keys := arena.NewObject()
			for path, key := range o.File.EncryptionKeys {
				keys.Set(path, arena.NewString(key))
			}
			file.Set("encryptionKeys", keys)
		}
		row.Set("file", file)
	}
	return sp.coll.UpsertOne(ctx, row)
}

// spoolChunkSize bounds how many rows one read transaction touches — and
// how many decoded objects the replay buffers between reads. Emission
// happens with NO transaction held, so persist backpressure never pins the
// db's read connection or the WAL (D1/D2: the old whole-pass iterator held
// both for the entire materialize pass — hundreds of MB of unreclaimable
// WAL and every concurrent read blocked). The buffer adds ≤ chunkSize
// resident objects on top of the 2C+K lane bound.
const spoolChunkSize = 16

// Replay streams the spooled objects back in emission order — the recorded
// definitions-before-use order pass 3's resolution depends on — in bounded
// chunks re-seeded on the last id, so the db stays usable throughout.
//
// Two properties, stated for DM-2: (1) after cancellation, up to
// chunkSize-1 already-buffered objects are still emitted (benign — the
// sink rejects them on its own ctx check); (2) chunked reads are NOT
// snapshot-isolated across chunks — fine while pass 2 strictly precedes
// pass 3, but late spooling would change that.
func (sp *Spool) Replay(ctx context.Context, emit func(o *importv2.Object) error) error {
	if sp.absent {
		return nil
	}
	lastId := ""
	for {
		objects, nextId, err := sp.readChunk(ctx, lastId)
		if err != nil {
			return err
		}
		if len(objects) == 0 {
			return nil
		}
		lastId = nextId
		for _, object := range objects {
			if err = emit(object); err != nil {
				return err
			}
		}
	}
}

// SourceKeys returns the spooled source keys with their recorded classes
// (SbType) and the row count, without decoding any snapshot — the restart's
// cheap census (which rehydrated claims have a spool row; how much replay
// remains; which rows the claim/spool cross-check may demand a claim for).
func (sp *Spool) SourceKeys(ctx context.Context) (map[string]coresb.SmartBlockType, int, error) {
	if sp.absent {
		return map[string]coresb.SmartBlockType{}, 0, nil
	}
	ctx, opDone := opCtx(ctx)
	defer opDone()

	iter, err := sp.coll.Find(nil).Iter(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("iterate spool keys: %w", err)
	}
	defer iter.Close()
	keys := map[string]coresb.SmartBlockType{}
	count := 0
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, 0, fmt.Errorf("read spool doc: %w", err)
		}
		keys[string(doc.Value().GetStringBytes("sourceKey"))] = coresb.SmartBlockType(doc.Value().GetInt("sbType"))
		count++
	}
	return keys, count, nil
}

// Census counts the spooled rows split by class without decoding any
// snapshot — the status surface's totals (§15.4: pages and files are
// separate counters by requirement).
//
// Derived-class rows (relations, types, options) are counted APART from
// pages: they carry no pass-1 claim, so folding them into the page counter
// made pagesDone outrun a pagesTotal that is the claim count. The
// classification is the shared root predicate, the same one the engine's
// countObject uses — the two must never disagree about what a page is.
func (sp *Spool) Census(ctx context.Context) (pages, files, derived int, err error) {
	if sp.absent {
		return 0, 0, 0, nil
	}
	ctx, opDone := opCtx(ctx)
	defer opDone()

	iter, err := sp.coll.Find(nil).Iter(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("iterate spool census: %w", err)
	}
	defer iter.Close()
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return 0, 0, 0, fmt.Errorf("read spool doc: %w", err)
		}
		switch sbType := coresb.SmartBlockType(doc.Value().GetInt("sbType")); {
		case doc.Value().Get("file") != nil || importv2.IsFileClass(sbType):
			files++
		case importv2.IsDerivedClass(sbType):
			derived++
		default:
			pages++
		}
	}
	return pages, files, derived, nil
}

// readChunk reads and decodes up to spoolChunkSize rows after lastId, then
// releases the read transaction before returning.
func (sp *Spool) readChunk(ctx context.Context, lastId string) ([]*importv2.Object, string, error) {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	var filter any
	if lastId != "" {
		filter = fmt.Sprintf(`{"id":{"$gt":%q}}`, lastId)
	}
	iter, err := sp.coll.Find(filter).Sort("id").Iter(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("iterate spool: %w", err)
	}
	defer iter.Close()
	var objects []*importv2.Object
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, "", fmt.Errorf("read spool doc: %w", err)
		}
		object, err := unmarshalSpoolRow(doc.Value())
		if err != nil {
			// Unlike the tolerant compensation reader, the spool is the
			// import's PRIMARY content: a row that cannot be decoded is a
			// loss the run must fail on, not skip silently.
			return nil, "", fmt.Errorf("decode spool row %q: %w", doc.Value().GetStringBytes("id"), err)
		}
		lastId = string(doc.Value().GetStringBytes("id"))
		objects = append(objects, object)
		if len(objects) >= spoolChunkSize {
			break
		}
	}
	return objects, lastId, nil
}

func unmarshalSpoolRow(v *anyenc.Value) (*importv2.Object, error) {
	var base model.SmartBlockSnapshotBase
	if err := base.Unmarshal(v.GetBytes("snapshot")); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	object := &importv2.Object{
		SourceKey:       string(v.GetStringBytes("sourceKey")),
		SbType:          coresb.SmartBlockType(v.GetInt("sbType")),
		Payload:         importv2.NewSnapshotFromProto(&base),
		IsRootCandidate: v.GetBool("isRootCandidate"),
		Favorite:        v.GetBool("favorite"),
		Archived:        v.GetBool("archived"),
	}
	if file := v.Get("file"); file != nil {
		source := &importv2.FileSource{
			Path:      string(file.GetStringBytes("path")),
			Name:      string(file.GetStringBytes("name")),
			URL:       string(file.GetStringBytes("url")),
			ImageKind: model.ImageKind(file.GetInt("imageKind")),
		}
		if keys := file.GetObject("encryptionKeys"); keys != nil {
			source.EncryptionKeys = map[string]string{}
			keys.Visit(func(key []byte, value *anyenc.Value) {
				source.EncryptionKeys[string(key)] = string(value.GetStringBytes())
			})
		}
		object.File = source
	}
	return object, nil
}
