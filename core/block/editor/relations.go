package editor

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"strings"
)

var ErrSubObjectAlreadyExists = fmt.Errorf("subobject already exists in the collection")

func (w *Workspaces) CreateRelation(details *types.Struct) (id, key string, err error) {
	if details == nil || details.Fields == nil {
		return "", "", fmt.Errorf("create relation: no data")
	}

	if v, ok := details.GetFields()[bundle.RelationKeyRelationFormat.String()]; !ok {
		return "", "", fmt.Errorf("missing relation format")
	} else if i, ok := v.Kind.(*types.Value_NumberValue); !ok {
		return "", "", fmt.Errorf("invalid relation format: not a number")
	} else if model.RelationFormat(int(i.NumberValue)).String() == "" {
		return "", "", fmt.Errorf("invalid relation format: unknown enum")
	}

	if pbtypes.GetString(details, bundle.RelationKeyName.String()) == "" {
		return "", "", fmt.Errorf("missing relation name")
	}

	if pbtypes.GetString(details, bundle.RelationKeyType.String()) != bundle.TypeKeyRelation.URL() {
		return "", "", fmt.Errorf("incorrect object type")
	}
	key = pbtypes.GetString(details, bundle.RelationKeyRelationKey.String())
	st := w.NewState()
	if key == "" {
		key = bson.NewObjectId().Hex()
	} else {
		// no need to check for the generated bson's
		if st.HasInStore([]string{collectionKeyRelations, key}) {
			return id, key, ErrSubObjectAlreadyExists
		}
	}
	id = addr.RelationKeyToIdPrefix + key
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relationOption))

	st.SetInStore([]string{collectionKeyRelations, key}, pbtypes.Struct(details))
	if err = w.initRelation(st, key); err != nil {
		return
	}
	if err = w.Apply(st, smartblock.NoHooks); err != nil {
		return
	}
	return
}

func (w *Workspaces) initRelation(st *state.State, relationKey string) (err error) {
	rel := NewRelation()
	subState, err := smartblock.SubState(st, collectionKeyRelations, addr.RelationKeyToIdPrefix+relationKey)
	if err != nil {
		return
	}

	w.relations[relationKey] = rel
	if err = rel.Init(&smartblock.InitContext{
		Source: w.sourceService.NewStaticSource(addr.RelationKeyToIdPrefix+relationKey, model.SmartBlockType_SubObjectRelation, subState, w.onRelationChange),
		App:    w.app,
	}); err != nil {
		return
	}
	return
}

func (w *Workspaces) onRelationChange(params source.PushChangeParams) (changeId string, err error) {
	st := w.NewState()
	id := params.State.RootId()
	subId := strings.TrimPrefix(id, addr.RelationKeyToIdPrefix)
	if _, ok := w.relations[subId]; !ok {
		return "", fmt.Errorf("onRelationChange: relation not exists")
	}
	changed := st.SetInStore([]string{collectionKeyRelations, subId}, pbtypes.Struct(params.State.CombinedDetails()))
	if id == "rel-artist" {
		fmt.Println()
	}
	if !changed {
		return "", nil
	}
	return "", w.Apply(st, smartblock.NoHooks)
}

func NewRelation() *Relation {
	return &Relation{
		SmartBlock: smartblock.New(),
	}
}

type Relation struct {
	smartblock.SmartBlock
}

func (o *Relation) Init(ctx *smartblock.InitContext) (err error) {
	if err = o.SmartBlock.Init(ctx); err != nil {
		return
	}
	return nil
}

func (o *Relation) SetStruct(st *types.Struct) error {
	o.Lock()
	defer o.Unlock()
	s := o.NewState()
	s.SetDetails(st)
	return o.Apply(s)
}
