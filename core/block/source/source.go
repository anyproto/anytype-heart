package source

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type Version struct {
	Meta   *core.SmartBlockMeta
	Blocks []*model.Block
}

type Source interface {
	Id() string
	Anytype() anytype.Service
	Meta() meta.Service
	Type() pb.SmartBlockType
	ReadDoc() (doc state.Doc, err error)
	PushChange(c *pb.Change) (err error)
	Close() (err error)
}

func NewSource(a anytype.Service, m meta.Service, id string) (s Source, err error) {
	sb, err := a.GetBlock(id)
	if err != nil {
		err = fmt.Errorf("anytype.GetBlock(%v) error: %v", id, err)
		return
	}
	s = &source{
		id:   id,
		a:    a,
		sb:   sb,
		meta: m,
	}
	return
}

type source struct {
	id   string
	a    anytype.Service
	sb   core.SmartBlock
	meta meta.Service
	tree *change.Tree
}

func (s *source) Id() string {
	return s.id
}

func (s *source) Anytype() anytype.Service {
	return s.a
}

func (s *source) Meta() meta.Service {
	return s.meta
}

func (s *source) Type() pb.SmartBlockType {
	return anytype.SmartBlockTypeToProto(s.sb.Type())
}

func (s *source) ReadDoc() (doc state.Doc, err error) {
	doc = state.NewDoc(s.id, nil)
	s.tree, err = change.BuildTree(s.sb)
	if err == change.ErrEmpty {
		return doc, nil
	} else if err != nil {
		return nil, err
	}
	st, err := change.BuildState(doc.(*state.State), s.tree)
	if err != nil {
		return
	}

	return
}

func (s *source) PushChange(c *pb.Change) (err error) {

	return
}

func (s *source) Close() (err error) {
	return nil
}
