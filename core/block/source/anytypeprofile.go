package source

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) NewAnytypeProfile(id string) Source {
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

func (v *anytypeProfile) getDetails() (p *types.Struct) {
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():        pbtypes.String("Anytype"),
		bundle.RelationKeyDescription.String(): pbtypes.String("Authored by Anytype team"),
		bundle.RelationKeyIconImage.String():   pbtypes.String("bafybeihdxbwosreebqthjccgjygystk2mgg3ebrctv2j36xghaawnqrz5e"),
		bundle.RelationKeyId.String():          pbtypes.String(v.id),
		bundle.RelationKeyIsReadonly.String():  pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String():  pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():    pbtypes.Bool(true),
		bundle.RelationKeyLayout.String():      pbtypes.Float64(float64(model.ObjectType_profile)),
	}}
}

func (v *anytypeProfile) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(v.id, nil).(*state.State)

	d := v.getDetails()

	s.SetDetails(d)

	// todo: add object type
	// s.SetObjectTypeKey(v.coreService.PredefinedObjects(v.spaceID).SystemTypes[bundle.TypeKeyDate])
	return s, nil
}

func (v *anytypeProfile) ReadMeta(ctx context.Context, r ChangeReceiver) (doc state.Doc, err error) {
	return v.ReadDoc(ctx, r, false)
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

func (s *anytypeProfile) PushChange(params PushChangeParams) (id string, err error) {
	return
}

func (s *anytypeProfile) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
