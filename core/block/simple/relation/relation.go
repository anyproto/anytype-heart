package relation

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

// Validate TODO: add validation rules
func (l *Relation) Validate() error {
	return nil
}

func (l *Relation) Diff(spaceId string, b simple.Block) (msgs []simple.EventMessage, err error) {
	relation, ok := b.(*Relation)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = l.Base.Diff(spaceId, relation); err != nil {
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
		msgs = append(msgs, simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockSetRelation{BlockSetRelation: changes})})
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
