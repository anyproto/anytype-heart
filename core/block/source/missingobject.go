package source

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type missingObject struct {
}

func NewMissingObject() (s Source) {
	return &missingObject{}
}

func (m *missingObject) ListIds() ([]string, error) {
	return []string{addr.MissingObject}, nil
}

func (m *missingObject) ReadOnly() bool {
	return true
}

// nolint:revive
func (m *missingObject) Id() string {
	return addr.MissingObject
}

func (m *missingObject) Type() model.SmartBlockType {
	return model.SmartBlockType_MissingObject
}

func (m *missingObject) getDetails() (p *types.Struct) {
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyIsDeleted.String():  pbtypes.Bool(true),
		bundle.RelationKeyId.String():         pbtypes.String(addr.MissingObject),
		bundle.RelationKeyIsReadonly.String(): pbtypes.Bool(true),
		bundle.RelationKeyIsHidden.String():   pbtypes.Bool(true),
	}}
}

func (m *missingObject) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(addr.MissingObject, nil).(*state.State)

	d := m.getDetails()

	s.SetDetails(d)
	return s, nil
}

func (m *missingObject) ReadMeta(ctx context.Context, _ ChangeReceiver) (doc state.Doc, err error) {
	s := &state.State{}
	d := m.getDetails()

	s.SetDetails(d)
	return s, nil
}

func (m *missingObject) Close() (err error) {
	return
}

func (m *missingObject) Heads() []string {
	return nil
}

func (m *missingObject) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (m *missingObject) PushChange(params PushChangeParams) (id string, err error) {
	return
}
