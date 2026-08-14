package identity

import (
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
)

// Rehydration seeds a Service from a previous incarnation's identity ledger
// (DM spec §8.1; the 08-13 §6.2 rehydration minus converter concerns). A
// resumed run mints nothing: every minted id was recorded at claim time
// with the very payload bytes whose hash IS the id, so the payload is
// reconstructed, never re-created (re-minting would produce a different id
// while the spooled references point at the recorded one).

// RehydratedEntry is one recorded identity decision.
type RehydratedEntry struct {
	SourceKey string
	ObjectId  string
	// Matched: the claim dedup-matched an existing object (update path).
	Matched bool
	// Terminal: the object reached a terminal status in a previous
	// incarnation — it counts as arrived (reconciliation must not flag it)
	// and will never be Assigned (the replay skips it).
	Terminal bool
	// PayloadRoot/PayloadHeads reconstruct the create payload for minted,
	// not-yet-terminal claims.
	PayloadRoot  []byte
	PayloadHeads []string
}

// RehydratedFile is one completed upload: its future rehydrates already
// resolved, so references to it resolve without any re-upload.
type RehydratedFile struct {
	SourceKey string
	ObjectId  string
}

// WithRehydrated seeds the index from ledger records.
func WithRehydrated(entries []RehydratedEntry, files []RehydratedFile) Option {
	return func(s *Service) {
		for _, record := range entries {
			mode := entryMinted
			if record.Matched {
				mode = entryMatched
			}
			s.entries[record.SourceKey] = &entry{
				id:       record.ObjectId,
				mode:     mode,
				claimed:  true,
				assigned: record.Terminal,
			}
			if !record.Matched && len(record.PayloadRoot) > 0 && !record.Terminal {
				s.payloads[record.ObjectId] = treestorage.TreeStorageCreatePayload{
					RootRawChange: &treechangeproto.RawTreeChangeWithId{
						Id:        record.ObjectId,
						RawChange: record.PayloadRoot,
					},
					Heads: record.PayloadHeads,
				}
			}
		}
		for _, record := range files {
			future := newFileFuture()
			future.complete(record.ObjectId, nil)
			s.entries[record.SourceKey] = &entry{
				id:       record.ObjectId,
				mode:     entryFile,
				future:   future,
				claimed:  false, // files are never pass-1 claims
				assigned: false,
			}
		}
	}
}
