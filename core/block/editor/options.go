package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/subobject"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

const optionsCollName = "options"

type SubObjectCreator interface {
	NewSubObject(subId string, parent subobject.ParentObject) (s *subobject.SubObject, err error)
}

func NewOptions(sc SubObjectCreator) *Options {
	return &Options{
		SmartBlock: smartblock.New(),
		sc:         sc,
	}
}

type Options struct {
	smartblock.SmartBlock
	sc          SubObjectCreator
	openedCount int
}

func (o *Options) Open(id string) (sb smartblock.SmartBlock, err error) {
	so, err := o.sc.NewSubObject(id, o)
	if err != nil {
		return nil, err
	}
	opt := &Option{
		parent:    o,
		SubObject: so,
	}
	return opt, nil
}

func (o *Options) CreateOption(opt *types.Struct) (id string, err error) {
	o.Lock()
	subId := bson.NewObjectId().Hex()
	id = o.Id() + "/" + subId
	st := o.NewState()
	st.SetInStore([]string{optionsCollName, subId}, pbtypes.Struct(opt))
	ids := pbtypes.GetStringList(st.Details(), bundle.RelationKeyRelationDict.String())
	ids = append(ids, id)
	st.SetDetail(bundle.RelationKeyRelationDict.String(), pbtypes.StringList(ids))
	if err = o.Apply(st); err != nil {
		o.Unlock()
		return
	}
	o.Unlock()

}

func (o *Options) Locked() bool {
	return o.SmartBlock.Locked() || o.openedCount > 0
}

func (o *Options) SubState(subId string) (*state.State, error) {
	o.Lock()
	defer o.Unlock()
	id := o.Id() + "/" + subId
	s := o.NewState()
	ids := pbtypes.GetStringList(s.Details(), bundle.RelationKeyRelationDict.String())
	if slice.FindPos(ids, id) == -1 {
		return nil, bundle.ErrNotFound
	}
	optData := pbtypes.GetStruct(s.NewState().GetCollection(optionsCollName), subId)
	o.openedCount++
	return state.NewDoc(id, nil).(*state.State).SetDetails(optData), nil
}

func (o *Options) SubStateApply(subId string, s *state.State) (err error) {
	o.Lock()
	defer o.Unlock()
	st := o.NewState()
	st.SetInStore([]string{optionsCollName, subId}, pbtypes.Struct(s.CombinedDetails()))
	return o.Apply(s)
}

type Option struct {
	parent *Options
	*subobject.SubObject
}

func (o *Option) Close() (err error) {
	o.parent.Lock()
	o.parent.openedCount--
	o.parent.Unlock()
	return o.SubObject.Close()
}
