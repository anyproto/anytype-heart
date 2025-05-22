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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TODO Is it used?
func NewAnytypeProfile(id string) (s source.Source) {
	return &anytypeProfile{
		id: id,
	}
}

type anytypeProfile struct {
	id string
}

func (v *anytypeProfile) ListIds() ([]string, error) {
	return []string{addr.AnytypeProfileId}, nil
}

func (v *anytypeProfile) ReadOnly() bool {
	return true
}

func (v *anytypeProfile) Id() string {
	return v.id
}

func (v *anytypeProfile) SpaceID() string {
	return addr.AnytypeMarketplaceWorkspace
}

func (v *anytypeProfile) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeAnytypeProfile
}

func (v *anytypeProfile) getDetails() (p *domain.Details) {
	det := domain.NewDetails()

	det.SetString(bundle.RelationKeyName, "Anytype")
	det.SetString(bundle.RelationKeyDescription, "Authored by Anytype team")
	det.SetString(bundle.RelationKeyIconImage, "bafybeihdxbwosreebqthjccgjygystk2mgg3ebrctv2j36xghaawnqrz5e")
	det.SetString(bundle.RelationKeyId, v.id)
	det.SetBool(bundle.RelationKeyIsReadonly, true)
	det.SetBool(bundle.RelationKeyIsArchived, false)
	det.SetBool(bundle.RelationKeyIsHidden, true)
	det.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_profile))
	return det
}

func (v *anytypeProfile) ReadDoc(ctx context.Context, receiver source.ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(v.id, nil).(*state.State)

	d := v.getDetails()

	s.SetDetails(d)

	// todo: add object type
	// s.SetObjectTypeKey(v.coreService.PredefinedObjects(v.spaceID).SystemTypes[bundle.TypeKeyDate])
	return s, nil
}

func (v *anytypeProfile) Close() (err error) {
	return
}

func (v *anytypeProfile) Heads() []string {
	return []string{"todo"} // todo hash of details
}

func (s *anytypeProfile) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *anytypeProfile) PushChange(params source.PushChangeParams) (id string, err error) {
	return
}

func (s *anytypeProfile) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
