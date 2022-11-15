package editor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var (
	ErrSubObjectNotFound      = errors.New("sub object not found")
	ErrCollectionNotFound     = errors.New("collection not found")
	ErrSubObjectAlreadyExists = fmt.Errorf("subobject already exists in the collection")
)

type SubObjectCollection struct {
	*Set
	defaultCollectionName string

	collections map[string]map[string]*SubObject

	sourceService source.Service
	app           *app.App
}

func NewSubObjectCollection(defaultCollectionName string) *SubObjectCollection {
	return &SubObjectCollection{
		Set:                   NewSet(),
		defaultCollectionName: defaultCollectionName,
		collections:           map[string]map[string]*SubObject{},
	}
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
		changed := st.SetInStore([]string{collection, subId}, pbtypes.Struct(p.State.CombinedDetails()))
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
	subObj := NewSubObject()
	var fullId string
	if collection == "" || collection == c.defaultCollectionName {
		fullId = subId
		collection = c.defaultCollectionName
	} else {
		fullId = collection + addr.SubObjectCollectionIdSeparator + subId
	}
	subState, err := smartblock.SubState(st, collection, fullId)
	if err != nil {
		return
	}
	template.WithForcedDetail(bundle.RelationKeyWorkspaceId, pbtypes.String(c.Id()))(subState)

	if _, exists := c.collections[collection]; !exists {
		c.collections[collection] = map[string]*SubObject{}
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
