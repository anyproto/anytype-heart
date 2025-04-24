package sourceimpl

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

type missingObject struct {
}

func NewMissingObject() (s source.Source) {
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

func (m *missingObject) getDetails() (p *domain.Details) {
	det := domain.NewDetails()
	det.SetString(bundle.RelationKeyId, addr.MissingObject)
	det.SetBool(bundle.RelationKeyIsDeleted, true)
	det.SetBool(bundle.RelationKeyIsReadonly, true)
	det.SetBool(bundle.RelationKeyIsHidden, true)
	return det
}

func (m *missingObject) ReadDoc(ctx context.Context, receiver source.ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(addr.MissingObject, nil).(*state.State)

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

func (m *missingObject) PushChange(params source.PushChangeParams) (id string, err error) {
	return
}

func (m *missingObject) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
