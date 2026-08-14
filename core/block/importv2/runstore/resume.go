package runstore

import (
	"context"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The dormant-run readers: everything a pass-3 restart (DM spec §8.1) and a
// status query over a run with no live engine (§15.4) read from the ledger.
// Unlike the deliberately tolerant compensation reader, these are STRICT —
// a row that cannot be decoded here would silently corrupt a replay (a lost
// identity re-mints; a lost file row re-uploads under a new id), so the
// resume must fail loudly instead and leave the dir to the sweep's
// compensate-on-exhaustion fallback.

// EntryRecord is one identity-ledger row as rehydration reads it, with the
// retained create payload joined for minted claims.
type EntryRecord struct {
	SourceKey string
	ObjectId  string
	// Matched means the claim dedup-matched an existing object (mode
	// "matched"): update path, never deletable, no payload.
	Matched bool
	// Derived means the row is a derived-class create's write-ahead intent
	// (mode "derived", review Class C) or its completed effect: no pass-1
	// claim, no payload — the replay re-derives it deterministically, and a
	// non-terminal row is heal proof for its class.
	Derived bool
	// Terminal means the object reached a terminal status in a previous
	// incarnation — the replay skips it.
	Terminal bool
	// Action is the recorded persist outcome ("created"/"updated"), empty
	// while non-terminal.
	Action string
	Rank   int
	// Late means the claim was recorded after materialization began — a
	// finalize-stage claim (root collection, report page), not a converter
	// entity: it has no spool row and must not be reconciled against one.
	Late bool
	// Synthetic means the row preserves a DISPLACED id (an identity
	// conflict upstream): it can have no spool row by construction and
	// exists for compensation alone — rehydration, reconciliation,
	// inference and counters all exclude it.
	Synthetic bool
	// PayloadRoot/PayloadHeads reconstruct the create payload for minted
	// claims (nil for matched rows and rows whose payload was never
	// recorded).
	PayloadRoot  []byte
	PayloadHeads []string
}

// FileRecord is one file-ledger row: an upload that completed in a previous
// incarnation, with its ownership classification.
type FileRecord struct {
	SourceKey   string
	ObjectId    string
	PreExisting bool
	// Synthetic marks a displaced-id preservation row (identity conflict
	// upstream): compensation-only, excluded from rehydration and counters.
	Synthetic bool
}

// ReadEntries returns every identity-ledger row with payloads joined.
//
// Scan order is load-bearing (review Class E, measured live: 4/27 loads
// failed against a claiming run): ENTRIES first, payloads second. A claim
// batch commits both in one tx, so any entry this scan observes already
// has its payload committed — the later payload scan can only see MORE
// rows, never fewer. The inverted order had a window (batch lands between
// the two scans) where a fresh entry read as payload-less, which the
// strict check classifies as corruption.
func (s *Store) ReadEntries(ctx context.Context) ([]EntryRecord, error) {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	type rawEntry struct {
		record EntryRecord
		minted bool
	}
	var raw []rawEntry
	err := s.scanStrict(ctx, s.entries, func(v *anyenc.Value) error {
		objectId := string(v.GetStringBytes("objectId"))
		if objectId == "" {
			return fmt.Errorf("entry %q: missing objectId", v.GetStringBytes("id"))
		}
		mode := string(v.GetStringBytes("mode"))
		record := EntryRecord{
			SourceKey: string(v.GetStringBytes("id")),
			ObjectId:  objectId,
			Matched:   mode == modeMatched,
			Derived:   mode == modeDerived,
			Terminal:  string(v.GetStringBytes("status")) == statusPersisted,
			Action:    string(v.GetStringBytes("action")),
			Rank:      v.GetInt("rank"),
			Late:      v.GetBool("late"),
			Synthetic: v.GetBool("synthetic"),
		}
		raw = append(raw, rawEntry{record: record, minted: !record.Matched && !record.Derived})
		return nil
	})
	if err != nil {
		return nil, err
	}
	payloads := map[string]payloadRecord{}
	err = s.scanStrict(ctx, s.payloads, func(v *anyenc.Value) error {
		payloads[string(v.GetStringBytes("id"))] = payloadRecord{
			root:  append([]byte(nil), v.GetBytes("root")...),
			heads: readHeads(v),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	records := make([]EntryRecord, 0, len(raw))
	for _, entry := range raw {
		if payload, ok := payloads[entry.record.ObjectId]; ok && entry.minted {
			entry.record.PayloadRoot = payload.root
			entry.record.PayloadHeads = payload.heads
		}
		records = append(records, entry.record)
	}
	return records, nil
}

type payloadRecord struct {
	root  []byte
	heads []string
}

// ReadFiles returns every file-ledger row.
func (s *Store) ReadFiles(ctx context.Context) ([]FileRecord, error) {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	var records []FileRecord
	err := s.scanStrict(ctx, s.files, func(v *anyenc.Value) error {
		objectId := string(v.GetStringBytes("objectId"))
		if objectId == "" {
			return fmt.Errorf("file row %q: missing objectId", v.GetStringBytes("id"))
		}
		records = append(records, FileRecord{
			SourceKey:   string(v.GetStringBytes("id")),
			ObjectId:    objectId,
			PreExisting: v.GetBool("preExisting"),
			Synthetic:   v.GetBool("synthetic"),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

// scanStrict iterates a collection, failing on the first row the reader
// rejects (see the package note above for why resume reads are strict where
// compensation reads are tolerant).
func (s *Store) scanStrict(ctx context.Context, coll anystore.Collection, read func(v *anyenc.Value) error) error {
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
			return fmt.Errorf("decode %s row: %w", coll.Name(), err)
		}
	}
	return nil
}

const rootSpecId = "rootSpec"

// SetRootSpec persists pass 2's RootSpec (DM spec §4.1: written at the pass
// boundary — a restart has no converter to re-produce it). A singleton
// config row, overwritten whole; it carries no merge invariant.
func (s *Store) SetRootSpec(ctx context.Context, spec importv2.RootSpec) error {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	arena := s.arenas.Get()
	defer func() {
		arena.Reset()
		s.arenas.Put(arena)
	}()
	row := arena.NewObject()
	row.Set("id", arena.NewString(rootSpecId))
	row.Set("collectionName", arena.NewString(spec.CollectionName))
	row.Set("rootObjectKey", arena.NewString(spec.RootObjectKey))
	row.Set("widgetLayout", arena.NewNumberInt(int(spec.WidgetLayout)))
	return s.kv.UpsertOne(ctx, row)
}

// ReadRootSpec returns the persisted RootSpec; found is false when pass 2
// never wrote one.
func (s *Store) ReadRootSpec(ctx context.Context) (spec importv2.RootSpec, found bool, err error) {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	doc, err := s.kv.FindId(ctx, rootSpecId)
	if err != nil {
		if IsMissingManifest(err) { // ErrDocNotFound
			return importv2.RootSpec{}, false, nil
		}
		return importv2.RootSpec{}, false, fmt.Errorf("read root spec: %w", err)
	}
	v := doc.Value()
	return importv2.RootSpec{
		CollectionName: string(v.GetStringBytes("collectionName")),
		RootObjectKey:  string(v.GetStringBytes("rootObjectKey")),
		WidgetLayout:   model.BlockContentWidgetLayout(v.GetInt("widgetLayout")),
	}, true, nil
}

// RefundResumeAttempt gives back one resume attempt (floor zero): an
// ORDERLY suspend is not a crash, and the cap exists to bound crash loops
// (review Class F: three clean quits during a long materialization
// exhausted the cap and the sweep compensated-and-dropped an import that
// never crashed). Crashes never refund — no settlement path runs — so the
// crash-loop bound is untouched.
func (s *Store) RefundResumeAttempt(ctx context.Context) error {
	m, err := s.Manifest(ctx)
	if err != nil {
		return err
	}
	if m.ResumeAttempts == 0 {
		return nil
	}
	m.ResumeAttempts--
	m.UpdatedAt = nowSecond()
	return s.writeManifest(ctx, m)
}

// MarkFetched records the pass-2/pass-3 boundary durably (DM spec §4.1 +
// §6.4), in the one order that keeps every prefix resumable: RootSpec
// first (a fetched manifest without it would restart pass 3 missing
// pass 2's output), then fetched flushed to disk, then materializing.
// One implementation for the adapter and every harness — the transition
// is journaling, and a drifted copy is how pass-boundary invariants die.
func (s *Store) MarkFetched(ctx context.Context, spec importv2.RootSpec) error {
	if err := s.SetRootSpec(ctx, spec); err != nil {
		return fmt.Errorf("persist root spec: %w", err)
	}
	if err := s.SetState(ctx, StateFetched); err != nil {
		return fmt.Errorf("mark run fetched: %w", err)
	}
	if err := s.Flush(ctx); err != nil {
		return fmt.Errorf("flush fetched run: %w", err)
	}
	if err := s.SetState(ctx, StateMaterializing); err != nil {
		return fmt.Errorf("mark run materializing: %w", err)
	}
	return nil
}

// BeginResume durably opens one resume attempt: incarnation and the attempt
// counter move BEFORE any work — a crash loop is bounded by the cap however
// early the crash lands — and the state enters materializing (setting the
// sticky compensation gate: from here a still-claimed row is the crash
// window of a possible create).
func (s *Store) BeginResume(ctx context.Context) (Manifest, error) {
	m, err := s.Manifest(ctx)
	if err != nil {
		return Manifest{}, err
	}
	m.Incarnation++
	m.ResumeAttempts++
	m.State = StateMaterializing
	m.MaterializeStarted = true
	m.UpdatedAt = nowSecond()
	if err = s.writeManifest(ctx, m); err != nil {
		return Manifest{}, fmt.Errorf("write resume manifest: %w", err)
	}
	if err = s.Flush(ctx); err != nil {
		return Manifest{}, fmt.Errorf("flush resume manifest: %w", err)
	}
	s.seedFromManifest(m)
	return m, nil
}
