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
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "source"

func New() Service {
	return &service{}
}

type Service interface {
	NewSource(ctx context.Context, id string, spaceID string, sbt smartblock.SmartBlockType, buildOptions BuildOptions) (source Source, err error)
	RegisterStaticSource(id string, s Source)
	NewStaticSource(id string, sbType model.SmartBlockType, doc *state.State, pushChange func(p PushChangeParams) (string, error)) SourceWithType
	RemoveStaticSource(id string)

	DetailsFromIdBasedSource(id string) (*types.Struct, error)
	IDsListerBySmartblockType(spaceID string, blockType smartblock.SmartBlockType) (IDsLister, error)
	app.Component
}

type service struct {
	coreService core.Service
	account     accountservice.Service
	fileStore   filestore.FileStore
	fileService files.Service

	mu        sync.Mutex
	staticIds map[string]Source
}

func (s *service) Init(a *app.App) (err error) {
	s.staticIds = make(map[string]Source)
	s.coreService = a.MustComponent(core.CName).(core.Service)
	s.account = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.fileService = app.MustComponent[files.Service](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

type BuildOptions struct {
	BuildTree         func(ctx context.Context) (objecttree.ObjectTree, error)
	DisableRemoteLoad bool
	// TODO Listener is unused, why?
	Listener updatelistener.UpdateListener
}

func (b *BuildOptions) BuildTreeOpts() objecttreebuilder.BuildTreeOpts {
	return objecttreebuilder.BuildTreeOpts{
		Listener: b.Listener,
	}
}

func (s *service) NewSource(ctx context.Context, id string, spaceID string, sbt smartblock.SmartBlockType, buildOptions BuildOptions) (source Source, err error) {
	if id == addr.AnytypeProfileId {
		return NewAnytypeProfile(id), nil
	}
	if id == addr.MissingObject {
		return NewMissingObject(), nil
	}
	switch sbt {
	case smartblock.SmartBlockTypeFile:
		return NewFile(s.coreService, s.fileStore, s.fileService, id), nil
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

	ot, err := buildOptions.BuildTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	deps := sourceDeps{
		coreService:    s.coreService,
		accountService: s.account,
		sbt:            sbt,
		ot:             ot,
		fileService:    s.fileService,
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
		return s.NewStaticSource("", model.SmartBlockType_BundledTemplate, nil, nil), nil
	default:
		if err := blockType.Valid(); err != nil {
			return nil, err
		}
		return &source{
			spaceID:        spaceID,
			smartblockType: blockType,
			coreService:    s.coreService,
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

func (s *service) RegisterStaticSource(id string, src Source) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staticIds[id] = src
	// s.sbtProvider.RegisterStaticType(id, smartblock.SmartBlockType(src.Type()))
}

func (s *service) RemoveStaticSource(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.staticIds, id)
}
