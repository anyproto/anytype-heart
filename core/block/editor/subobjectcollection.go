package editor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var (
	ErrSubObjectNotFound      = errors.New("sub object not found")
	ErrCollectionNotFound     = errors.New("collection not found")
	ErrSubObjectAlreadyExists = fmt.Errorf("subobject already exists in the collection")
)

type SubObjectImpl interface {
	smartblock.SmartBlock
	SetStruct(*types.Struct) error
	InitState(st *state.State) // InitState normalize state and fill it with simple blocks
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

	sourceService source.Service
	objectStore   objectstore.ObjectStore
}

func NewSubObjectCollection(
	sb smartblock.SmartBlock,
	defaultCollectionName string,
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	relationService relation.Service,
	sourceService source.Service,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
	eventSender event.Sender,
) *SubObjectCollection {
	return &SubObjectCollection{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, objectStore, relationService, layoutConverter),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			objectStore,
			eventSender,
		),
		Dataview: dataview.NewDataview(
			sb,
			anytype,
			objectStore,
			relationService,
			sbtProvider,
		),

		objectStore:           objectStore,
		sourceService:         sourceService,
		defaultCollectionName: defaultCollectionName,
		collections:           map[string]map[string]SubObjectImpl{},
	}
}

func (c *SubObjectCollection) Init(ctx *smartblock.InitContext) error {
	return c.SmartBlock.Init(ctx)
}

// GetAllDocInfoIterator returns all sub objects in the collection
func (c *SubObjectCollection) GetAllDocInfoIterator(f func(smartblock.DocInfo) (contin bool)) {
	st := c.NewState()
	workspaceID := pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeyWorkspaceId.String())

	for _, coll := range objectTypeToCollection {
		data := st.GetSubObjectCollection(coll)
		if data == nil {
			continue
		}

		for subId := range data.GetFields() {
			fullId := c.getId(coll, subId)

			_, err := c.subState(st, coll, fullId, workspaceID)
			if err != nil {
				log.Errorf("failed to get sub object %s: %v", subId, err)
				continue
			}
			// todo: migrate
		}
	}
	return
}

func (c *SubObjectCollection) getId(collection, key string) string {
	if collection == c.defaultCollectionName {
		return key
	}
	return collection + addr.SubObjectCollectionIdSeparator + key
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
	err := c.removeObject(st, objectId)
	if err != nil {
		return err
	}
	return c.Apply(st, smartblock.NoEvent, smartblock.NoHistory, smartblock.NoHooks)
}

func (c *SubObjectCollection) removeObject(st *state.State, objectId string) (err error) {
	collection, key := c.getCollectionAndKeyFromId(objectId)
	// todo: check inbound links
	links, err := c.objectStore.GetInboundLinksByID(objectId)
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

func (c *SubObjectCollection) updateSubObject(ctx context.Context) func(info smartblock.ApplyInfo) (err error) {
	return func(info smartblock.ApplyInfo) (err error) {
		if len(info.Changes) == 0 {
			return nil
		}
		st := c.NewState()
		for _, ch := range info.Changes {
			if keySet := ch.GetStoreKeySet(); keySet != nil {
				if len(keySet.Path) < 2 {
					continue
				}
				var (
					collName = keySet.Path[0]
					subId    = keySet.Path[1]
				)
				coll, exists := c.collections[collName]
				if !exists {
					coll = map[string]SubObjectImpl{}
					c.collections[collName] = coll
				}
				if opt, ok := coll[subId]; ok {
					if e := opt.SetStruct(pbtypes.GetStruct(c.NewState().GetSubObjectCollection(collName), subId)); e != nil {
						log.With("treeId", c.Id()).
							Errorf("options: can't set struct %s-%s: %v", collName, subId, e)
					}
				} else {
					if err = c.initSubObject(ctx, st, collName, subId, false); err != nil {
						return
					}
				}
			} else if keyUnset := ch.GetStoreKeyUnset(); keyUnset != nil {
				err = c.removeObject(st, strings.Join(keyUnset.Path, addr.SubObjectCollectionIdSeparator))
				if err != nil {
					log.With("objectID", c.Id()).Errorf("failed to remove object %s: %v", strings.Join(keyUnset.Path, addr.SubObjectCollectionIdSeparator), err)
					return err
				}
			}
		}
		return
	}
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
		prevSubState := pbtypes.GetStruct(st.GetSubObjectCollection(collection), subId)

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
		err := c.Apply(st, smartblock.NoHooks)
		if err != nil {
			return "", err
		}

		return c.SmartBlock.(state.Doc).ChangeId(), nil
	}
}

func (c *SubObjectCollection) initSubObject(ctx context.Context, st *state.State, collection string, subId string, justCreated bool) (err error) {
	if len(strings.Split(subId, addr.SubObjectCollectionIdSeparator)) > 1 {
		// handle invalid cases for our own accounts
		return fmt.Errorf("invalid id: %s", subId)
	}

	var fullId string
	if collection == "" || collection == c.defaultCollectionName {
		fullId = subId
		collection = c.defaultCollectionName
	} else {
		fullId = collection + addr.SubObjectCollectionIdSeparator + subId
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

	workspaceID := pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeyWorkspaceId.String())
	if workspaceID == "" {
		// SubObjectCollection is used only workspaces now so get ID from the workspace object
		workspaceID = st.RootId()
	}
	subState, err := c.subState(st, collection, fullId, workspaceID)
	if err != nil {
		return
	}
	if justCreated {
		det := subState.CombinedDetails()
		internalflag.PutToDetails(det, []*model.InternalFlag{{Value: model.InternalFlag_editorDeleteEmpty}})
		subState.SetDetails(det)
		// inject the internal flag to the state
	}
	return
}

// subState returns a details-only state for a subobject
// make sure to call sbimpl.initState(st) before using it
func (c *SubObjectCollection) subState(st *state.State, collection string, fullId string, workspaceId string) (*state.State, error) {
	subId := strings.TrimPrefix(fullId, collection+addr.SubObjectCollectionIdSeparator)
	data := pbtypes.GetStruct(st.GetSubObjectCollection(collection), subId)
	if data == nil || data.Fields == nil {
		return nil, fmt.Errorf("no data for subId %s: %v", collection, subId)
	}
	subst := structToState(fullId, data)

	relationsToCopy := []bundle.RelationKey{bundle.RelationKeyCreator}
	for _, rk := range relationsToCopy {
		subst.SetDetailAndBundledRelation(rk, pbtypes.String(pbtypes.GetString(st.CombinedDetails(), rk.String())))
	}

	restrictions := restriction.GetRestrictionsForSubobject(fullId)
	subst.SetLocalDetail(bundle.RelationKeyRestrictions.String(), restrictions.ToPB())
	subst.SetLocalDetail(bundle.RelationKeyLinks.String(), pbtypes.StringList([]string{}))
	changeId := st.StoreChangeIdForPath(collection + addr.SubObjectCollectionIdSeparator + subId)
	if changeId == "" {
		log.Infof("subState %s: no changeId for %s", fullId, collection+addr.SubObjectCollectionIdSeparator+subId)
	}
	subst.SetLocalDetail(bundle.RelationKeyLastChangeId.String(), pbtypes.String(changeId))

	subst.AddBundledRelations(bundle.RelationKeyLastModifiedDate, bundle.RelationKeyLastOpenedDate, bundle.RelationKeyLastModifiedBy)
	subst.SetDetailAndBundledRelation(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyDescription.String(), bundle.RelationKeyType.String(), bundle.RelationKeySourceObject.String()}))
	subst.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(workspaceId))
	subst.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, pbtypes.String(c.SpaceID()))

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

func (p *SubObjectCollection) TryClose(objectTTL time.Duration) (res bool, err error) {
	// never close SubObjectCollection
	return false, nil
}

type SubObjectCollectionGetter interface {
	GetAllDocInfoIterator(func(smartblock.DocInfo) bool)
}
