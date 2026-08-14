package runstore

import (
	"context"
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
}

// Spool opens the run db's spool collection.
func (s *Store) Spool(ctx context.Context) (*Spool, error) {
	coll, err := s.db.Collection(ctx, collSpool)
	if err != nil {
		return nil, fmt.Errorf("open spool collection: %w", err)
	}
	return &Spool{coll: coll, arenas: s.arenas}, nil
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
	return &Spool{coll: coll, arenas: &anyenc.ArenaPool{}, ownDb: db}, nil
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

// Replay streams the spooled objects back in emission order — the recorded
// definitions-before-use order pass 3's resolution depends on.
func (sp *Spool) Replay(ctx context.Context, emit func(o *importv2.Object) error) error {
	iter, err := sp.coll.Find(nil).Sort("id").Iter(ctx)
	if err != nil {
		return fmt.Errorf("iterate spool: %w", err)
	}
	defer iter.Close()
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return fmt.Errorf("read spool doc: %w", err)
		}
		object, err := unmarshalSpoolRow(doc.Value())
		if err != nil {
			// Unlike the tolerant compensation reader, the spool is the
			// import's PRIMARY content: a row that cannot be decoded is a
			// loss the run must fail on, not skip silently.
			return fmt.Errorf("decode spool row %q: %w", doc.Value().GetStringBytes("id"), err)
		}
		if err = emit(object); err != nil {
			return err
		}
	}
	return nil
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
