package source

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func NewBundledObjectType(a anytype.Service, id string) (s Source) {
	return &bundledObjectType{
		id: id,
		a:  a,
	}
}

type bundledObjectType struct {
	id string
	a  anytype.Service
}

func (v *bundledObjectType) Id() string {
	return v.id
}

func (v *bundledObjectType) Anytype() anytype.Service {
	return v.a
}

func (v *bundledObjectType) Type() pb.SmartBlockType {
	return pb.SmartBlockType_File
}

func (v *bundledObjectType) Virtual() bool {
	return false
}

func getDetailsForBundledObjectType(id string) (extraRels []*pbrelation.Relation, p *types.Struct, err error) {
	ot, err := bundle.GetTypeByUrl(id)
	if err != nil{
		return nil, nil, err
	}

	var relationKeys []string
	for i := range ot.Relations{
		extraRels = append(extraRels, ot.Relations[i])
		relationKeys = append(relationKeys, "_br"+ot.Relations[i].Key)
	}

	det := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyType.String(): pbtypes.String(bundle.TypeKeyObjectType.String()),
		bundle.RelationKeyName.String(): pbtypes.String(ot.Name),
		bundle.RelationKeyIconEmoji.String(): pbtypes.String(ot.IconEmoji),
		bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList(relationKeys),
		bundle.RelationKeyRecommendedLayout.String(): pbtypes.Float64(float64(ot.Layout)),
		bundle.RelationKeyDescription.String(): pbtypes.String(ot.Description),
		bundle.RelationKeyId.String(): pbtypes.String(id),
	}}

	return extraRels, det, nil
}

func (v *bundledObjectType) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(v.id, nil).(*state.State)


	rels, d, err := getDetailsForBundledObjectType(v.id)
	if err != nil {
		return nil, err
	}

	s.SetExtraRelations(rels)
	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyObjectType.URL())
	return s, nil
}

func (v *bundledObjectType) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
	s := &state.State{}

	rels, d, err := getDetailsForBundledObjectType(v.id)
	if err != nil {
		return nil, err
	}

	s.SetExtraRelations(rels)
	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyObjectType.URL())
	return s, nil
}

func (v *bundledObjectType) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *bundledObjectType) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *bundledObjectType) Close() (err error) {
	return
}
