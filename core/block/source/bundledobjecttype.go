package source

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewBundledObjectType(a core.Service, id string) (s Source) {
	return &bundledObjectType{
		id: id,
		a:  a,
	}
}

type bundledObjectType struct {
	id string
	a  core.Service
}

func (v *bundledObjectType) ReadOnly() bool {
	return true
}

func (v *bundledObjectType) Id() string {
	return v.id
}

func (v *bundledObjectType) Anytype() core.Service {
	return v.a
}

func (v *bundledObjectType) Type() model.SmartBlockType {
	return model.SmartBlockType_BundledObjectType
}

func (v *bundledObjectType) Virtual() bool {
	return true
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

func (v *bundledObjectType) LogHeads() map[string]string {
	return nil
}

func (s *bundledObjectType) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}
