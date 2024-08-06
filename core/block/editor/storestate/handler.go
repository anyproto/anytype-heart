package storestate

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-store/query"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/pb"
)

type ChangeOp struct {
	Change Change
	State  *StoreState
	Value  *fastjson.Value
	Arena  *fastjson.Arena
}

type DeleteMode uint

const (
	DeleteModeDelete DeleteMode = iota
	DeleteModeMark
)

type ModifyMode uint

const (
	ModifyModeUpdate ModifyMode = iota
	ModifyModeUpsert
)

type Handler interface {
	CollectionName() string
	Init(ctx context.Context, s *StoreState) (err error)
	BeforeCreate(ctx context.Context, ch ChangeOp) (err error)
	BeforeModify(ctx context.Context, ch ChangeOp) (mode ModifyMode, err error)
	BeforeDelete(ctx context.Context, ch ChangeOp) (mode DeleteMode, err error)
	UpgradeKeyModifier(ch ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier
}

type DefaultHandler struct {
	Name       string
	ModifyMode ModifyMode
	DeleteMode DeleteMode
}

func (d DefaultHandler) CollectionName() string {
	return d.Name
}

func (d DefaultHandler) Init(ctx context.Context, s *StoreState) (err error) {
	_, err = s.Collection(ctx, d.Name)
	return
}

func (d DefaultHandler) BeforeCreate(ctx context.Context, ch ChangeOp) (err error) {
	return
}

func (d DefaultHandler) BeforeModify(ctx context.Context, ch ChangeOp) (mode ModifyMode, err error) {
	return d.ModifyMode, nil
}

func (d DefaultHandler) BeforeDelete(ctx context.Context, ch ChangeOp) (mode DeleteMode, err error) {
	return d.DeleteMode, nil
}

func (d DefaultHandler) UpgradeKeyModifier(ch ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier {
	return mod
}

// TODO Move out here

type ChatHandler struct {
	Name       string
	MyIdentity string
}

func (d ChatHandler) CollectionName() string {
	return d.Name
}

func (d ChatHandler) Init(ctx context.Context, s *StoreState) (err error) {
	_, err = s.Collection(ctx, d.Name)
	return
}

func (d ChatHandler) BeforeCreate(ctx context.Context, ch ChangeOp) (err error) {
	return
}

func (d ChatHandler) BeforeModify(ctx context.Context, ch ChangeOp) (mode ModifyMode, err error) {
	return ModifyModeUpsert, nil
}

func (d ChatHandler) BeforeDelete(ctx context.Context, ch ChangeOp) (mode DeleteMode, err error) {
	return DeleteModeDelete, nil
}

func (d ChatHandler) UpgradeKeyModifier(ch ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier {
	return query.ModifyFunc(func(a *fastjson.Arena, v *fastjson.Value) (result *fastjson.Value, modified bool, err error) {
		author := v.GetStringBytes("author")
		if string(author) != d.MyIdentity {
			return v, false, errors.Join(ErrIgnore, fmt.Errorf("can't modify not own message"))
		}
		return mod.Modify(a, v)
	})
}
