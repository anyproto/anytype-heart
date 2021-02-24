package source

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/gogo/protobuf/types"
	"strings"
)

const indexedRelationPrefix = "_ir"

func NewIndexedRelation(a anytype.Service, id string) (s Source) {
	return &indexedRelation{
		id: id,
		a:  a,
	}
}

type indexedRelation struct {
	id string
	a  anytype.Service
}

func (v *indexedRelation) Id() string {
	return v.id
}

func (v *indexedRelation) Anytype() anytype.Service {
	return v.a
}

func (v *indexedRelation) Type() pb.SmartBlockType {
	return pb.SmartBlockType_Page
}

func (v *indexedRelation) Virtual() bool {
	return false
}

func (v *indexedRelation) getDetails(id string) (p *types.Struct, err error) {
	if !strings.HasPrefix(id, indexedRelationPrefix){
		return nil, fmt.Errorf("incorrect relation id: not an indexed relation id")
	}

	key := strings.TrimPrefix(id, indexedRelationPrefix)
	rel, err := v.Anytype().ObjectStore().GetRelation(key)
	if err != nil {
		return nil, err
	}

	return getDetailsForRelation(indexedRelationPrefix, rel), nil
}

func (v *indexedRelation) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(v.id, nil).(*state.State)

	d, err := v.getDetails(v.id)
	if err != nil {
		return nil, err
	}

	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyRelation.URL())
	return s, nil
}

func (v *indexedRelation) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
	s := &state.State{}

	d, err := v.getDetails(v.id)
	if err != nil {
		return nil, err
	}

	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyRelation.URL())
	return s, nil
}

func (v *indexedRelation) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *indexedRelation) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *indexedRelation) Close() (err error) {
	return
}
