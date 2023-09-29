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
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
		tk, ok := collectionKeyToObjectType(coll)
		if !ok {
			log.With("collection", coll).Errorf("subobject migration: collection is invalid")
			continue
		}

		for subId, d := range data.GetFields() {
			if st, ok := d.Kind.(*types.Value_StructValue); !ok {
				log.Errorf("got invalid value for %s.%s:%t", coll, subId, d.Kind)
				continue
			} else {
				uk, err := c.getUniqueKey(coll, subId)
				if err != nil {
					log.With("collection", coll).Errorf("subobject migration: failed to get uniqueKey: %s", err.Error())
					continue
				}

				d := st.StructValue
				d.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uk.Marshal())

				if !f(smartblock.DocInfo{
					SpaceID:    c.SpaceID(),
					Links:      nil,
					FileHashes: nil,
					Heads:      nil,
					Type:       tk,
					Details:    d,
				}) {
					return
				}
			}
		}
	}
	return
}

func (c *SubObjectCollection) getUniqueKey(collection, key string) (domain.UniqueKey, error) {
	ot, ok := collectionKeyToObjectType(collection)
	if !ok {
		return nil, fmt.Errorf("unknown collection %s", collection)
	}

	var sbt smartblock2.SmartBlockType
	switch ot {
	case bundle.TypeKeyRelation:
		sbt = smartblock2.SmartBlockTypeRelation
	case bundle.TypeKeyObjectType:
		sbt = smartblock2.SmartBlockTypeObjectType
	case bundle.TypeKeyRelationOption:
		sbt = smartblock2.SmartBlockTypeRelationOption
	default:
		return nil, fmt.Errorf("unknown collection %s", collection)
	}

	return domain.NewUniqueKey(sbt, key)
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

func (p *SubObjectCollection) TryClose(objectTTL time.Duration) (res bool, err error) {
	// never close SubObjectCollection
	return false, nil
}

type SubObjectCollectionGetter interface {
	GetAllDocInfoIterator(func(smartblock.DocInfo) bool)
}
