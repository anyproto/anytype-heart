package editor

import (
	"errors"
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

var ErrSubObjectNotFound = errors.New("sub object not found")

func (w *Workspaces) Open(subId string) (sb smartblock.SmartBlock, err error) {
	w.Lock()
	defer w.Unlock()
	if strings.HasPrefix(subId, addr.RelationKeyToIdPrefix) {
		subId = strings.TrimPrefix(subId, addr.RelationKeyToIdPrefix)
		if opt, ok := w.relations[subId]; ok {
			return opt, nil
		}
		return nil, ErrSubObjectNotFound
	}
	if opt, ok := w.options[subId]; ok {
		return opt, nil
	}

	return nil, ErrSubObjectNotFound
}

func (w *Workspaces) CreateRelationOption(opt *types.Struct) (subId string, err error) {
	if opt == nil || opt.Fields == nil {
		return "", fmt.Errorf("create option: no data")
	}
	if pbtypes.GetString(opt, bundle.RelationKeyRelationKey.String()) == "" {
		return "", fmt.Errorf("field relationKey is empty or absent")
	}

	subId = bson.NewObjectId().Hex()
	opt.Fields[bundle.RelationKeyId.String()] = pbtypes.String(subId)

	st := w.NewState()
	st.SetInStore([]string{collectionKeyRelationOptions, subId}, pbtypes.Struct(opt))
	if err = w.initOption(st, subId); err != nil {
		return
	}
	if err = w.Apply(st, smartblock.NoHooks); err != nil {
		return
	}
	return
}

func (w *Workspaces) initOption(st *state.State, subId string) (err error) {
	opt := NewOption()
	subState, err := w.subState(st, collectionKeyRelationOptions, subId)
	if err != nil {
		return
	}
	w.options[subId] = opt
	if err = opt.Init(&smartblock.InitContext{
		Source: w.sourceService.NewStaticSource(subId, model.SmartBlockType_SubObjectRelationOption, subState, w.onOptionChange),
		App:    w.app,
	}); err != nil {
		return
	}
	return
}

func (w *Workspaces) Locked() bool {
	w.Lock()
	defer w.Unlock()
	if w.IsLocked() {
		return true
	}
	for _, opt := range w.options {
		if opt.Locked() {
			return true
		}
	}

	for _, rel := range w.relations {
		if rel.Locked() {
			return true
		}
	}
	return false
}

func (w *Workspaces) updateSubObject(info smartblock.ApplyInfo) (err error) {
	for _, ch := range info.Changes {
		if keySet := ch.GetStoreKeySet(); keySet != nil {
			if len(keySet.Path) >= 2 {
				switch keySet.Path[0] {
				case collectionKeyRelationOptions:
					if opt, ok := w.options[keySet.Path[1]]; ok {
						if e := opt.SetStruct(pbtypes.GetStruct(w.NewState().GetCollection(collectionKeyRelationOptions), keySet.Path[1])); e != nil {
							log.With("threadId", w.Id()).Errorf("options: can't set struct: %v", e)
						}
					}
				case collectionKeyRelations:
					if opt, ok := w.relations[keySet.Path[1]]; ok {
						if e := opt.SetStruct(pbtypes.GetStruct(w.NewState().GetCollection(collectionKeyRelations), keySet.Path[1])); e != nil {
							log.With("threadId", w.Id()).Errorf("relations: can't set struct: %v", e)
						}
					}
				}
			}
		}
	}
	return
}

func (w *Workspaces) onOptionChange(params source.PushChangeParams) (changeId string, err error) {
	st := w.NewState()
	id := params.State.RootId()
	if _, ok := w.options[id]; !ok {
		return "", fmt.Errorf("onOptionChange: option not exists")
	}
	st.SetInStore([]string{collectionKeyRelationOptions, id}, pbtypes.Struct(params.State.CombinedDetails()))
	return "", w.Apply(st, smartblock.NoHooks)
}

func NewOption() *Option {
	return &Option{
		SmartBlock: smartblock.New(),
	}
}

type Option struct {
	smartblock.SmartBlock
}

func (o *Option) Init(ctx *smartblock.InitContext) (err error) {
	if err = o.SmartBlock.Init(ctx); err != nil {
		return
	}
	return smartblock.ObjectApplyTemplate(o, ctx.State,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyRelationOption.URL()}),
	)
}

func (o *Option) SetStruct(st *types.Struct) error {
	o.Lock()
	defer o.Unlock()
	s := o.NewState()
	s.SetDetails(st)
	return o.Apply(s)
}
