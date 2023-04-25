package source

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
)

const CName = "source"

func New() Service {
	return &service{}
}

type Service interface {
	NewSource(id string, spaceID string, buildOptions commonspace.BuildTreeOpts) (source Source, err error)
	RegisterStaticSource(id string, s Source)
	NewStaticSource(id string, sbType model.SmartBlockType, doc *state.State, pushChange func(p PushChangeParams) (string, error)) SourceWithType
	RemoveStaticSource(id string)

	GetDetailsFromIdBasedSource(id string) (*types.Struct, error)
	SourceTypeBySbType(blockType smartblock.SmartBlockType) (SourceType, error)
	app.Component
}

type service struct {
	anytype       core.Service
	statusService status.Service
	sbtProvider   typeprovider.SmartBlockTypeProvider
	account       accountservice.Service
	fileStore     filestore.FileStore
	spaceService  space.Service
	staticIds     map[string]Source
	mu            sync.Mutex
}

func (s *service) Init(a *app.App) (err error) {
	s.staticIds = make(map[string]Source)
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.statusService = a.MustComponent(status.CName).(status.Service)
	s.sbtProvider = a.MustComponent(typeprovider.CName).(typeprovider.SmartBlockTypeProvider)
	s.account = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.spaceService = app.MustComponent[space.Service](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) NewSource(id string, spaceID string, buildOptions commonspace.BuildTreeOpts) (source Source, err error) {
	if id == addr.AnytypeProfileId {
		return NewAnytypeProfile(s.anytype, id), nil
	}
	if id == addr.MissingObject {
		return NewMissingObject(s.anytype), nil
	}
	st, err := s.sbtProvider.Type(id)
	switch st {
	case smartblock.SmartBlockTypeFile:
		return NewFiles(s.anytype, s.fileStore, id), nil
	case smartblock.SmartBlockTypeDate:
		return NewDate(s.anytype, id), nil
	case smartblock.SmartBlockTypeBundledObjectType:
		return NewBundledObjectType(s.anytype, id), nil
	case smartblock.SmartBlockTypeBundledRelation:
		return NewBundledRelation(s.anytype, id), nil
	}

	s.mu.Lock()
	staticSrc := s.staticIds[id]
	s.mu.Unlock()
	if staticSrc != nil {
		return staticSrc, nil
	}

	ctx := context.Background()
	spc, err := s.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return
	}
	var ot objecttree.ObjectTree
	ot, err = spc.BuildTree(ctx, id, buildOptions)
	if err != nil {
		return
	}

	// TODO: [MR] get this from objectTree directly
	sbt, err := s.sbtProvider.Type(id)
	if err != nil {
		return nil, err
	}
	deps := sourceDeps{
		anytype:        s.anytype,
		statusService:  s.statusService,
		accountService: s.account,
		sbt:            sbt,
		ot:             ot,
		spaceService:   s.spaceService,
		sbtProvider:    s.sbtProvider,
	}
	return newTreeSource(id, deps)
}

func (s *service) DetailsFromIdBasedSource(id string) (*types.Struct, error) {
	if !strings.HasPrefix(id, addr.DatePrefix) {
		return nil, fmt.Errorf("unsupported id")
	}
	ss := NewDate(s.anytype, id)
	defer ss.Close()
	if v, ok := ss.(SourceIdEndodedDetails); ok {
		return v.DetailsFromId()
	}
	_ = ss.Close()
	return nil, fmt.Errorf("date source miss the details")
}

func (s *service) GetDetailsFromIdBasedSource(id string) (*types.Struct, error) {
	if !strings.HasPrefix(id, addr.DatePrefix) {
		return nil, fmt.Errorf("id unsupported")
	}
	ss := NewDate(s.anytype, id)
	defer ss.Close()
	if v, ok := ss.(SourceIdEndodedDetails); ok {
		return v.DetailsFromId()
	}
	_ = ss.Close()
	return nil, fmt.Errorf("id unsupported")
}

func (s *service) RegisterStaticSource(id string, src Source) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staticIds[id] = src
	s.sbtProvider.RegisterStaticType(id, smartblock.SmartBlockType(src.Type()))
}

func (s *service) RemoveStaticSource(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.staticIds, id)
}
