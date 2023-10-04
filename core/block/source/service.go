package source

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

const CName = "source"

func New() Service {
	return &service{}
}

type idResolver interface {
	BindSpaceID(spaceID string, objectID string) error
	ResolveSpaceID(objectID string) (spaceID string, err error)
}

type Service interface {
	NewSource(ctx context.Context, id string, spaceID string, buildOptions BuildOptions) (source Source, err error)
	RegisterStaticSource(s Source) error
	NewStaticSource(id domain.FullID, sbType smartblock.SmartBlockType, doc *state.State, pushChange func(p PushChangeParams) (string, error)) SourceWithType
	RemoveStaticSource(id string)

	DetailsFromIdBasedSource(id string) (*types.Struct, error)
	IDsListerBySmartblockType(spaceID string, blockType smartblock.SmartBlockType) (IDsLister, error)
	app.Component
}

type service struct {
	coreService         core.Service
	sbtProvider         typeprovider.SmartBlockTypeProvider
	account             accountservice.Service
	fileStore           filestore.FileStore
	spaceService        spacecore.SpaceCoreService
	storageService      storage.ClientStorage
	fileService         files.Service
	systemObjectService system_object.Service

	objectStore objectstore.ObjectStore

	mu        sync.Mutex
	staticIds map[string]Source
}

func (s *service) Init(a *app.App) (err error) {
	s.staticIds = make(map[string]Source)
	s.coreService = a.MustComponent(core.CName).(core.Service)
	s.sbtProvider = a.MustComponent(typeprovider.CName).(typeprovider.SmartBlockTypeProvider)
	s.account = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.spaceService = app.MustComponent[spacecore.SpaceCoreService](a)
	s.storageService = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	s.systemObjectService = app.MustComponent[system_object.Service](a)

	s.fileService = app.MustComponent[files.Service](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

type BuildOptions struct {
	DisableRemoteLoad bool
	Listener          updatelistener.UpdateListener
}

func (b *BuildOptions) BuildTreeOpts() objecttreebuilder.BuildTreeOpts {
	return objecttreebuilder.BuildTreeOpts{
		Listener: b.Listener,
	}
}

func (s *service) NewSource(ctx context.Context, id string, spaceID string, buildOptions BuildOptions) (Source, error) {
	src, err := s.newSource(ctx, id, spaceID, buildOptions)
	if err != nil {
		return nil, err
	}
	err = s.storageService.BindSpaceID(src.SpaceID(), src.Id())
	if err != nil {
		return nil, fmt.Errorf("store space id for object: %w", err)
	}
	return src, nil
}

func (s *service) newSource(ctx context.Context, id string, spaceID string, buildOptions BuildOptions) (Source, error) {
	if id == addr.AnytypeProfileId {
		return NewAnytypeProfile(id), nil
	}
	if id == addr.MissingObject {
		return NewMissingObject(), nil
	}
	st, _ := s.sbtProvider.Type(spaceID, id)
	switch st {
	case smartblock.SmartBlockTypeFile:
		return NewFile(s.coreService, s.fileStore, s.fileService, spaceID, id), nil
	case smartblock.SmartBlockTypeDate:
		return NewDate(spaceID, id, s.coreService), nil
	case smartblock.SmartBlockTypeBundledObjectType:
		return NewBundledObjectType(id), nil
	case smartblock.SmartBlockTypeBundledRelation:
		return NewBundledRelation(id), nil
	}

	s.mu.Lock()
	staticSrc := s.staticIds[id]
	s.mu.Unlock()
	if staticSrc != nil {
		return staticSrc, nil
	}

	spc, err := s.spaceService.Get(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	var ot objecttree.ObjectTree
	ot, err = spc.TreeBuilder().BuildTree(ctx, id, buildOptions.BuildTreeOpts())
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	sbt, err := typeprovider.GetTypeFromRoot(ot.Header())
	if err != nil {
		return nil, err
	}
	deps := sourceDeps{
		coreService:         s.coreService,
		accountService:      s.account,
		sbt:                 sbt,
		ot:                  ot,
		spaceService:        s.spaceService,
		sbtProvider:         s.sbtProvider,
		fileService:         s.fileService,
		systemObjectService: s.systemObjectService,
	}
	return newTreeSource(spaceID, id, deps)
}

func (s *service) IDsListerBySmartblockType(spaceID string, blockType smartblock.SmartBlockType) (IDsLister, error) {
	switch blockType {
	case smartblock.SmartBlockTypeAnytypeProfile:
		return &anytypeProfile{}, nil
	case smartblock.SmartBlockTypeMissingObject:
		return &missingObject{}, nil
	case smartblock.SmartBlockTypeFile:
		return &file{a: s.coreService, fileStore: s.fileStore}, nil
	case smartblock.SmartBlockTypeBundledObjectType:
		return &bundledObjectType{}, nil
	case smartblock.SmartBlockTypeBundledRelation:
		return &bundledRelation{}, nil
	case smartblock.SmartBlockTypeBundledTemplate:
		return s.NewStaticSource(domain.FullID{}, smartblock.SmartBlockTypeBundledTemplate, nil, nil), nil
	default:
		if err := blockType.Valid(); err != nil {
			return nil, err
		}
		return &source{
			spaceID:        spaceID,
			smartblockType: blockType,
			coreService:    s.coreService,
			spaceService:   s.spaceService,
			sbtProvider:    s.sbtProvider,
		}, nil
	}
}

func (s *service) DetailsFromIdBasedSource(id string) (*types.Struct, error) {
	if !strings.HasPrefix(id, addr.DatePrefix) {
		return nil, fmt.Errorf("unsupported id")
	}
	ss := NewDate("", id, s.coreService)
	defer ss.Close()
	if v, ok := ss.(SourceIdEndodedDetails); ok {
		return v.DetailsFromId()
	}
	_ = ss.Close()
	return nil, fmt.Errorf("date source miss the details")
}

func (s *service) RegisterStaticSource(src Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staticIds[src.Id()] = src
	err := s.storageService.BindSpaceID(src.SpaceID(), src.Id())
	if err != nil {
		return fmt.Errorf("store space id for object: %w", err)
	}
	s.sbtProvider.RegisterStaticType(src.Id(), src.Type())
	return nil
}

func (s *service) RemoveStaticSource(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.staticIds, id)
}
