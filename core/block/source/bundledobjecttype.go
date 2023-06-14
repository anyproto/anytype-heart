package source

import (
	"context"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func NewBundledObjectType(id string) (s Source) {
	return &bundledObjectType{
		id: id,
	}
}

type bundledObjectType struct {
	id string
}

func (v *bundledObjectType) ReadOnly() bool {
	return true
}

func (v *bundledObjectType) Id() string {
	return v.id
}

func (v *bundledObjectType) Type() model.SmartBlockType {
	return model.SmartBlockType_BundledObjectType
}

func getDetailsForBundledObjectType(id string) (extraRels []*model.RelationLink, p *types.Struct, err error) {
	ot, err := bundle.GetTypeByUrl(id)
	if err != nil {
		return nil, nil, err
	}
	extraRels = []*model.RelationLink{bundle.MustGetRelationLink(bundle.RelationKeyRecommendedRelations), bundle.MustGetRelationLink(bundle.RelationKeyRecommendedLayout)}

	for i := range ot.RelationLinks {
		extraRels = append(extraRels, ot.RelationLinks[i])
	}

	return extraRels, (&relationutils.ObjectType{ot}).ToStruct(), nil
}

func (v *bundledObjectType) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(v.id, nil).(*state.State)

	rels, d, err := getDetailsForBundledObjectType(v.id)
	if err != nil {
		return nil, err
	}
	for _, r := range rels {
		s.AddRelationLinks(&model.RelationLink{Format: r.Format, Key: r.Key})
	}
	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyObjectType.BundledURL())
	return s, nil
}

func (v *bundledObjectType) ReadMeta(ctx context.Context, _ ChangeReceiver) (doc state.Doc, err error) {
	return v.ReadDoc(ctx, nil, false)
}

func (v *bundledObjectType) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *bundledObjectType) ListIds() ([]string, error) {
	var ids []string
	for _, tk := range bundle.ListTypesKeys() {
		ids = append(ids, tk.BundledURL())
	}
	return ids, nil
}

func (v *bundledObjectType) Close() (err error) {
	return
}

func (v *bundledObjectType) Heads() []string {
	return []string{"todo"} // todo hash of model
}

func (s *bundledObjectType) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *bundledObjectType) GetCreationInfo() (creator string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
