package source

import (
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/textileio/go-threads/core/thread"
)

const CName = "source"

func New() Service {
	return &service{}
}

type Service interface {
	NewSource(id string, listenToOwnChanges bool) (s Source, err error)
	RegisterStaticSource(id string, new func() Source)
	NewStaticSource(id string, sbType model.SmartBlockType, doc *state.State) SourceWithType
	SourceTypeBySbType(blockType smartblock.SmartBlockType) (SourceType, error)
	app.Component
}

type service struct {
	anytype       core.Service
	statusService status.Service

	staticIds map[string]func() Source
	mu        sync.Mutex
}

func (s *service) Init(a *app.App) (err error) {
	s.staticIds = make(map[string]func() Source)
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.statusService = a.MustComponent(status.CName).(status.Service)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) NewSource(id string, listenToOwnChanges bool) (source Source, err error) {
	st, err := smartblock.SmartBlockTypeFromID(id)

	if id == addr.AnytypeProfileId {
		return NewAnytypeProfile(s.anytype, id), nil
	}

	if st == smartblock.SmartBlockTypeFile {
		return NewFiles(s.anytype, id), nil
	}

	if st == smartblock.SmartBlockTypeBundledObjectType {
		return NewBundledObjectType(s.anytype, id), nil
	}

	if st == smartblock.SmartBlockTypeBundledRelation {
		return NewBundledRelation(s.anytype, id), nil
	}
	if st == smartblock.SmartBlockTypeIndexedRelation {
		return NewIndexedRelation(s.anytype, id), nil
	}

	tid, err := thread.Decode(id)
	if err != nil {
		err = fmt.Errorf("can't restore thread ID: %w", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if newStatic := s.staticIds[id]; newStatic != nil {
		return newStatic(), nil
	}
	return newSource(s.anytype, s.statusService, tid, listenToOwnChanges)
}

func (s *service) RegisterStaticSource(id string, new func() Source) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staticIds[id] = new
}
