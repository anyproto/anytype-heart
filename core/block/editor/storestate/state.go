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
	// Deprecated: CollChangeOrders is kept for migration purposes only.
	// New code should use CollMeta instead.
	CollChangeOrders = "_change_orders"

	// CollMeta is a shared collection storing per-object metadata (e.g. maxAddSeq).
	// Documents are keyed by object ID: {id: "<objectId>", q: <addSeq>}.
	CollMeta = "_meta"
)

func New(ctx context.Context, id string, db anystore.DB, handlers ...Handler) (state *StoreState, err error) {
	if len(handlers) == 0 {
		return nil, fmt.Errorf("should be at least one handler")
	}

	state = &StoreState{
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

	defer func() {
		if err != nil {
			if state.collMeta != nil {
				_ = state.collMeta.Close()
			}
			_ = tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	if err = state.init(tx.Context()); err != nil {
		return
	}
	return
}

type ChangeSet struct {
	Id        string
	Order     string
	Creator   string
	AclHeadId string
	Changes   []*pb.StoreChangeContent
	Timestamp int64
}

type Change struct {
	Id        string
	Order     string
	Creator   string
	AclHeadId string
	Change    *pb.StoreChangeContent
	Timestamp int64
}

type StoreState struct {
	id       string
	collMeta anystore.Collection

	handlers map[string]Handler

	arena  *anyenc.Arena
	parser *fastjson.Parser

	db anystore.DB
}

func (ss *StoreState) Id() string {
	return ss.id
}

func (ss *StoreState) init(ctx context.Context) (err error) {
	// Open shared meta collection (no object-ID prefix)
	if ss.collMeta, err = ss.db.Collection(ctx, CollMeta); err != nil {
		return
	}
	// Migration: drop old per-object _change_orders collection if it exists
	ss.dropOldChangeOrders(ctx)

	for _, h := range ss.handlers {
		if err = h.Init(ctx, ss); err != nil {
			return
		}
	}
	return
}

func (ss *StoreState) dropOldChangeOrders(ctx context.Context) {
	coll, err := ss.db.OpenCollection(ctx, ss.id+CollChangeOrders)
	if err != nil {
		return // Collection doesn't exist or other error — nothing to do
	}
	_ = coll.Drop(ctx)
}

// ResetReplayState makes the next store-tree open replay the complete change
// history. It is used by derived-index migrations that need events older than
// the incremental add-sequence watermark. Deleting only this object's metadata
// leaves its materialized collections intact; replay handlers are idempotent.
func ResetReplayState(ctx context.Context, db anystore.DB, objectId string) error {
	meta, err := db.Collection(ctx, CollMeta)
	if err != nil {
		return err
	}
	if err = meta.DeleteId(ctx, objectId); errors.Is(err, anystore.ErrDocNotFound) {
		return nil
	}
	return err
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

func (ss *StoreState) applyChangeSet(ctx context.Context, set ChangeSet, returnAllErrors bool) (err error) {
	for _, ch := range set.Changes {
		applyErr := ss.applyChange(ctx, Change{
			Id:        set.Id,
			Order:     set.Order,
			Change:    ch,
			Creator:   set.Creator,
			AclHeadId: set.AclHeadId,
			Timestamp: set.Timestamp,
		})
		if applyErr == nil || errors.Is(applyErr, ErrIgnore) {
			continue
		}
		if errors.Is(applyErr, ErrCritical) {
			err = applyErr
			break
		}
		if returnAllErrors {
			err = applyErr
			break
		}
		// Default: log and skip for forward compatibility
		log.Warn("change apply error",
			zap.Error(applyErr),
			zap.String("changeId", set.Id),
			zap.String("order", set.Order),
		)
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
		return errors.Join(ErrCritical, fmt.Errorf("get collection: %w", err))
	}

	if err = coll.Insert(ctx, value); err != nil {
		if errors.Is(err, anystore.ErrDocExists) {
			return ErrIgnore
		}
		return errors.Join(ErrCritical, fmt.Errorf("insert document: %w", err))
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
		return errors.Join(ErrCritical, fmt.Errorf("get collection: %w", err))
	}

	var exec func(ctx context.Context, id any, m query.Modifier) (anystore.ModifyResult, error)
	if mode == ModifyModeUpsert {
		exec = coll.UpsertId
	} else {
		exec = coll.UpdateId
	}

	_, err = exec(ctx, modify.DocumentId, wrapModifierErrors(mod))
	if err != nil {
		if errors.Is(err, anystore.ErrDocNotFound) {
			return nil
		}
		// An error from the modifier
		if errors.Is(err, errModifier) {
			return err
		}
		// An error from any-store
		return errors.Join(ErrCritical, fmt.Errorf("modify document: %w", err))
	}
	return nil
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
		return errors.Join(ErrCritical, fmt.Errorf("get collection: %w", err))
	}
	switch mode {
	case DeleteModeMark:
		payload := ss.newDeleteMark(del.DocumentId)
		fillRootOrder(ss.arena, payload, ch.Order)
		if err = coll.UpdateOne(ctx, payload); err != nil {
			return errors.Join(ErrCritical, fmt.Errorf("update document: %w", err))
		}
		return nil
	case DeleteModeDelete:
		err = coll.DeleteId(ctx, del.DocumentId)
		if errors.Is(err, anystore.ErrDocNotFound) {
			return nil
		}
		if err != nil {
			return errors.Join(ErrCritical, fmt.Errorf("delete document: %w", err))
		}
		return nil
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
	return nil, errors.Join(ErrUnexpectedHandler, fmt.Errorf("'%s'", collection))
}
