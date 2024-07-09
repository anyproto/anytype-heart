package storestate

import (
	"context"
	"errors"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/valyala/fastjson"
)

type StoreStateTx struct {
	tx    anystore.WriteTx
	ctx   context.Context
	state *StoreState
	arena *fastjson.Arena
}

func (stx *StoreStateTx) GetOrder(changeId string) (orderId string, err error) {
	doc, err := stx.state.collChangeOrders.FindId(stx.ctx, changeId)
	if err != nil {
		return
	}
	return string(doc.Value().GetStringBytes("o")), nil
}

func (stx *StoreStateTx) setOrder(changeId, order string) (err error) {
	stx.arena.Reset()
	obj := stx.arena.NewObject()
	obj.Set("id", stx.arena.NewString(changeId))
	obj.Set("o", stx.arena.NewString(order))
	obj.Set("t", stx.arena.NewNumberInt(int(time.Now().UnixMilli())))
	return stx.state.collChangeOrders.Insert(stx.ctx, obj)
}

func (stx *StoreStateTx) ApplyChangeSet(ch ChangeSet) (err error) {
	if err = stx.setOrder(ch.Id, ch.Order); err != nil && !errors.Is(err, anystore.ErrDocExists) {
		return
	}
	return stx.state.applyChangeSet(stx.ctx, ch)
}

func (stx *StoreStateTx) Commit() (err error) {
	return stx.tx.Commit()
}

func (stx *StoreStateTx) Rollback() (err error) {
	return stx.tx.Rollback()
}
