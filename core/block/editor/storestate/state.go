package storestate

import (
	"context"
	"errors"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/lexid"
	"github.com/valyala/fastjson"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	log = logger.NewNamed("storeState")
)

const (
	IdFromChange = "$changeId"
)

var LexId = lexid.Must(lexid.CharsAllNoEscape, 4, 100)

const (
	CollChangeOrders = "_change_orders"
)

func New(ctx context.Context, id string, db anystore.DB, handlers ...Handler) (*StoreState, error) {
	if len(handlers) == 0 {
		return nil, fmt.Errorf("should be at least one handler")
	}

	state := &StoreState{
		id:       id,
		handlers: map[string]Handler{},
		arena:    &anyenc.Arena{},
		parser:   &fastjson.Parser{},
		db:       db,
	}

	for _, h := range handlers {
		if _, ok := state.handlers[h.CollectionName()]; ok {
			return nil, fmt.Errorf("more than one handler for collection '%s'", h.CollectionName())
		}
		state.handlers[h.CollectionName()] = h
	}

	tx, err := db.WriteTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := state.init(tx.Context()); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return state, nil
}

type ChangeSet struct {
	Id        string
	Order     string
	Creator   string
	Changes   []*pb.StoreChangeContent
	Timestamp int64
}

type Change struct {
	Id        string
	Order     string
	Creator   string
	Change    *pb.StoreChangeContent
	Timestamp int64
}

type StoreState struct {
	id               string
	collChangeOrders anystore.Collection

	handlers map[string]Handler

	arena  *anyenc.Arena
	parser *fastjson.Parser

	db anystore.DB
}

func (ss *StoreState) Id() string {
	return ss.id
}

func (ss *StoreState) init(ctx context.Context) (err error) {
	if ss.collChangeOrders, err = ss.Collection(ctx, CollChangeOrders); err != nil {
		return
	}
	for _, h := range ss.handlers {
		if err = h.Init(ctx, ss); err != nil {
			return
		}
	}
	return
}

func (ss *StoreState) NewTx(ctx context.Context) (*StoreStateTx, error) {
	tx, err := ss.db.WriteTx(ctx)
	if err != nil {
		return nil, err
	}
	stx := &StoreStateTx{state: ss, tx: tx, ctx: tx.Context(), arena: &anyenc.Arena{}}
	if err = stx.init(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return stx, nil
}

func (ss *StoreState) Collection(ctx context.Context, name string) (anystore.Collection, error) {
	return ss.db.Collection(ctx, ss.id+name)
}

func (ss *StoreState) applyChangeSet(ctx context.Context, set ChangeSet) (err error) {
	for _, ch := range set.Changes {
		applyErr := ss.applyChange(ctx, Change{
			Id:        set.Id,
			Order:     set.Order,
			Change:    ch,
			Creator:   set.Creator,
			Timestamp: set.Timestamp,
		})
		if applyErr == nil || errors.Is(applyErr, ErrIgnore) {
			continue
		}
		if errors.Is(applyErr, ErrLog) {
			log.Warn("change apply error",
				zap.Error(applyErr),
				zap.String("changeId", set.Id),
				zap.String("order", set.Order),
			)
			continue
		}
		err = applyErr
		break
	}
	return
}

func (ss *StoreState) applyChange(ctx context.Context, ch Change) (err error) {
	ss.arena.Reset()
	if create := ch.Change.GetCreate(); create != nil {
		return ss.applyCreate(ctx, ch)
	}
	if modify := ch.Change.GetModify(); modify != nil {
		return ss.applyModify(ctx, ch)
	}
	if del := ch.Change.GetDelete(); del != nil {
		return ss.applyDelete(ctx, ch)
	}
	log.Warn("got unexpected store change", zap.String("change", pbtypes.Sprint(ch.Change)))
	return
}

func (ss *StoreState) applyCreate(ctx context.Context, ch Change) (err error) {
	create := ch.Change.GetCreate()

	handler, err := ss.getHandler(create.Collection)
	if err != nil {
		return
	}

	if create.DocumentId == IdFromChange {
		create.DocumentId = ch.Id
	}
	// parse value and force set id
	jsonValue, err := ss.parser.Parse(create.Value)
	if err != nil {
		return
	}
	value := ss.arena.NewFromFastJson(jsonValue)
	value.Set("id", ss.arena.NewString(create.DocumentId))
	// call handler
	if err = handler.BeforeCreate(ctx, ss.changeOp(ch, value)); err != nil {
		return
	}

	// fill initial order
	fillRootOrder(ss.arena, value, ch.Order)

	// insert
	coll, err := ss.Collection(ctx, create.Collection)
	if err != nil {
		return err
	}

	if err = coll.Insert(ctx, value); err != nil {
		if errors.Is(err, anystore.ErrDocExists) {
			return ErrIgnore
		}
		return
	}
	return
}

func (ss *StoreState) applyModify(ctx context.Context, ch Change) (err error) {
	modify := ch.Change.GetModify()

	handler, err := ss.getHandler(modify.Collection)
	if err != nil {
		return
	}

	mod, err := makeModifier(ss.changeOp(ch, nil), handler)
	if err != nil {
		return
	}

	changeOp := ss.changeOp(ch, nil)

	mode, err := handler.BeforeModify(ctx, changeOp)
	if err != nil {
		return
	}

	coll, err := ss.Collection(ctx, modify.Collection)
	if err != nil {
		return
	}

	var exec func(ctx context.Context, id any, m query.Modifier) (anystore.ModifyResult, error)
	if mode == ModifyModeUpsert {
		exec = coll.UpsertId
	} else {
		exec = coll.UpdateId
	}

	// TODO: check result?
	_, err = exec(ctx, modify.DocumentId, mod)
	if err != nil {
		if errors.Is(err, anystore.ErrDocNotFound) {
			return ErrLog
		} else {
			return
		}
	}
	return
}

func (ss *StoreState) applyDelete(ctx context.Context, ch Change) (err error) {
	del := ch.Change.GetDelete()

	handler, err := ss.getHandler(del.Collection)
	if err != nil {
		return
	}

	mode, err := handler.BeforeDelete(ctx, ss.changeOp(ch, nil))
	if err != nil {
		return
	}

	coll, err := ss.Collection(ctx, del.Collection)
	if err != nil {
		return
	}
	switch mode {
	case DeleteModeMark:
		payload := ss.newDeleteMark(del.DocumentId)
		fillRootOrder(ss.arena, payload, ch.Order)
		return coll.UpdateOne(ctx, payload)
	case DeleteModeDelete:
		err = coll.DeleteId(ctx, del.DocumentId)
		if errors.Is(err, anystore.ErrDocNotFound) {
			return nil
		}
		return err
	}
	return
}

func (ss *StoreState) changeOp(ch Change, val *anyenc.Value) ChangeOp {
	return ChangeOp{
		Change: ch,
		State:  ss,
		Value:  val,
		Arena:  ss.arena,
	}
}

func (ss *StoreState) newDeleteMark(id string) *anyenc.Value {
	obj := ss.arena.NewObject()
	obj.Set("id", ss.arena.NewString(id))
	obj.Set("_d", ss.arena.NewNumberInt(int(time.Now().UnixMilli())))
	return obj
}

func (ss *StoreState) getHandler(collection string) (Handler, error) {
	if h, ok := ss.handlers[collection]; ok {
		return h, nil
	}
	return nil, errors.Join(ErrLog, ErrUnexpectedHandler, fmt.Errorf("'%s'", collection))
}
