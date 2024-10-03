package clientspace

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/headsync"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/profilemigration"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/internal/objectprovider"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/peermanager"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

type Space interface {
	objectcache.Cache
	objectprovider.ObjectProvider

	Id() string
	TreeBuilder() objecttreebuilder.TreeBuilder
	DebugAllHeads() []headsync.TreeHeads
	DeleteTree(ctx context.Context, id string) (err error)
	StoredIds() []string
	Storage() spacestorage.SpaceStorage

	DerivedIDs() threads.DerivedSmartblockIds

	WaitMandatoryObjects(ctx context.Context) (err error)
	CommonSpace() commonspace.Space

	Do(objectId string, apply func(sb smartblock.SmartBlock) error) error
	DoCtx(ctx context.Context, objectId string, apply func(sb smartblock.SmartBlock) error) error
	GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error)
	GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error)

	IsReadOnly() bool
	IsPersonal() bool

	Close(ctx context.Context) error
}

type spaceIndexer interface {
	ReindexMarketplaceSpace(space Space) error
	ReindexSpace(space Space) error
	RemoveIndexes(spaceID string) (err error)
}

type bundledObjectsInstaller interface {
	InstallBundledObjects(ctx context.Context, spc Space, ids []string, isNewSpace bool) ([]string, []*types.Struct, error)

	BundledObjectsIdsToInstall(ctx context.Context, spc Space, sourceObjectIds []string) (objectIds []string, err error)
}

var log = logger.NewNamed("client.space")
var BundledObjectsPeerFindTimeout = time.Second * 30

type space struct {
	objectcache.Cache
	objectprovider.ObjectProvider

	indexer         spaceIndexer
	derivedIDs      threads.DerivedSmartblockIds
	installer       bundledObjectsInstaller
	spaceCore       spacecore.SpaceCoreService
	personalSpaceId string

	myIdentity crypto.PubKey
	common     commonspace.Space

	loadMandatoryObjectsCh  chan struct{}
	loadMandatoryObjectsErr error
}

type SpaceDeps struct {
	Indexer           spaceIndexer
	Installer         bundledObjectsInstaller
	CommonSpace       commonspace.Space
	ObjectFactory     objectcache.ObjectFactory
	AccountService    accountservice.Service
	StorageService    storage.ClientStorage
	SpaceCore         spacecore.SpaceCoreService
	PersonalSpaceId   string
	LoadCtx           context.Context
	DisableRemoteLoad bool
}

func BuildSpace(ctx context.Context, deps SpaceDeps) (Space, error) {
	sp := &space{
		indexer:                deps.Indexer,
		installer:              deps.Installer,
		common:                 deps.CommonSpace,
		personalSpaceId:        deps.PersonalSpaceId,
		spaceCore:              deps.SpaceCore,
		myIdentity:             deps.AccountService.Account().SignKey.GetPublic(),
		loadMandatoryObjectsCh: make(chan struct{}),
	}
	sp.Cache = objectcache.New(deps.AccountService, deps.ObjectFactory, deps.PersonalSpaceId, sp)
	sp.ObjectProvider = objectprovider.NewObjectProvider(deps.CommonSpace.Id(), deps.PersonalSpaceId, sp.Cache)
	var err error
	sp.derivedIDs, err = sp.ObjectProvider.DeriveObjectIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("derive object ids: %w", err)
	}
	if deps.StorageService.IsSpaceCreated(deps.CommonSpace.Id()) {
		err = sp.ObjectProvider.CreateMandatoryObjects(ctx, sp)
		if err != nil {
			return nil, fmt.Errorf("create mandatory objects: %w", err)
		}
		err = deps.StorageService.UnmarkSpaceCreated(deps.CommonSpace.Id())
		if err != nil {
			return nil, fmt.Errorf("unmark space created: %w", err)
		}
		if err = sp.InstallBundledObjects(ctx); err != nil {
			return nil, fmt.Errorf("install bundled objects: %w", err)
		}
	}
	go sp.mandatoryObjectsLoad(deps.LoadCtx, deps.DisableRemoteLoad)
	return sp, nil
}

func (s *space) mandatoryObjectsLoad(ctx context.Context, disableRemoteLoad bool) {
	defer close(s.loadMandatoryObjectsCh)
	s.loadMandatoryObjectsErr = s.indexer.ReindexSpace(s)
	if s.loadMandatoryObjectsErr != nil {
		return
	}
	loadCtx := ctx
	if !disableRemoteLoad {
		loadCtx = peer.CtxWithPeerId(ctx, peer.CtxResponsiblePeers)
	}
	s.loadMandatoryObjectsErr = s.LoadObjects(loadCtx, s.derivedIDs.IDs())
	if s.loadMandatoryObjectsErr != nil {
		return
	}
	ctxWithPeerTimeout := context.WithValue(loadCtx, peermanager.ContextPeerFindDeadlineKey, time.Now().Add(BundledObjectsPeerFindTimeout))
	if err := s.TryLoadBundledObjects(ctxWithPeerTimeout); err != nil {
		log.Error("failed to load bundled objects", zap.Error(err))
	}
	s.loadMandatoryObjectsErr = s.InstallBundledObjects(ctx)
	if s.loadMandatoryObjectsErr != nil {
		return
	}
	err := s.migrationProfileObject(ctx)
	if err != nil {
		log.Error("failed to migrate profile object", zap.Error(err))
	}
	if !disableRemoteLoad {
		s.common.TreeSyncer().StartSync()
	}
}

func (s *space) Id() string {
	return s.common.Id()
}

func (s *space) TreeBuilder() objecttreebuilder.TreeBuilder {
	return s.common.TreeBuilder()
}

func (s *space) DebugAllHeads() []headsync.TreeHeads {
	return s.common.DebugAllHeads()
}

func (s *space) DeleteTree(ctx context.Context, id string) (err error) {
	return s.common.DeleteTree(ctx, id)
}

func (s *space) StoredIds() []string {
	return s.common.StoredIds()
}

func (s *space) Storage() spacestorage.SpaceStorage {
	return s.common.Storage()
}

func (s *space) DerivedIDs() threads.DerivedSmartblockIds {
	return s.derivedIDs
}

func (s *space) CommonSpace() commonspace.Space {
	return s.common
}

func (s *space) WaitMandatoryObjects(ctx context.Context) (err error) {
	select {
	case <-s.loadMandatoryObjectsCh:
		return s.loadMandatoryObjectsErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *space) Do(objectId string, apply func(sb smartblock.SmartBlock) error) error {
	return s.DoCtx(context.Background(), objectId, apply)
}

func (s *space) DoCtx(ctx context.Context, objectId string, apply func(sb smartblock.SmartBlock) error) error {
	sb, err := s.GetObject(ctx, objectId)
	if err != nil {
		return err
	}
	sb.Lock()
	defer sb.Unlock()
	return apply(sb)
}

func (s *space) GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error) {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, key.String())
	if err != nil {
		return "", err
	}
	return s.DeriveObjectID(ctx, uk)
}

func (s *space) GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error) {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, key.String())
	if err != nil {
		return "", err
	}
	return s.DeriveObjectID(ctx, uk)
}

func (s *space) IsPersonal() bool {
	return s.Id() == s.personalSpaceId
}

func (s *space) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	err := s.Cache.Close(ctx)
	if err != nil {
		return err
	}
	if s.spaceCore != nil {
		// we need to remove it from space cache also
		return s.spaceCore.CloseSpace(ctx, s.Id())
	}
	return s.common.Close()
}

func (s *space) InstallBundledObjects(ctx context.Context) error {
	ids := make([]string, 0, len(bundle.SystemTypes)+len(bundle.SystemRelations))
	for _, ot := range bundle.SystemTypes {
		ids = append(ids, ot.BundledURL())
	}
	for _, rk := range bundle.SystemRelations {
		ids = append(ids, rk.BundledURL())
	}
	_, _, err := s.installer.InstallBundledObjects(ctx, s, ids, true)
	if err != nil {
		return err
	}
	return nil
}

func (s *space) TryLoadBundledObjects(ctx context.Context) error {
	st := time.Now()
	defer func() {
		if dur := time.Since(st); dur > time.Millisecond*200 {
			log.Warn("load bundled objects", zap.Duration("duration", dur))
		}
	}()

	ids := make([]string, 0, len(bundle.SystemTypes)+len(bundle.SystemRelations))
	for _, ot := range bundle.SystemTypes {
		ids = append(ids, ot.BundledURL())
	}
	for _, rk := range bundle.SystemRelations {
		ids = append(ids, rk.BundledURL())
	}
	objectIds, err := s.installer.BundledObjectsIdsToInstall(ctx, s, ids)
	if err != nil {
		return err
	}
	storedIds, err := s.Storage().StoredIds()
	if err != nil {
		return err
	}
	// only load objects that are not already stored
	objectIds = slice.Difference(objectIds, storedIds)
	s.LoadObjectsIgnoreErrs(ctx, objectIds)
	return nil
}

func (s *space) migrationProfileObject(ctx context.Context) error {
	if !s.IsPersonal() {
		return nil
	}
	if s.derivedIDs.Profile == "" {
		return nil
	}

	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypePage, profilemigration.InternalKeyOldProfileData)
	if err != nil {
		return err
	}
	// lets do the cheap check if we already has this extracted object
	extractedProfileId, err := s.DeriveObjectID(ctx, uniqueKey)
	if err != nil {
		return err
	}

	extractedProfileExists, _ := s.Storage().HasTree(extractedProfileId)
	if extractedProfileExists {
		return nil
	}

	return s.Do(s.derivedIDs.Profile, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		extractedState, err := profilemigration.ExtractCustomState(st)
		if err != nil {
			if err == profilemigration.ErrNoCustomStateFound {
				log.Error("no extra state found")
				return nil
			}
			return err
		}

		payload, err := s.DeriveTreePayload(ctx, payloadcreator.PayloadDerivationParams{UseAccountSignature: true, Key: uniqueKey})
		if err != nil {
			return err
		}
		newSb, err := s.CreateTreeObjectWithPayload(ctx, payload, func(id string) *smartblock.InitContext {
			extractedState.SetRootId(id)
			return &smartblock.InitContext{
				IsNewObject:    true,
				ObjectTypeKeys: []domain.TypeKey{bundle.TypeKeyPage},
				State:          extractedState,
				SpaceID:        s.Id(),
			}
		})
		if err != nil {
			return err
		}
		log.Warn("old profile custom state migrated")
		newSb.Close()

		return sb.Apply(st)
	})
}

func (s *space) IsReadOnly() bool {
	return !s.CommonSpace().Acl().AclState().Permissions(s.myIdentity).CanWrite()
}
