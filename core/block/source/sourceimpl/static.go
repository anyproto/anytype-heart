package sourceimpl

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func (s *service) NewStaticSource(params source.StaticSourceParams) source.SourceWithType {
	return &static{
		id:        params.Id,
		sbType:    params.SbType,
		doc:       params.State,
		s:         s,
		creatorId: params.CreatorId,
	}
}

type static struct {
	id        domain.FullID
	sbType    smartblock.SmartBlockType
	doc       *state.State
	creatorId string
	s         *service
}

func (s *static) Id() string {
	return s.id.ObjectID
}

func (s *static) SpaceID() string {
	return s.id.SpaceID
}

func (s *static) Type() smartblock.SmartBlockType {
	return s.sbType
}

func (s *static) ReadOnly() bool {
	return true
}

func (s *static) ReadDoc(ctx context.Context, receiver source.ChangeReceiver, empty bool) (doc state.Doc, err error) {
	return s.doc, nil
}

func (s *static) PushChange(params source.PushChangeParams) (id string, err error) {
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

func (s *static) Heads() []string {
	return []string{"todo"} // todo hash of the details
}

func (s *static) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *static) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return s.creatorId, 0, nil
}
