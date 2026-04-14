package storestate

import (
	"context"
	"errors"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
)

const addSeqKey = "q"

type StoreStateTx struct {
	tx              anystore.WriteTx
	ctx             context.Context
	state           *StoreState
	arena           *anyenc.Arena
	maxAddSeq       uint64
	maxAddSeqChanged bool
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
	return nil
}

func (stx *StoreStateTx) GetMaxAddSeq() uint64 {
	return stx.maxAddSeq
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
	if stx.maxAddSeqChanged {
		stx.arena.Reset()
		obj := stx.arena.NewObject()
		obj.Set("id", stx.arena.NewString(stx.state.id))
		obj.Set(addSeqKey, stx.arena.NewNumberInt(int(stx.maxAddSeq)))
		if err = stx.state.collMeta.UpsertOne(stx.ctx, obj); err != nil {
			return
		}
	}
	return stx.tx.Commit()
}

func (stx *StoreStateTx) Rollback() (err error) {
	return stx.tx.Rollback()
}
