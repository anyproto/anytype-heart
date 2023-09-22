package editor

import (
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
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
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
	systemObjectService system_object.Service,
	sourceService source.Service,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
	eventSender event.Sender,
) *SubObjectCollection {
	return &SubObjectCollection{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, objectStore, systemObjectService, layoutConverter),
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
			systemObjectService,
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
	for _, coll := range objectTypeToCollection {
		data := st.GetSubObjectCollection(coll)
		if data == nil {
			continue
		}

		for subId := range data.GetFields() {
			fullId := c.getId(coll, subId)

			_, err := c.subState(st, coll, fullId)
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

// subState returns a details-only state for a subobject
// make sure to call sbimpl.initState(st) before using it
func (c *SubObjectCollection) subState(st *state.State, collection string, fullId string) (*state.State, error) {
	subId := strings.TrimPrefix(fullId, collection+addr.SubObjectCollectionIdSeparator)
	data := pbtypes.GetStruct(st.GetSubObjectCollection(collection), subId)
	if data == nil || data.Fields == nil {
		return nil, fmt.Errorf("no data for subId %s: %v", collection, subId)
	}
	subst := structToState(fullId, data)

	relationsToCopy := []domain.RelationKey{bundle.RelationKeyCreator}
	for _, rk := range relationsToCopy {
		subst.SetDetailAndBundledRelation(rk, pbtypes.String(pbtypes.GetString(st.CombinedDetails(), rk.String())))
	}

	subst.SetLocalDetail(bundle.RelationKeyLinks.String(), pbtypes.StringList([]string{}))
	changeId := st.StoreChangeIdForPath(collection + addr.SubObjectCollectionIdSeparator + subId)
	if changeId == "" {
		log.Infof("subState %s: no changeId for %s", fullId, collection+addr.SubObjectCollectionIdSeparator+subId)
	}
	subst.SetLocalDetail(bundle.RelationKeyLastChangeId.String(), pbtypes.String(changeId))

	subst.AddBundledRelations(bundle.RelationKeyLastModifiedDate, bundle.RelationKeyLastOpenedDate, bundle.RelationKeyLastModifiedBy)
	subst.SetDetailAndBundledRelation(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyDescription.String(), bundle.RelationKeyType.String(), bundle.RelationKeySourceObject.String()}))
	subst.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, pbtypes.String(c.SpaceID()))

	return subst, nil
}

func structToState(id string, data *types.Struct) *state.State {
	blocks := map[string]simple.Block{
		id: simple.New(&model.Block{Id: id, ChildrenIds: []string{}}),
	}
	subState := state.NewDoc(id, blocks).(*state.State)

	for k, v := range data.Fields {
		if rel, err := bundle.GetRelation(domain.RelationKey(k)); err == nil {
			if rel.DataSource == model.Relation_details || slice.FindPos(localDetailsAllowedToBeStored, k) > -1 {
				subState.SetDetailAndBundledRelation(domain.RelationKey(k), v)
			}
		}
	}
	subState.SetDetailAndBundledRelation(bundle.RelationKeyId, pbtypes.String(id))
	switch pbtypes.GetInt64(data, bundle.RelationKeyLayout.String()) {
	case int64(model.ObjectType_relationOption):
		subState.SetObjectTypeKey(bundle.TypeKeyRelationOption)
	case int64(model.ObjectType_relation):
		subState.SetObjectTypeKey(bundle.TypeKeyRelation)
	case int64(model.ObjectType_objectType):
		subState.SetObjectTypeKey(bundle.TypeKeyObjectType)
	}

	return subState
}

func (p *SubObjectCollection) TryClose(objectTTL time.Duration) (res bool, err error) {
	// never close SubObjectCollection
	return false, nil
}

type SubObjectCollectionGetter interface {
	GetAllDocInfoIterator(func(smartblock.DocInfo) bool)
}
