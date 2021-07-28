package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
)

func NewBundledRelation(a core.Service, id string) (s Source) {
	return &bundledRelation{
		id: id,
		a:  a,
	}
}

type bundledRelation struct {
	id string
	a  core.Service
}

func (v *bundledRelation) ReadOnly() bool {
	return true
}

func (v *bundledRelation) Id() string {
	return v.id
}

func (v *bundledRelation) Anytype() core.Service {
	return v.a
}

func (v *bundledRelation) Type() model.SmartBlockType {
	return model.SmartBlockType_BundledRelation
}

func (v *bundledRelation) Virtual() bool {
	return true
}

func (v *bundledRelation) getDetails(id string) (rels []*model.Relation, p *types.Struct, err error) {
	if !strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return nil, nil, fmt.Errorf("incorrect relation id: not a bundled relation id")
	}

	rel, err := bundle.GetRelation(bundle.RelationKey(strings.TrimPrefix(id, addr.BundledRelationURLPrefix)))
	if err != nil {
		return nil, nil, err
	}

	rel.Creator = addr.AnytypeProfileId
	rels, d := bundle.GetDetailsForRelation(true, rel)
	return rels, d, nil
}

func (v *bundledRelation) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(v.id, nil).(*state.State)

	rels, d, err := v.getDetails(v.id)
	if err != nil {
		return nil, err
	}

	s.SetDetails(d)
	s.SetExtraRelations(rels)
	s.SetObjectType(bundle.TypeKeyRelation.URL())
	return s, nil
}

func (v *bundledRelation) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
	s := &state.State{}

	rels, d, err := v.getDetails(v.id)
	if err != nil {
		return nil, err
	}

	s.SetDetails(d)
	s.SetExtraRelations(rels)
	s.SetObjectType(bundle.TypeKeyRelation.URL())
	return s, nil
}

func (v *bundledRelation) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *bundledRelation) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *bundledRelation) ListIds() ([]string, error) {
	return bundle.ListRelationsUrls(), nil
}

func (v *bundledRelation) Close() (err error) {
	return
}
