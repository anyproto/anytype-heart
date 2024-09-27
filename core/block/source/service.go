package source

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/spacestorage"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

const CName = "source"

func New() Service {
	return &service{}
}

type accountService interface {
	AccountID() string
	MyParticipantId(string) string
	PersonalSpaceID() string
}

type Space interface {
	Id() string
	TreeBuilder() objecttreebuilder.TreeBuilder
	GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error)
	GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error)
	DeriveObjectID(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error)
	StoredIds() []string
	IsPersonal() bool
}

type Service interface {
	NewSource(ctx context.Context, space Space, id string, buildOptions BuildOptions) (source Source, err error)
	RegisterStaticSource(s Source) error
	NewStaticSource(params StaticSourceParams) SourceWithType

	DetailsFromIdBasedSource(id string) (*types.Struct, error)
	IDsListerBySmartblockType(space Space, blockType smartblock.SmartBlockType) (IDsLister, error)
	app.Component
}

type service struct {
	sbtProvider        typeprovider.SmartBlockTypeProvider
	accountService     accountService
	accountKeysService accountservice.Service
	storageService     storage.ClientStorage
	fileService        files.Service
	objectStore        objectstore.ObjectStore
	fileObjectMigrator fileObjectMigrator

	mu        sync.Mutex
	staticIds map[string]Source
}

func (s *service) Init(a *app.App) (err error) {
	s.staticIds = make(map[string]Source)

	s.sbtProvider = a.MustComponent(typeprovider.CName).(typeprovider.SmartBlockTypeProvider)
	s.accountService = app.MustComponent[accountService](a)
	s.accountKeysService = a.MustComponent(accountservice.CName).(accountservice.Service)
	s.storageService = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)

	s.fileService = app.MustComponent[files.Service](a)
	s.fileObjectMigrator = app.MustComponent[fileObjectMigrator](a)
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
		TreeBuilder: func(treeStorage treestorage.TreeStorage, aclList list.AclList) (objecttree.ObjectTree, error) {
			ot, err := objecttree.BuildKeyFilterableObjectTree(treeStorage, aclList)
			if err != nil {
				return nil, err
			}
			sbt, _, err := typeprovider.GetTypeAndKeyFromRoot(ot.Header())
			if err != nil {
				return nil, err
			}
			if sbt == smartblock.SmartBlockTypeChatDerivedObject {
				// here we have special
				ot.SetFlusher(objecttree.MarkNewChangeFlusher())
			}
			return ot, nil
		},
		TreeValidator: func(payload treestorage.TreeStorageCreatePayload, buildFunc objecttree.BuildObjectTreeFunc, aclList list.AclList) (retPayload treestorage.TreeStorageCreatePayload, err error) {
			return objecttree.ValidateFilterRawTree(payload, aclList)
		},
	}
}

func (s *service) NewSource(ctx context.Context, space Space, id string, buildOptions BuildOptions) (Source, error) {
	src, err := s.newSource(ctx, space, id, buildOptions)
	if err != nil {
		return nil, err
	}
	err = s.storageService.BindSpaceID(src.SpaceID(), src.Id())
	if err != nil {
		return nil, fmt.Errorf("store space id for object: %w", err)
	}
	return src, nil
}

func (s *service) newSource(ctx context.Context, space Space, id string, buildOptions BuildOptions) (Source, error) {
	if id == addr.AnytypeProfileId {
		return NewAnytypeProfile(id), nil
	}
	if id == addr.MissingObject {
		return NewMissingObject(), nil
	}
	st, err := typeprovider.SmartblockTypeFromID(id)
	if err == nil {
		switch st {
		case smartblock.SmartBlockTypeDate:
			return NewDate(space, id), nil
		case smartblock.SmartBlockTypeBundledObjectType:
			return NewBundledObjectType(id), nil
		case smartblock.SmartBlockTypeBundledRelation:
			return NewBundledRelation(id), nil
		case smartblock.SmartBlockTypeParticipant:
			participantState := state.NewDoc(id, nil).(*state.State)
			// Set object type here in order to derive value of Type relation in smartblock.Init
			participantState.SetObjectTypeKey(bundle.TypeKeyParticipant)
			params := StaticSourceParams{
				Id: domain.FullID{
					ObjectID: id,
					SpaceID:  space.Id(),
				},
				State:     participantState,
				SbType:    smartblock.SmartBlockTypeParticipant,
				CreatorId: addr.AnytypeProfileId,
			}
			return s.NewStaticSource(params), nil
		}
	}

	s.mu.Lock()
	staticSrc := s.staticIds[id]
	s.mu.Unlock()
	if staticSrc != nil {
		return staticSrc, nil
	}

	return s.newTreeSource(ctx, space, id, buildOptions.BuildTreeOpts())
}

func (s *service) IDsListerBySmartblockType(space Space, blockType smartblock.SmartBlockType) (IDsLister, error) {
	switch blockType {
	case smartblock.SmartBlockTypeAnytypeProfile:
		return &anytypeProfile{}, nil
	case smartblock.SmartBlockTypeMissingObject:
		return &missingObject{}, nil
	case smartblock.SmartBlockTypeBundledObjectType:
		return &bundledObjectType{}, nil
	case smartblock.SmartBlockTypeBundledRelation:
		return &bundledRelation{}, nil
	case smartblock.SmartBlockTypeBundledTemplate:
		params := StaticSourceParams{
			SbType:    smartblock.SmartBlockTypeBundledTemplate,
			CreatorId: addr.AnytypeProfileId,
		}
		return s.NewStaticSource(params), nil
	default:
		if err := blockType.Valid(); err != nil {
			return nil, err
		}
		return &source{
			space:          space,
			spaceID:        space.Id(),
			smartblockType: blockType,
			sbtProvider:    s.sbtProvider,
		}, nil
	}
}

func (s *service) DetailsFromIdBasedSource(id string) (*types.Struct, error) {
	if !strings.HasPrefix(id, addr.DatePrefix) {
		return nil, fmt.Errorf("unsupported id")
	}
	// TODO Fix this, but how? It's broken by design, because no one pass spaceId here
	ss := NewDate(nil, id)
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
