package source

import (
	"fmt"
	"sync"

	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
)

const CName = "source"

func New() Service {
	return &service{}
}

type Service interface {
	NewSource(id string, ot objecttree.ObjectTree) (s Source, err error)
	RegisterStaticSource(id string, new func() Source)
	NewStaticSource(id string, sbType model.SmartBlockType, doc *state.State, pushChange func(p PushChangeParams) (string, error)) SourceWithType
	RemoveStaticSource(id string)

	GetDetailsFromIdBasedSource(id string) (*types.Struct, error)
	SourceTypeBySbType(blockType smartblock.SmartBlockType) (SourceType, error)
	app.Component
}

type service struct {
	anytype       core.Service
	statusService status.Service
	typeProvider  typeprovider.ObjectTypeProvider
	account       accountservice.Service
	fileStore     filestore.FileStore

	staticIds map[string]func() Source
	mu        sync.Mutex
}

func (s *service) Init(a *app.App) (err error) {
	s.staticIds = make(map[string]func() Source)
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.statusService = a.MustComponent(status.CName).(status.Service)
	s.typeProvider = a.MustComponent(typeprovider.CName).(typeprovider.ObjectTypeProvider)
	s.account = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) NewSource(id string, ot objecttree.ObjectTree) (source Source, err error) {
	if id == addr.AnytypeProfileId {
		return NewAnytypeProfile(s.anytype, id), nil
	}
	st, err := smartblock.SmartBlockTypeFromID(id)
	switch st {
	case smartblock.SmartBlockTypeFile:
		return NewFiles(s.anytype, s.fileStore, id), nil
	case smartblock.SmartBlockTypeDate:
		return NewDate(s.anytype, id), nil
	case smartblock.SmartBlockTypeBundledObjectType:
		return NewBundledObjectType(s.anytype, id), nil
	case smartblock.SmartBlockTypeBundledRelation:
		return NewBundledRelation(s.anytype, id), nil
	case smartblock.SmartBlockTypeBreadcrumbs:
		return NewVirtual(s.anytype, st.ToProto()), nil
	case smartblock.SmartBlockTypeWorkspaceOld:
		return nil, fmt.Errorf("threadDB-based workspaces are deprecated")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if newStatic := s.staticIds[id]; newStatic != nil {
		return newStatic(), nil
	}

	if ot == nil {
		err = fmt.Errorf("for this type we need an object tree to create a source")
		return
	}

	// TODO: [MR] get this from objectTree directly
	sbt, err := s.typeProvider.Type(id)
	if err != nil {
		return nil, err
	}
	deps := sourceDeps{
		anytype:        s.anytype,
		statusService:  s.statusService,
		accountService: s.account,
		sbt:            sbt,
		ot:             ot,
	}
	return newTreeSource(id, deps)
}

func (s *service) GetDetailsFromIdBasedSource(id string) (*types.Struct, error) {
	ss, err := s.NewSource(id, nil)
	if err != nil {
		return nil, err
	}
	defer ss.Close()
	if v, ok := ss.(SourceIdEndodedDetails); ok {
		return v.DetailsFromId()
	}
	_ = ss.Close()
	return nil, fmt.Errorf("id unsupported")
}

func (s *service) RegisterStaticSource(id string, new func() Source) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staticIds[id] = new
}

func (s *service) RemoveStaticSource(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.staticIds, id)
}
