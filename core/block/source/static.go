package source

import (
	"context"
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type StaticSourceParams struct {
	Id          domain.FullID
	SbType      smartblock.SmartBlockType
	Doc         *state.State
	CreatorId   string
	StayInCache bool
	PushChange  func(p PushChangeParams) (string, error)
}

func (s *service) NewStaticSource(params StaticSourceParams) SourceWithType {
	return &static{
		id:          params.Id,
		sbType:      params.SbType,
		doc:         params.Doc,
		s:           s,
		creatorId:   params.CreatorId,
		pushChange:  params.PushChange,
		stayInCache: params.StayInCache,
	}
}

type static struct {
	id          domain.FullID
	sbType      smartblock.SmartBlockType
	doc         *state.State
	creatorId   string
	pushChange  func(p PushChangeParams) (string, error)
	s           *service
	stayInCache bool
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
	return s.pushChange == nil
}

func (s *static) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
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

func (s *static) TryClose(objectTTL time.Duration) (res bool, err error) {
	return !s.stayInCache, nil
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
