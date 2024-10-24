package storestate

import (
	"context"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/pb"
)

type ChangeOp struct {
	Change Change
	State  *StoreState
	Value  *anyenc.Value
	Arena  *anyenc.Arena
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
