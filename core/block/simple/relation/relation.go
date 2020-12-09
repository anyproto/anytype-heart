package relation

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewRelation)
}

func NewRelation(m *model.Block) simple.Block {
	if relation := m.GetRelation(); relation != nil {
		return &Relation{
			Base:    base.NewBase(m).(*base.Base),
			content: relation,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	SetKey(key string)
	ApplyEvent(e *pb.EventBlockSetRelation) error
}

type Relation struct {
	*base.Base
	content *model.BlockContentRelation
}

func (l *Relation) Copy() simple.Block {
	copy := pbtypes.CopyBlock(l.Model())
	return &Relation{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetRelation(),
	}
}

func (l *Relation) Diff(b simple.Block) (msgs []simple.EventMessage, err error) {
	relation, ok := b.(*Relation)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = l.Base.Diff(relation); err != nil {
		return
	}
	changes := &pb.EventBlockSetRelation{
		Id: relation.Id,
	}
	hasChanges := false

	if l.content.Key != relation.content.Key {
		hasChanges = true
		changes.Key = &pb.EventBlockSetRelationKey{Value: relation.content.Key}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetRelation{BlockSetRelation: changes}}})
	}
	return
}

func (r *Relation) SetKey(key string) {
	r.content.Key = key
}

func (l *Relation) ApplyEvent(e *pb.EventBlockSetRelation) error {
	if e.Key != nil {
		l.content.Key = e.Key.GetValue()
	}
	return nil
}
