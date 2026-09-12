package runstore

import (
	"context"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
)

// The two write rules every ledger collection shares, expressed once.
//
// Three review rounds produced the same defect shape — an invariant fixed
// in one collection's writer and left broken in a sibling — because
// entries, files, spool and issues each carried their own sequence and
// upsert logic. The collections split into two genuine kinds (append-only
// logs: spool, issues; merge ledgers: entries, files), so full four-way
// unification would force merge semantics onto appenders and
// sequence-keyed ids onto source-keyed rows. What IS one rule everywhere:
//
//   1. a sequence continues after reopen (seedMaxSequenceId — spool and
//      issue ids; entries/files ranks share the rule via seedRank's field
//      scan, the same invariant over a different shape);
//   2. a displaced row is PLACED, never blindly written: probe occupancy,
//      never overwrite a different occupant — not even a same-id occupant
//      whose other fields differ, so a matched (never-deletable) row can
//      never be flipped (placeRow — entries synthetics and files
//      displacement).

// seedMaxSequenceId returns the highest zero-padded-decimal id in coll, 0
// when empty.
func seedMaxSequenceId(ctx context.Context, coll anystore.Collection) (int64, error) {
	ctx, opDone := opCtx(ctx)
	defer opDone()

	iter, err := coll.Find(nil).Sort("-id").Iter(ctx)
	if err != nil {
		return 0, fmt.Errorf("seed sequence: %w", err)
	}
	defer iter.Close()
	if iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return 0, fmt.Errorf("seed sequence: %w", err)
		}
		var last int64
		if _, err = fmt.Sscanf(string(doc.Value().GetStringBytes("id")), "%d", &last); err == nil {
			return last, nil
		}
	}
	return 0, nil
}

// placeRow writes a displaced row under baseKey, probing suffixed keys
// until it finds a slot that is empty or holds the IDENTICAL row
// (idempotent re-record). isSame decides identity over the FULL row, not
// the objectId alone.
//
// The idempotent branch re-invokes write over the identical row, so write
// callbacks must guard frozen fields themselves — rank in particular is
// set only when absent (recordEntry's first-write rule; compensation
// ordering depends on it).
func placeRow(ctx context.Context, coll anystore.Collection, baseKey string,
	isSame func(v *anyenc.Value) bool, write func(a *anyenc.Arena, v *anyenc.Value)) error {
	key := baseKey
	for attempt := 0; attempt < 100; attempt++ {
		occupied := false
		_, err := coll.UpsertId(ctx, key, query.ModifyFunc(
			func(a *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
				if len(v.GetStringBytes("objectId")) > 0 && !isSame(v) {
					occupied = true
					return v, false, nil
				}
				write(a, v)
				return v, true, nil
			}))
		if err != nil {
			return err
		}
		if !occupied {
			return nil
		}
		key = fmt.Sprintf("%s-%d", baseKey, attempt+2)
	}
	return fmt.Errorf("displaced row for %q could not be placed after 100 attempts", baseKey)
}
