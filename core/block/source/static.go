package source

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/pb"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func (s *service) NewStaticSource(id string, sbType model.SmartBlockType, doc *state.State) SourceWithType {
	return &static{
		id:     id,
		sbType: sbType,
		doc:    doc,
		a:      s.anytype,
		s:      s,
	}
}

type static struct {
	id     string
	sbType model.SmartBlockType
	doc    *state.State
	a      core.Service
	s      *service
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

func (s *static) ListIds() (result []string, err error) {
	s.s.mu.Lock()
	defer s.s.mu.Unlock()
	for id := range s.s.staticIds {
		if sbt, _ := smartblock.SmartBlockTypeFromID(id); sbt.ToProto() == s.Type() {
			result = append(result, id)
		}
	}
	return
}

func (s *static) Close() (err error) {
	return
}

func (v *static) LogHeads() map[string]string {
	return nil
}

func (s *static) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}
