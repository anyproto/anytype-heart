package source

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type missingObject struct {
}

func (s *service) NewMissingObject() Source {
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

func (m *missingObject) SpaceID() string {
	return addr.AnytypeMarketplaceWorkspace
}

func (m *missingObject) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeMissingObject
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
	return []string{"todo"}
}

func (m *missingObject) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (m *missingObject) PushChange(params PushChangeParams) (id string, err error) {
	return
}

func (m *missingObject) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
