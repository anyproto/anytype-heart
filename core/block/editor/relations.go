package editor

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"strings"
)

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

	key = bson.NewObjectId().Hex()
	id = addr.RelationKeyToIdPrefix + key
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)

	st := w.NewState()
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
	subState, err := w.subState(st, collectionKeyRelations, addr.RelationKeyToIdPrefix+relationKey)
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

func (w *Workspaces) subState(st *state.State, collection string, id string) (*state.State, error) {
	var subId string
	if collection == collectionKeyRelations {
		subId = strings.TrimPrefix(id, addr.RelationKeyToIdPrefix)
	} else {
		subId = id
	}
	data := pbtypes.GetStruct(st.GetCollection(collection), subId)
	if data == nil || data.Fields == nil {
		return nil, fmt.Errorf("no data for subId %s: %v", collection, subId)
	}
	subState := state.NewDoc(id, nil).(*state.State)
	for k, v := range data.Fields {
		if _, err := bundle.GetRelation(bundle.RelationKey(k)); err == nil {
			subState.SetDetailAndBundledRelation(bundle.RelationKey(k), v)
		}
	}
	subState.SetObjectType(pbtypes.GetString(data, bundle.RelationKeyType.String()))
	return subState, nil
}

func (w *Workspaces) onRelationChange(params source.PushChangeParams) (changeId string, err error) {
	st := w.NewState()
	id := params.State.RootId()
	subId := strings.TrimPrefix(id, addr.RelationKeyToIdPrefix)
	if _, ok := w.relations[subId]; !ok {
		return "", fmt.Errorf("onRelationChange: relation not exists")
	}
	st.SetInStore([]string{collectionKeyRelations, subId}, pbtypes.Struct(params.State.CombinedDetails()))
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
	return smartblock.ObjectApplyTemplate(o, ctx.State,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyRelation.URL()}),
	)
}

func (o *Relation) SetStruct(st *types.Struct) error {
	o.Lock()
	defer o.Unlock()
	s := o.NewState()
	s.SetDetails(st)
	return o.Apply(s)
}
