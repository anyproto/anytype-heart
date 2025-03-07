package storestate

import (
	"context"
	"errors"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
)

const maxOrderId = "_max"

type StoreStateTx struct {
	tx              anystore.WriteTx
	ctx             context.Context
	state           *StoreState
	arena           *anyenc.Arena
	maxOrder        string
	maxOrderChanged bool
}

func (stx *StoreStateTx) Context() context.Context {
	return stx.ctx
}

func (stx *StoreStateTx) init() (err error) {
	stx.maxOrder, err = stx.GetOrder(maxOrderId)
	if err != nil && !errors.Is(err, ErrOrderNotFound) {
		return
	}
	return nil
}

func (stx *StoreStateTx) GetPrevOrderId(orderId string) (string, error) {
	iter, err := stx.state.collChangeOrders.Find(query.Key{
		Path:   []string{"o"},
		Filter: query.NewComp(query.CompOpLt, orderId),
	}).Sort("-o").Limit(1).Iter(stx.ctx)
	if err != nil {
		return "", fmt.Errorf("open iterator: %w", err)
	}
	defer iter.Close()

	if !iter.Next() {
		return "", iter.Err()
	}
	doc, err := iter.Doc()
	if err != nil {
		return "", fmt.Errorf("get prev order id: %w", err)
	}
	return string(doc.Value().GetStringBytes("o")), nil
}

func (stx *StoreStateTx) GetOrder(changeId string) (orderId string, err error) {
	doc, err := stx.state.collChangeOrders.FindId(stx.ctx, changeId)
	if err != nil {
		if errors.Is(err, anystore.ErrDocNotFound) {
			err = ErrOrderNotFound
		}
		return
	}
	return string(doc.Value().GetStringBytes("o")), nil
}

func (stx *StoreStateTx) GetMaxOrder() string {
	return stx.maxOrder
}

func (stx *StoreStateTx) NextOrder(prev string) string {
	return lexId.Next(prev)
}

func (stx *StoreStateTx) SetOrder(changeId, order string) (err error) {
	stx.arena.Reset()
	obj := stx.arena.NewObject()
	obj.Set("id", stx.arena.NewString(changeId))
	obj.Set("o", stx.arena.NewString(order))
	obj.Set("t", stx.arena.NewNumberInt(int(time.Now().UnixMilli())))
	if err = stx.state.collChangeOrders.UpsertOne(stx.ctx, obj); err != nil {
		return
	}
	stx.checkMaxOrder(order)
	return
}

func (stx *StoreStateTx) checkMaxOrder(order string) {
	if order > stx.maxOrder {
		stx.maxOrder = order
		stx.maxOrderChanged = true
	}
}

func (stx *StoreStateTx) ApplyChangeSet(ch ChangeSet) (err error) {
	if err = stx.SetOrder(ch.Id, ch.Order); err != nil && !errors.Is(err, anystore.ErrDocExists) {
		return
	}
	err = stx.state.applyChangeSet(stx.ctx, ch)
	return err
}

func (stx *StoreStateTx) Commit() (err error) {
	if stx.maxOrderChanged {
		if err = stx.SetOrder(maxOrderId, stx.maxOrder); err != nil {
			return
		}
	}
	return stx.tx.Commit()
}

func (stx *StoreStateTx) Rollback() (err error) {
	return stx.tx.Rollback()
}
