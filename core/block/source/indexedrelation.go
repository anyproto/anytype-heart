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
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/gogo/protobuf/types"
)

func NewIndexedRelation(a core.Service, id string) (s Source) {
	return &indexedRelation{
		id: id,
		a:  a,
	}
}

type indexedRelation struct {
	id string
	a  core.Service
}

func (v *indexedRelation) ReadOnly() bool {
	// should be false if we proxy relation via the object type
	return true
}

func (v *indexedRelation) Id() string {
	return v.id
}

func (v *indexedRelation) Anytype() core.Service {
	return v.a
}

func (v *indexedRelation) Type() model.SmartBlockType {
	return model.SmartBlockType_IndexedRelation
}

func (v *indexedRelation) Virtual() bool {
	return false
}

func (v *indexedRelation) getDetails(id string) (rels []*relation.Relation, p *types.Struct, err error) {
	if !strings.HasPrefix(id, addr.CustomRelationURLPrefix) {
		return nil, nil, fmt.Errorf("incorrect relation id: not an indexed relation id")
	}

	key := strings.TrimPrefix(id, addr.CustomRelationURLPrefix)
	rel, err := v.Anytype().ObjectStore().GetRelation(key)
	if err != nil {
		return nil, nil, err
	}

	// todo: store source objectType and extract real profileId
	rel.Creator = v.a.ProfileID()
	rels, d := bundle.GetDetailsForRelation(false, rel)
	return rels, d, nil
}

func (v *indexedRelation) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
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

func (v *indexedRelation) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
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

func (v *indexedRelation) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *indexedRelation) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *indexedRelation) ListIds() ([]string, error) {
	return v.Anytype().ObjectStore().ListRelationsKeys()
}

func (v *indexedRelation) Close() (err error) {
	return
}
