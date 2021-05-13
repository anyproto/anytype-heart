package source

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewStaticSource(a core.Service, id string, sbType model.SmartBlockType, doc *state.State) Source {
	return &static{
		id:     id,
		sbType: sbType,
		doc:    doc,
		a:      a,
	}
}

type static struct {
	id     string
	sbType model.SmartBlockType
	doc    *state.State
	a      core.Service
}

func (s *static) Id() string {
	return s.id
}

func (s *static) Anytype() core.Service {
	return s.a
}

func (s *static) Type() model.SmartBlockType {
	return s.sbType
}

func (s *static) Virtual() bool {
	return true
}

func (s *static) ReadOnly() bool {
	return true
}

func (s *static) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	return s.doc, nil
}

func (s *static) ReadMeta(receiver ChangeReceiver) (doc state.Doc, err error) {
	return s.doc, nil
}

func (s *static) PushChange(params PushChangeParams) (id string, err error) {
	return
}

func (s *static) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (s *static) Close() (err error) {
	return
}
