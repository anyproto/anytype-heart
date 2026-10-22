package storestate

import (
	"context"
	"errors"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
)

const (
	addSeqKey = "q"
	// replayedKey marks that the whole change tree has been replayed into the store at
	// least once, which is what makes addSeqKey usable as an incremental watermark.
	replayedKey = "r"
)

type StoreStateTx struct {
	tx                   anystore.WriteTx
	ctx                  context.Context
	state                *StoreState
	arena                *anyenc.Arena
	maxAddSeq            uint64
	maxAddSeqChanged     bool
	fullyReplayed        bool
	fullyReplayedChanged bool
}

func (stx *StoreStateTx) Context() context.Context {
	return stx.ctx
}

func (stx *StoreStateTx) init() (err error) {
	doc, findErr := stx.state.collMeta.FindId(stx.ctx, stx.state.id)
	if findErr != nil {
		if errors.Is(findErr, anystore.ErrDocNotFound) {
			stx.maxAddSeq = 0
			return nil
		}
		return findErr
	}
	stx.maxAddSeq = uint64(doc.Value().GetInt(addSeqKey))
	stx.fullyReplayed = doc.Value().GetBool(replayedKey)
	return nil
}

func (stx *StoreStateTx) GetMaxAddSeq() uint64 {
	return stx.maxAddSeq
}

// IsFullyReplayed reports whether the object's whole change tree has already been replayed
// into the store. Until that has happened once, GetMaxAddSeq is NOT a usable watermark:
// addSeq is a local per-insert sequence that older builds never assigned, so changes stored
// back then carry addSeq == 0, and the tree only hands out changes with an addSeq strictly
// greater than the watermark — it can never reach them.
func (stx *StoreStateTx) IsFullyReplayed() bool {
	return stx.fullyReplayed
}

// MarkFullyReplayed records that the whole tree has been replayed, so that subsequent
// applies can use the addSeq watermark and only read changes added since.
func (stx *StoreStateTx) MarkFullyReplayed() {
	if !stx.fullyReplayed {
		stx.fullyReplayed = true
		stx.fullyReplayedChanged = true
	}
}

func (stx *StoreStateTx) UpdateMaxAddSeq(seq uint64) {
	if seq > stx.maxAddSeq {
		stx.maxAddSeq = seq
		stx.maxAddSeqChanged = true
	}
}

func (stx *StoreStateTx) NextOrder(prev string) string {
	return LexId.Next(prev)
}

func (stx *StoreStateTx) ApplyChangeSet(ch ChangeSet) (err error) {
	return stx.applyChangeSet(ch, false)
}

func (stx *StoreStateTx) ApplyChangeSetReturnAllErrors(ch ChangeSet) (err error) {
	return stx.applyChangeSet(ch, true)
}

func (stx *StoreStateTx) applyChangeSet(ch ChangeSet, returnAllErrors bool) (err error) {
	return stx.state.applyChangeSet(stx.ctx, ch, returnAllErrors)
}

func (stx *StoreStateTx) Commit() (err error) {
	if stx.maxAddSeqChanged || stx.fullyReplayedChanged {
		// UpsertOne replaces the whole document, so both fields are always written out,
		// not just the changed one.
		stx.arena.Reset()
		obj := stx.arena.NewObject()
		obj.Set("id", stx.arena.NewString(stx.state.id))
		obj.Set(addSeqKey, stx.arena.NewNumberInt(int(stx.maxAddSeq)))
		if stx.fullyReplayed {
			obj.Set(replayedKey, stx.arena.NewTrue())
		}
		if err = stx.state.collMeta.UpsertOne(stx.ctx, obj); err != nil {
			return
		}
	}
	return stx.tx.Commit()
}

func (stx *StoreStateTx) Rollback() (err error) {
	return stx.tx.Rollback()
}
