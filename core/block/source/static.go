package source

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *service) NewStaticSource(id string, sbType model.SmartBlockType, doc *state.State, pushChange func(p PushChangeParams) (string, error)) SourceWithType {
	return &static{
		id:         id,
		sbType:     sbType,
		doc:        doc,
		s:          s,
		pushChange: pushChange,
	}
}

type static struct {
	id         string
	sbType     model.SmartBlockType
	doc        *state.State
	pushChange func(p PushChangeParams) (string, error)
	s          *service
}

func (s *static) Id() string {
	return s.id
}

func (s *static) Type() model.SmartBlockType {
	return s.sbType
}

func (s *static) ReadOnly() bool {
	return s.pushChange == nil
}

func (s *static) ReadDoc(ctx session.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	return s.doc, nil
}

func (s *static) ReadMeta(ctx context.Context, receiver ChangeReceiver) (doc state.Doc, err error) {
	return s.doc, nil
}

func (s *static) PushChange(params PushChangeParams) (id string, err error) {
	if s.pushChange != nil {
		return s.pushChange(params)
	}
	return
}

func (s *static) ListIds() (result []string, err error) {
	s.s.mu.Lock()
	defer s.s.mu.Unlock()
	for id, src := range s.s.staticIds {
		if src.Type() == s.Type() {
			result = append(result, id)
		}
	}
	return
}

func (s *static) Close() (err error) {
	return
}

func (v *static) Heads() []string {
	return []string{"todo"} // todo hash of the details
}

func (s *static) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}
