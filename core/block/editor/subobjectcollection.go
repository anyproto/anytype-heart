package editor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var (
	ErrSubObjectNotFound      = errors.New("sub object not found")
	ErrCollectionNotFound     = errors.New("collection not found")
	ErrSubObjectAlreadyExists = fmt.Errorf("subobject already exists in the collection")
)

type SubObjectImpl interface {
	smartblock.SmartBlock
	SetStruct(*types.Struct) error
}

var localDetailsAllowedToBeStored = []string{
	bundle.RelationKeyType.String(),
	bundle.RelationKeyLastModifiedDate.String(),
	bundle.RelationKeyLastModifiedBy.String(),
}

type SubObjectCollection struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	dataview.Dataview
	stext.Text

	defaultCollectionName string
	collections           map[string]map[string]SubObjectImpl

	sourceService source.Service
	app           *app.App
}

func NewSubObjectCollection(defaultCollectionName string) *SubObjectCollection {
	sc := &SubObjectCollection{
		SmartBlock:            smartblock.New(),
		defaultCollectionName: defaultCollectionName,
		collections:           map[string]map[string]SubObjectImpl{},
	}

	sc.AllOperations = basic.NewBasic(sc.SmartBlock)
	sc.IHistory = basic.NewHistory(sc.SmartBlock)
	sc.Dataview = dataview.NewDataview(sc.SmartBlock)
	sc.Text = stext.NewText(sc.SmartBlock)
	return sc
}

func (c *SubObjectCollection) getCollectionAndKeyFromId(id string) (collection, key string) {
	parts := strings.Split(id, addr.SubObjectCollectionIdSeparator)
	if len(parts) == 1 {
		collection = c.defaultCollectionName
		key = parts[0]
	} else {
		collection = parts[0]
		key = parts[1]
	}
	return
}

func (c *SubObjectCollection) Open(subId string) (sb smartblock.SmartBlock, err error) {
	c.Lock()
	defer c.Unlock()

	collection, key := c.getCollectionAndKeyFromId(subId)
	if coll, exists := c.collections[collection]; exists {
		if sub, exists := coll[key]; exists {
			return sub, nil
		} else {
			return nil, ErrSubObjectNotFound
		}
	}

	return nil, ErrCollectionNotFound
}

func (c *SubObjectCollection) DeleteSubObject(objectId string) error {
	st := c.NewState()
	collection, key := c.getCollectionAndKeyFromId(objectId)
	err := c.ObjectStore().DeleteObject(objectId)
	if err != nil {
		log.Errorf("error deleting subobject from store %s %s %v", objectId, c.Id(), err.Error())
	}
	st.RemoveFromStore([]string{collection, key})
	return c.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (c *SubObjectCollection) removeObject(st *state.State, objectId string) (err error) {
	collection, key := c.getCollectionAndKeyFromId(objectId)
	// todo: check inbound links
	links, err := c.ObjectStore().GetInboundLinksById(objectId)
	if err != nil {
		return err
	}
	if len(links) > 0 {
		// todo: return the error to user?
		log.Errorf("workspace removeObject: found inbound links: %v", links)
	}
	st.RemoveFromStore([]string{collection, key})
	if v, exists := c.collections[collection]; exists {
		delete(v, key)
	}
	c.sourceService.RemoveStaticSource(objectId)

	err = c.ObjectStore().DeleteObject(objectId)
	if err != nil {
		return err
	}
	return nil
}

func (c *SubObjectCollection) Locked() bool {
	c.Lock()
	defer c.Unlock()
	if c.IsLocked() {
		return true
	}
	for _, coll := range c.collections {
		for _, sub := range coll {
			if sub.IsLocked() {
				return true
			}
		}
	}
	return false
}

func (c *SubObjectCollection) updateSubObject(info smartblock.ApplyInfo) (err error) {
	if len(info.Changes) == 0 {
		return nil
	}
	st := c.NewState()
	for _, ch := range info.Changes {
		if keySet := ch.GetStoreKeySet(); keySet != nil {
			if len(keySet.Path) >= 2 {
				if coll, ok := c.collections[keySet.Path[0]]; ok {
					if opt, ok := coll[keySet.Path[1]]; ok {
						if e := opt.SetStruct(pbtypes.GetStruct(c.NewState().GetCollection(keySet.Path[0]), keySet.Path[1])); e != nil {
							log.With("threadId", c.Id()).Errorf("options: can't set struct: %v", e)
						}
					} else {
						if err = c.initSubObject(st, keySet.Path[0], keySet.Path[1]); err != nil {
							return
						}
					}
				}
			}
		}
	}
	return
}

// cleanSubObjectDetails returns the new type.Struct but the values of fields are passed by reference
func cleanSubObjectDetails(details *types.Struct) *types.Struct {
	dataToSave := &types.Struct{Fields: map[string]*types.Value{}}
	for k, v := range details.GetFields() {
		r, _ := bundle.GetRelation(bundle.RelationKey(k))
		if r == nil {
			continue
		}
		if r.DataSource == model.Relation_details || slice.FindPos(localDetailsAllowedToBeStored, k) > -1 {
			dataToSave.Fields[k] = v
		}
	}
	return dataToSave
}

func (c *SubObjectCollection) onSubObjectChange(collection, subId string) func(p source.PushChangeParams) (string, error) {
	return func(p source.PushChangeParams) (string, error) {
		st := c.NewState()

		coll, exists := c.collections[collection]
		if !exists {
			return "", fmt.Errorf("collection not found")
		}

		if _, ok := coll[subId]; !ok {
			return "", fmt.Errorf("onSubObjectChange: subObject '%s' not exists in collection '%s'", subId, collection)
		}

		dataToSave := cleanSubObjectDetails(p.State.CombinedDetails())

		var notOnlyLocalDetailsChanged bool
		for k, _ := range dataToSave.Fields {
			if slice.FindPos(localDetailsAllowedToBeStored, k) == -1 {
				notOnlyLocalDetailsChanged = true
				break
			}
		}

		if !notOnlyLocalDetailsChanged {
			// todo: it shouldn't be done here, we have a place for it in the state, but it's not possible to set the virtual changes there
			// revert lastModifiedDate details
			prev := p.State.ParentState().LocalDetails().GetFields()[bundle.RelationKeyLastModifiedDate.String()]
			if prev != nil {
				dataToSave.Fields[bundle.RelationKeyLastModifiedDate.String()] = prev
			}
		}

		if !pbtypes.StructCompareIgnoreKeys(dataToSave, st.Store(), []string{bundle.RelationKeyLastModifiedDate.String()}) {
			return "", nil
		}

		if !pbtypes.StructCompareIgnoreKeys(dataToSave, st.Store(), localDetailsAllowedToBeStored) {
			return "", nil
		}

		changed := st.SetInStore([]string{collection, subId}, pbtypes.Struct(dataToSave))
		if !changed {
			return "", nil
		}
		return "", c.Apply(st, smartblock.NoHooks)
	}
}

func (c *SubObjectCollection) Init(ctx *smartblock.InitContext) error {
	c.app = ctx.App
	c.sourceService = c.app.MustComponent(source.CName).(source.Service)

	return c.SmartBlock.Init(ctx)
}

func (c *SubObjectCollection) initSubObject(st *state.State, collection string, subId string) (err error) {
	var subObj SubObjectImpl
	switch collection {
	case collectionKeyObjectTypes:
		subObj = NewObjectType()
	default:
		subObj = NewSubObject()
	}

	var fullId string
	if collection == "" || collection == c.defaultCollectionName {
		fullId = subId
		collection = c.defaultCollectionName
	} else {
		fullId = collection + addr.SubObjectCollectionIdSeparator + subId
	}

	ws := pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeyWorkspaceId.String())
	if ws == "" && c.Anytype().PredefinedBlocks().Account == st.RootId() {
		ws = st.RootId()
	}
	subState, err := SubState(st, collection, fullId, ws)
	if err != nil {
		return
	}

	if _, exists := c.collections[collection]; !exists {
		c.collections[collection] = map[string]SubObjectImpl{}
	}
	c.collections[collection][subId] = subObj
	if err = subObj.Init(&smartblock.InitContext{
		Source: c.sourceService.NewStaticSource(fullId, model.SmartBlockType_SubObject, subState, c.onSubObjectChange(collection, subId)),
		App:    c.app,
	}); err != nil {
		return
	}

	return
}

func SubState(st *state.State, collection string, fullId string, workspaceId string) (*state.State, error) {
	subId := strings.TrimPrefix(fullId, collection+addr.SubObjectCollectionIdSeparator)
	data := pbtypes.GetStruct(st.GetCollection(collection), subId)
	if data == nil || data.Fields == nil {
		return nil, fmt.Errorf("no data for subId %s: %v", collection, subId)
	}
	subst := structToState(fullId, data)

	relationsToCopy := []bundle.RelationKey{bundle.RelationKeyCreator}
	for _, rk := range relationsToCopy {
		subst.SetDetailAndBundledRelation(rk, pbtypes.String(pbtypes.GetString(st.CombinedDetails(), rk.String())))
	}
	subst.AddBundledRelations(bundle.RelationKeyLastModifiedDate, bundle.RelationKeyLastOpenedDate)
	subst.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(workspaceId))
	return subst, nil
}

func structToState(id string, data *types.Struct) *state.State {
	blocks := map[string]simple.Block{
		id: simple.New(&model.Block{Id: id, ChildrenIds: []string{}}),
	}
	subState := state.NewDoc(id, blocks).(*state.State)

	for k, v := range data.Fields {
		if rel, err := bundle.GetRelation(bundle.RelationKey(k)); err == nil {
			if rel.DataSource == model.Relation_details || slice.FindPos(localDetailsAllowedToBeStored, k) > -1 {
				subState.SetDetailAndBundledRelation(bundle.RelationKey(k), v)
			}
		}
	}
	subState.SetDetailAndBundledRelation(bundle.RelationKeyId, pbtypes.String(id))
	subState.SetObjectType(pbtypes.GetString(data, bundle.RelationKeyType.String()))
	return subState
}
