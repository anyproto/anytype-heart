package editor

import (
	"errors"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"strings"
)

var (
	ErrSubObjectNotFound      = errors.New("sub object not found")
	ErrCollectionNotFound     = errors.New("collection not found")
	ErrSubObjectAlreadyExists = fmt.Errorf("subobject already exists in the collection")
)

const defaultCollectionName = "opt"

// todo: extract collection of subobjects into a separate smartblock impl

func (w *Workspaces) Open(subId string) (sb smartblock.SmartBlock, err error) {
	w.Lock()
	defer w.Unlock()
	parts := strings.Split(subId, addr.VirtualObjectSeparator)
	var collection, key string
	if len(parts) == 1 {
		collection = defaultCollectionName
		key = parts[0]
	} else {
		collection = parts[0]
		key = parts[1]
	}
	if coll, exists := w.collections[collection]; exists {
		if sub, exists := coll[key]; exists {
			return sub, nil
		} else {
			return nil, ErrSubObjectNotFound
		}
	}

	return nil, ErrCollectionNotFound
}

func (w *Workspaces) Locked() bool {
	w.Lock()
	defer w.Unlock()
	if w.IsLocked() {
		return true
	}
	for _, coll := range w.collections {
		for _, sub := range coll {
			if sub.IsLocked() {
				return true
			}
		}
	}
	return false
}

func (w *Workspaces) updateSubObject(info smartblock.ApplyInfo) (err error) {
	for _, ch := range info.Changes {
		if keySet := ch.GetStoreKeySet(); keySet != nil {
			if len(keySet.Path) >= 2 {
				if coll, ok := w.collections[keySet.Path[0]]; ok {
					if opt, ok := coll[keySet.Path[1]]; ok {
						if e := opt.SetStruct(pbtypes.GetStruct(w.NewState().GetCollection(collectionKeyRelationOptions), keySet.Path[1])); e != nil {
							log.With("threadId", w.Id()).Errorf("options: can't set struct: %v", e)
						}
					}
				}
			}
		}
	}
	return
}

func (w *Workspaces) onSubObjectChange(collection string) func(p source.PushChangeParams) (string, error) {
	return func(p source.PushChangeParams) (string, error) {
		st := w.NewState()
		id := p.State.RootId()
		coll, exists := w.collections[collection]
		if !exists {
			return "", fmt.Errorf("collection not found")
		}

		if _, ok := coll[id]; !ok {
			return "", fmt.Errorf("onSubObjectChange: subObject not exists")
		}
		changed := st.SetInStore([]string{collection, id}, pbtypes.Struct(p.State.CombinedDetails()))
		if !changed {
			return "", nil
		}
		return "", w.Apply(st, smartblock.NoHooks)
	}
}

func NewSubObject() *SubObject {
	return &SubObject{
		SmartBlock: smartblock.New(),
	}
}

type SubObject struct {
	smartblock.SmartBlock
}

func (o *SubObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = o.SmartBlock.Init(ctx); err != nil {
		return
	}
	return nil
}

func (o *SubObject) SetStruct(st *types.Struct) error {
	o.Lock()
	defer o.Unlock()
	s := o.NewState()
	s.SetDetails(st)
	return o.Apply(s)
}

func (w *Workspaces) initSubObject(st *state.State, collection string, subId string) (err error) {
	subObj := NewSubObject()
	var fullId string
	if collection == "" {
		fullId = subId
	} else {
		fullId = collection + addr.VirtualObjectSeparator + subId
	}
	subState, err := smartblock.SubState(st, collection, fullId)
	if err != nil {
		return
	}
	if _, exists := w.collections[collection]; !exists {
		w.collections[collection] = map[string]*SubObject{}
	}
	w.collections[collection][subId] = subObj
	if err = subObj.Init(&smartblock.InitContext{
		Source: w.sourceService.NewStaticSource(fullId, model.SmartBlockType_SubObject, subState, w.onSubObjectChange(collection)),
		App:    w.app,
	}); err != nil {
		return
	}
	return
}
