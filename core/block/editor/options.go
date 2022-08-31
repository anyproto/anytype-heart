package editor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

var ErrOptionNotFound = errors.New("option not found")

func (w *Workspaces) Open(subId string) (sb smartblock.SmartBlock, err error) {
	w.Lock()
	defer w.Unlock()
	if opt, ok := w.options[subId]; ok {
		return opt, nil
	}
	return nil, ErrOptionNotFound
}

func (w *Workspaces) CreateRelationOption(opt *types.Struct) (id string, err error) {
	if opt == nil || opt.Fields == nil {
		return "", fmt.Errorf("create option: no data")
	}
	if pbtypes.GetString(opt, bundle.RelationKeyRelationKey.String()) == "" {
		return "", fmt.Errorf("field relationKey is empty or absent")
	}

	subId := bson.NewObjectId().Hex()
	id = w.Id() + "/" + subId
	opt.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)

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
	subState, err := w.optionSubState(st, subId)
	if err != nil {
		return
	}
	if err = opt.Init(&smartblock.InitContext{
		Source: w.sourceService.NewStaticSource(w.Id()+"/"+subId, model.SmartBlockType_RelationOption, subState, w.onOptionChange),
		App:    w.app,
	}); err != nil {
		return
	}
	w.options[subId] = opt
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
	return false
}

func (w *Workspaces) optionSubState(s *state.State, subId string) (*state.State, error) {
	id := w.Id() + "/" + subId
	optData := pbtypes.GetStruct(s.NewState().GetCollection(collectionKeyRelationOptions), subId)
	if optData == nil || optData.Fields == nil {
		return nil, fmt.Errorf("no data for option: %v", id)
	}
	subState := state.NewDoc(id, nil).(*state.State)
	for k, v := range optData.Fields {
		if _, err := bundle.GetRelation(bundle.RelationKey(k)); err == nil {
			subState.SetDetailAndBundledRelation(bundle.RelationKey(k), v)
		}
	}
	return subState, nil
}

func (w *Workspaces) updateOptions(info smartblock.ApplyInfo) (err error) {
	for _, ch := range info.Changes {
		if keySet := ch.GetStoreKeySet(); keySet != nil {
			if len(keySet.Path) >= 2 && keySet.Path[0] == collectionKeyRelationOptions {
				if opt, ok := w.options[keySet.Path[1]]; ok {
					if e := opt.SetStruct(pbtypes.GetStruct(w.NewState().GetCollection(collectionKeyRelationOptions), keySet.Path[1])); e != nil {
						log.With("threadId", w.Id()).Errorf("options: can't set struct: %v", e)
					}
				}
			}
		}
	}
	return
}

func (w *Workspaces) onOptionChange(params source.PushChangeParams) (changeId string, err error) {
	w.Lock()
	defer w.Unlock()
	st := w.NewState()
	id := params.State.RootId()
	var subId string
	if idx := strings.Index(id, "/"); idx != -1 {
		subId = id[idx+1:]
	}
	if _, ok := w.options[subId]; !ok {
		return "", fmt.Errorf("onOptionChange: option not exists")
	}
	st.SetInStore([]string{collectionKeyRelationOptions, subId}, pbtypes.Struct(params.State.CombinedDetails()))
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
