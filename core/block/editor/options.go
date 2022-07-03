package editor

import (
	"errors"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"strings"
)

var ErrOptionNotFound = errors.New("option not found")

const optionsCollName = "options"

func NewOptions(s source.Service) *Options {
	return &Options{
		SmartBlock:    smartblock.New(),
		options:       make(map[string]*Option),
		sourceService: s,
	}
}

type Options struct {
	smartblock.SmartBlock
	options       map[string]*Option
	sourceService source.Service
	app           *app.App
}

func (o *Options) Init(ctx *smartblock.InitContext) (err error) {
	o.AddHook(o.onAfterApply, smartblock.HookAfterApply)
	o.app = ctx.App
	if err = o.SmartBlock.Init(ctx); err != nil {
		return
	}
	data := ctx.State.GetCollection(optionsCollName)
	if data != nil && data.Fields != nil {
		for subId := range data.Fields {
			if err = o.initOption(subId); err != nil {
				return
			}
		}
	}
	if err = template.InitTemplate(ctx.State,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyRelationOptionList.URL()}),
	); err != nil {
		return
	}
	return o.Apply(ctx.State, smartblock.NoHooks)
}

func (o *Options) Open(subId string) (sb smartblock.SmartBlock, err error) {
	o.Lock()
	defer o.Unlock()
	if opt, ok := o.options[subId]; ok {
		return opt, nil
	}
	return nil, ErrOptionNotFound
}

func (o *Options) CreateOption(opt *types.Struct) (id string, err error) {
	o.Lock()
	defer o.Unlock()
	subId := bson.NewObjectId().Hex()
	id = o.Id() + "/" + subId
	st := o.NewState()
	st.SetObjectType(bundle.TypeKeyRelationOption.URL())
	st.SetInStore([]string{optionsCollName, subId}, pbtypes.Struct(opt))
	ids := pbtypes.GetStringList(st.Details(), bundle.RelationKeyRelationOptionsDict.String())
	ids = append(ids, id)
	st.SetDetail(bundle.RelationKeyRelationOptionsDict.String(), pbtypes.StringList(ids))
	if err = o.initOption(subId); err != nil {
		return
	}
	if err = o.Apply(st, smartblock.NoHooks); err != nil {
		return
	}
	return
}

func (o *Options) initOption(subId string) (err error) {
	opt := NewOption()
	st, err := o.subState(subId)
	if err != nil {
		return
	}
	if err = opt.Init(&smartblock.InitContext{
		Source: o.sourceService.NewStaticSource(o.Id()+"/"+subId, model.SmartBlockType_RelationOption, st, o.onOptionChange),
		App:    o.app,
	}); err != nil {
		return
	}
	o.options[subId] = opt
	return
}

func (o *Options) Locked() bool {
	o.Lock()
	defer o.Unlock()
	if o.IsLocked() {
		return true
	}
	for _, opt := range o.options {
		if opt.Locked() {
			return true
		}
	}
	return false
}

func (o *Options) subState(subId string) (*state.State, error) {
	id := o.Id() + "/" + subId
	s := o.NewState()
	ids := pbtypes.GetStringList(s.Details(), bundle.RelationKeyRelationOptionsDict.String())
	if slice.FindPos(ids, id) == -1 {
		return nil, ErrOptionNotFound
	}
	optData := pbtypes.GetStruct(s.NewState().GetCollection(optionsCollName), subId)
	return state.NewDoc(id, nil).(*state.State).SetDetails(optData), nil
}

func (o *Options) onOptionChange(params source.PushChangeParams) (changeId string, err error) {
	o.Lock()
	defer o.Unlock()
	st := o.NewState()
	id := params.State.RootId()
	var subId string
	if idx := strings.Index(id, "/"); idx != -1 {
		subId = id[idx+1:]
	}
	if _, ok := o.options[subId]; !ok {
		return "", fmt.Errorf("onOptionChange: option not exists")
	}
	st.SetInStore([]string{optionsCollName, subId}, pbtypes.Struct(params.State.CombinedDetails()))
	return "", o.Apply(st, smartblock.NoHooks)
}

func (o *Options) onAfterApply(info smartblock.ApplyInfo) (err error) {
	for _, ch := range info.Changes {
		if keySet := ch.GetStoreKeySet(); keySet != nil {
			if len(keySet.Path) >= 2 && keySet.Path[0] == optionsCollName {
				if opt, ok := o.options[keySet.Path[1]]; ok {
					if e := opt.SetStruct(pbtypes.GetStruct(o.NewState().GetCollection(optionsCollName), keySet.Path[1])); e != nil {
						log.With("threadId", o.Id()).Errorf("options: can't set struct: %v", e)
					}
				}
			}
		}
	}
	return
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
