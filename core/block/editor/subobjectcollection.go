package editor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
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
	bundle.RelationKeyCreatedDate.String(),
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

	app              *app.App
	sourceService    source.Service
	objectStore      objectstore.ObjectStore
	anytype          core.Service
	relationService  relation2.Service
	fileBlockService file.BlockService
}

func NewSubObjectCollection(
	defaultCollectionName string,
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	relationService relation2.Service,
	sourceService source.Service,
	fileBlockService file.BlockService,
) *SubObjectCollection {
	sb := smartblock.New()
	return &SubObjectCollection{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			objectStore,
		),
		Dataview: dataview.NewDataview(
			sb,
			anytype,
			objectStore,
			relationService,
		),

		objectStore:           objectStore,
		sourceService:         sourceService,
		anytype:               anytype,
		relationService:       relationService,
		fileBlockService:      fileBlockService,
		defaultCollectionName: defaultCollectionName,
		collections:           map[string]map[string]SubObjectImpl{},
	}
}

func (c *SubObjectCollection) Init(ctx *smartblock.InitContext) error {
	c.app = ctx.App

	return c.SmartBlock.Init(ctx)
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
	err := c.objectStore.DeleteObject(objectId)
	if err != nil {
		log.Errorf("error deleting subobject from store %s %s %v", objectId, c.Id(), err.Error())
	}
	st.RemoveFromStore([]string{collection, key})
	return c.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (c *SubObjectCollection) removeObject(st *state.State, objectId string) (err error) {
	collection, key := c.getCollectionAndKeyFromId(objectId)
	// todo: check inbound links
	links, err := c.objectStore.GetInboundLinksById(objectId)
	if err != nil {
		return err
	}
	if len(links) > 0 {
		log.With("id", objectId).With("total", len(links)).Debugf("workspace removeObject: found inbound links: %v", links)
	}
	st.RemoveFromStore([]string{collection, key})
	if v, exists := c.collections[collection]; exists {
		if o, exists := v[key]; exists {
			o.SetIsDeleted()
			o.Close()
			delete(v, key)
		}
	}
	c.sourceService.RemoveStaticSource(objectId)

	err = c.objectStore.DeleteObject(objectId)
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
							log.With("threadId", c.Id()).Errorf("options: can't set struct %s-%s: %v", keySet.Path[0], keySet.Path[1], e)
						}
					} else {
						if err = c.initSubObject(st, keySet.Path[0], keySet.Path[1], false); err != nil {
							return
						}
					}
				}
			}
		} else if keyUnset := ch.GetStoreKeyUnset(); keyUnset != nil {
			err = c.removeObject(st, strings.Join(keyUnset.Path, addr.SubObjectCollectionIdSeparator))
			if err != nil {
				log.With("threadId", c.Id()).Errorf("failed to remove object %s: %v", strings.Join(keyUnset.Path, addr.SubObjectCollectionIdSeparator), err)
				return err
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

		var hasPersistentDetails bool
		for k, _ := range dataToSave.Fields {
			if slice.FindPos(append(bundle.LocalRelationsKeys, bundle.DerivedRelationsKeys...), k) == -1 ||
				slice.FindPos(localDetailsAllowedToBeStored, k) > -1 {
				hasPersistentDetails = true
				break
			}
		}
		prevSubState := pbtypes.GetStruct(st.GetCollection(collection), subId)

		if !hasPersistentDetails {
			// todo: it shouldn't be done here, we have a place for it in the state, but it's not possible to set the virtual changes there
			// revert lastModifiedDate details
			if prevSubState.GetFields() != nil && prevSubState.Fields[bundle.RelationKeyLastModifiedDate.String()] != nil {
				dataToSave.Fields[bundle.RelationKeyLastModifiedDate.String()] = prevSubState.Fields[bundle.RelationKeyLastModifiedDate.String()]
			}
		}

		// ignore lastModifiedDate if this is the only thing that has changed
		if pbtypes.StructCompareIgnoreKeys(dataToSave, prevSubState, []string{bundle.RelationKeyLastModifiedDate.String()}) {
			// nothing changed
			return "", nil
		}

		changed := st.SetInStore([]string{collection, subId}, pbtypes.Struct(dataToSave))
		if !changed {
			return "", nil
		}
		return "", c.Apply(st, smartblock.NoHooks)
	}
}

func (c *SubObjectCollection) initSubObject(st *state.State, collection string, subId string, justCreated bool) (err error) {
	var subObj SubObjectImpl
	if len(strings.Split(subId, addr.SubObjectCollectionIdSeparator)) > 1 {
		// handle invalid cases for our own accounts
		return fmt.Errorf("invalid id: %s", subId)
	}

	switch collection {
	case collectionKeyObjectTypes:
		subObj = NewObjectType(c.anytype, c.objectStore, c.relationService)
	default:
		ot, ok := collectionKeyToObjectType(collection)
		if !ok {
			return fmt.Errorf("unknown collection '%s'", collection)
		}
		subObj = NewSubObject(c.objectStore, c.fileBlockService, c.anytype, c.relationService, ot)
	}

	var fullId string
	if collection == "" || collection == c.defaultCollectionName {
		fullId = subId
		collection = c.defaultCollectionName
	} else {
		fullId = collection + addr.SubObjectCollectionIdSeparator + subId
	}

	ws := pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeyWorkspaceId.String())
	if ws == "" && c.anytype.PredefinedBlocks().Account == st.RootId() {
		ws = st.RootId()
	}
	if v := st.StoreKeysRemoved(); v != nil {
		if _, exists := v[fullId]; exists {
			log.Errorf("initSubObject %s: found keyremoved, calling removeObject", fullId)
			return c.removeObject(st, fullId)
		}
	}

	storedDetails, err := c.objectStore.GetDetails(fullId)
	if storedDetails.GetDetails() != nil && pbtypes.GetBool(storedDetails.Details, bundle.RelationKeyIsDeleted.String()) {
		// we have removed this subobject previously, so let's removed stored details(with isDeleted=true) so it will not be injected to the new subobject
		err = c.objectStore.DeleteDetails(fullId)
		if err != nil {
			log.Errorf("initSubObject %s: failed to delete deleted details: %v", fullId, err)
		}
	}

	subState, err := SubState(st, collection, fullId, ws)
	if err != nil {
		return
	}

	if justCreated {
		det := subState.CombinedDetails()
		internalflag.PutToDetails(det, []*model.InternalFlag{{Value: model.InternalFlag_editorDeleteEmpty}})
		subState.SetDetails(det)
		// inject the internal flag to the state
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
