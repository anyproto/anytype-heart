package source

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"strings"
)

const bundledRelationPrefix = "_br"

func NewBundledRelation(a anytype.Service, id string) (s Source) {
	return &bundledRelation{
		id: id,
		a:  a,
	}
}

type bundledRelation struct {
	id string
	a  anytype.Service
}

func (v *bundledRelation) Id() string {
	return v.id
}

func (v *bundledRelation) Anytype() anytype.Service {
	return v.a
}

func (v *bundledRelation) Type() pb.SmartBlockType {
	return pb.SmartBlockType_File
}

func (v *bundledRelation) Virtual() bool {
	return false
}

func getDetailsForRelation(prefix string, rel *relation.Relation) *types.Struct {
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String(): pbtypes.String(rel.Name),
		bundle.RelationKeyDescription.String(): pbtypes.String(rel.Description),
		bundle.RelationKeyId.String(): pbtypes.String(prefix+rel.Key),
		bundle.RelationKeyLayout.String(): pbtypes.Float64(float64(relation.ObjectType_relation)),
		"isHidden": pbtypes.Bool(rel.Hidden),
	}}
}

func (v *bundledRelation) getDetails(id string) (p *types.Struct, err error) {
	if !strings.HasPrefix(id, bundledRelationPrefix) {
		return nil, fmt.Errorf("incorrect relation id: not a bundled relation id")
	}

	rel, err := bundle.GetRelation(bundle.RelationKey(strings.TrimPrefix(id, bundledRelationPrefix)))
	if err != nil{
		return nil, err
	}

	return getDetailsForRelation(bundledRelationPrefix, rel), nil
}

func (v *bundledRelation) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(v.id, nil).(*state.State)

	d, err := v.getDetails(v.id)
	if err != nil {
		return nil, err
	}

	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyRelation.URL())
	return s, nil
}

func (v *bundledRelation) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
	s := &state.State{}

	d, err := v.getDetails(v.id)
	if err != nil {
		return nil, err
	}

	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyRelation.URL())
	return s, nil
}

func (v *bundledRelation) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *bundledRelation) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *bundledRelation) Close() (err error) {
	return
}
