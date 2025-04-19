package source

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/spacestorage"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idderiver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/keyvalueservice"
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

	DetailsFromIdBasedSource(id domain.FullID) (*domain.Details, error)
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
	idDeriver          idderiver.Deriver
	keyValueService    keyvalueservice.Service

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
	s.idDeriver = app.MustComponent[idderiver.Deriver](a)
	s.keyValueService = app.MustComponent[keyvalueservice.Service](a)

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
		TreeBuilder: func(treeStorage objecttree.Storage, aclList list.AclList) (objecttree.ObjectTree, error) {
			ot, err := objecttree.BuildKeyFilterableObjectTree(treeStorage, aclList)
			if err != nil {
				return nil, err
			}
			sbt, _, err := typeprovider.GetTypeAndKeyFromRoot(ot.Header())
			if err != nil {
				return nil, err
			}
			if sbt == smartblock.SmartBlockTypeChatDerivedObject || sbt == smartblock.SmartBlockTypeAccountObject {
				ot.SetFlusher(objecttree.MarkNewChangeFlusher())
			}
			return ot, nil
		},
		TreeValidator: objecttree.ValidateFilterRawTree,
	}
}

func (s *service) NewSource(ctx context.Context, space Space, id string, buildOptions BuildOptions) (Source, error) {
	src, err := s.newSource(ctx, space, id, buildOptions)
	if err != nil {
		return nil, err
	}
	err = s.objectStore.BindSpaceId(src.SpaceID(), src.Id())
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
			typeId, err := space.GetTypeIdByKey(context.Background(), bundle.TypeKeyDate)
			if err != nil {
				return nil, fmt.Errorf("failed to find Date type to build Date object: %w", err)
			}
			return NewDate(DateSourceParams{
				Id: domain.FullID{
					ObjectID: id,
					SpaceID:  space.Id(),
				},
				DateObjectTypeId: typeId,
			}), nil
		case smartblock.SmartBlockTypeBundledObjectType:
			return NewBundledObjectType(id), nil
		case smartblock.SmartBlockTypeBundledRelation:
			return NewBundledRelation(id), nil
		case smartblock.SmartBlockTypeParticipant:
			spaceId, _, err := domain.ParseParticipantId(id)
			if err != nil {
				return nil, err
			}
			if spaceId != space.Id() {
				return nil, fmt.Errorf("invalid space id for participant object")
			}
			participantState := state.NewDoc(id, nil).(*state.State)
			// Set object type here in order to derive value of Type relation in smartblock.Init
			participantState.SetObjectTypeKey(bundle.TypeKeyParticipant)
			params := StaticSourceParams{
				Id: domain.FullID{
					ObjectID: id,
					SpaceID:  spaceId,
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

func (s *service) DetailsFromIdBasedSource(id domain.FullID) (*domain.Details, error) {
	if !strings.HasPrefix(id.ObjectID, addr.DatePrefix) {
		return nil, fmt.Errorf("unsupported id")
	}

	dateTypeId, err := s.idDeriver.DeriveObjectId(context.Background(), id.SpaceID,
		domain.MustUniqueKey(smartblock.SmartBlockTypeObjectType, bundle.TypeKeyDate.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to derive id of Date type object: %w", err)
	}

	ss := NewDate(DateSourceParams{
		Id:               id,
		DateObjectTypeId: dateTypeId,
	})
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
	err := s.objectStore.BindSpaceId(src.SpaceID(), src.Id())
	if err != nil {
		return fmt.Errorf("store space id for object: %w", err)
	}
	s.sbtProvider.RegisterStaticType(src.Id(), src.Type())
	return nil
}
