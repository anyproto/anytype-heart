package spacefactory

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/space/clientspace"
	dependencies "github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/marketplacespace"
	"github.com/anyproto/anytype-heart/space/internal/personalspace"
	"github.com/anyproto/anytype-heart/space/internal/shareablespace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/streamablespace"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

type SpaceFactory interface {
	app.Component
	CreatePersonalSpace(ctx context.Context, metadata []byte) (sp spacecontroller.SpaceController, err error)
	NewPersonalSpace(ctx context.Context, metadata []byte) (spacecontroller.SpaceController, error)
	CreateShareableSpace(ctx context.Context, id string, desc *spaceinfo.SpaceDescription) (sp spacecontroller.SpaceController, err error)
	NewShareableSpace(ctx context.Context, id string, info spaceinfo.SpacePersistentInfo) (spacecontroller.SpaceController, error)
	CreateStreamableSpace(ctx context.Context, privKey crypto.PrivKey, id string, metadata []byte) (spacecontroller.SpaceController, error)
	NewStreamableSpace(ctx context.Context, id string, info spaceinfo.SpacePersistentInfo, metadata []byte) (spacecontroller.SpaceController, error)
	CreateActiveSpace(ctx context.Context, id, aclHeadId string) (sp spacecontroller.SpaceController, err error)
	CreateMarketplaceSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error)
	CreateAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error)
	LoadAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error)
	CreateInvitingSpace(ctx context.Context, id, aclHeadId string) (sp spacecontroller.SpaceController, err error)
	CreateOneToOneSpace(ctx context.Context, id string, description *spaceinfo.SpaceDescription, participantData spaceinfo.OneToOneParticipantData) (sp spacecontroller.SpaceController, err error)
}

const CName = "client.space.spacefactory"

type spaceFactory struct {
	app             *app.App
	spaceCore       spacecore.SpaceCoreService
	techSpace       techspace.TechSpace
	accountService  accountservice.Service
	objectFactory   objectcache.ObjectFactory
	indexer         dependencies.SpaceIndexer
	installer       dependencies.BundledObjectsInstaller
	storageService  storage.ClientStorage
	personalSpaceId string
}

func New() SpaceFactory {
	return &spaceFactory{}
}

func (s *spaceFactory) Init(a *app.App) (err error) {
	s.app = a
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.objectFactory = app.MustComponent[objectcache.ObjectFactory](a)
	s.indexer = app.MustComponent[dependencies.SpaceIndexer](a)
	s.installer = app.MustComponent[dependencies.BundledObjectsInstaller](a)
	s.storageService = app.MustComponent[storage.ClientStorage](a)
	s.personalSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacedomain.SpaceTypeRegular)
	if err != nil {
		return
	}
	return
}

func (s *spaceFactory) CreatePersonalSpace(ctx context.Context, metadata []byte) (sp spacecontroller.SpaceController, err error) {
	coreSpace, err := s.spaceCore.Derive(ctx, spacedomain.SpaceTypeRegular)
	if err != nil {
		return
	}
	err = coreSpace.Storage().(anystorage.ClientSpaceStorage).MarkSpaceCreated(ctx)
	if err != nil {
		return
	}
	info := spaceinfo.NewSpacePersistentInfo(coreSpace.Id())
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
	if err := s.techSpace.SpaceViewCreate(ctx, coreSpace.Id(), true, info, nil); err != nil {
		if errors.Is(err, techspace.ErrSpaceViewExists) {
			return s.NewPersonalSpace(ctx, metadata)
		}
		return nil, err
	}
	ctrl, err := personalspace.NewSpaceController(ctx, coreSpace.Id(), metadata, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) NewPersonalSpace(ctx context.Context, metadata []byte) (ctrl spacecontroller.SpaceController, err error) {
	id, err := s.spaceCore.DeriveID(ctx, spacedomain.SpaceTypeRegular)
	if err != nil {
		return nil, err
	}
	ctrl, err = personalspace.NewSpaceController(ctx, id, metadata, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) CreateAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	techSpace := techspace.New()
	techCoreSpace, err := s.spaceCore.Derive(ctx, spacedomain.SpaceTypeTech)
	if err != nil {
		return nil, fmt.Errorf("derive tech space: %w", err)
	}
	kvObserver := techCoreSpace.KeyValueObserver()
	deps := clientspace.TechSpaceDeps{
		CommonSpace:      techCoreSpace,
		ObjectFactory:    s.objectFactory,
		AccountService:   s.accountService,
		PersonalSpaceId:  s.personalSpaceId,
		Indexer:          s.indexer,
		Installer:        s.installer,
		TechSpace:        techSpace,
		KeyValueObserver: kvObserver,
	}
	ts, err := clientspace.NewTechSpace(deps)
	if err != nil {
		return nil, fmt.Errorf("build tech space: %w", err)
	}

	s.techSpace = ts
	s.app = s.app.ChildApp()
	s.app.Register(s.techSpace)
	err = ts.Run(techCoreSpace, ts.Cache, true)
	if err != nil {
		return nil, fmt.Errorf("run tech space: %w", err)
	}

	return ts, nil
}

func (s *spaceFactory) LoadAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	techSpace := techspace.New()
	id, err := s.spaceCore.DeriveID(ctx, spacedomain.SpaceTypeTech)
	if err != nil {
		return nil, fmt.Errorf("derive tech space id: %w", err)
	}
	techCoreSpace, err := s.spaceCore.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("derive tech space: %w", err)
	}
	kvObserver := techCoreSpace.KeyValueObserver()
	deps := clientspace.TechSpaceDeps{
		CommonSpace:      techCoreSpace,
		ObjectFactory:    s.objectFactory,
		AccountService:   s.accountService,
		PersonalSpaceId:  s.personalSpaceId,
		Indexer:          s.indexer,
		Installer:        s.installer,
		TechSpace:        techSpace,
		KeyValueObserver: kvObserver,
	}
	ts, err := clientspace.NewTechSpace(deps)
	if err != nil {
		return nil, fmt.Errorf("build tech space: %w", err)
	}
	s.techSpace = ts
	s.app = s.app.ChildApp()
	s.app.Register(s.techSpace)
	err = ts.Run(techCoreSpace, ts.Cache, false)
	if err != nil {
		return nil, fmt.Errorf("run tech space: %w", err)
	}
	err = s.indexer.ReindexSpace(ts)
	if err != nil {
		return nil, fmt.Errorf("reindex tech space: %w", err)
	}
	return ts, nil
}

func (s *spaceFactory) NewShareableSpace(ctx context.Context, id string, info spaceinfo.SpacePersistentInfo) (spacecontroller.SpaceController, error) {
	ctrl, err := shareablespace.NewSpaceController(id, info, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) CreateInvitingSpace(ctx context.Context, id, aclHeadId string) (sp spacecontroller.SpaceController, err error) {
	exists, err := s.techSpace.SpaceViewExists(ctx, id)
	if err != nil {
		return
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusJoining)
	if !exists {
		if err := s.techSpace.SpaceViewCreate(ctx, id, true, info, nil); err != nil {
			return nil, err
		}
	}
	ctrl, err := shareablespace.NewSpaceController(id, info, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) CreateActiveSpace(ctx context.Context, id, aclHeadId string) (sp spacecontroller.SpaceController, err error) {
	exists, err := s.techSpace.SpaceViewExists(ctx, id)
	if err != nil {
		return
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusActive)
	if !exists {
		if err := s.techSpace.SpaceViewCreate(ctx, id, true, info, nil); err != nil {
			return nil, err
		}
	}
	ctrl, err := shareablespace.NewSpaceController(id, info, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

// creates regular shared space
func (s *spaceFactory) CreateShareableSpace(ctx context.Context, id string, spaceDesc *spaceinfo.SpaceDescription) (sp spacecontroller.SpaceController, err error) {
	coreSpace, err := s.spaceCore.Get(ctx, id)
	if err != nil {
		return
	}
	err = coreSpace.Storage().(anystorage.ClientSpaceStorage).MarkSpaceCreated(ctx)
	if err != nil {
		return
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
	if err := s.techSpace.SpaceViewCreate(ctx, id, true, info, spaceDesc); err != nil {
		return nil, err
	}
	ctrl, err := shareablespace.NewSpaceController(id, info, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) CreateStreamableSpace(ctx context.Context, privKey crypto.PrivKey, id string, metadata []byte) (spacecontroller.SpaceController, error) {
	encodedKey, err := crypto.EncodeKeyToString(privKey)
	if err != nil {
		return nil, err
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown).
		SetEncodedKey(encodedKey)
	if err := s.techSpace.SpaceViewCreate(ctx, id, false, info, nil); err != nil {
		return nil, err
	}
	return s.NewStreamableSpace(ctx, id, info, metadata)
}

func (s *spaceFactory) NewStreamableSpace(ctx context.Context, id string, info spaceinfo.SpacePersistentInfo, metadata []byte) (spacecontroller.SpaceController, error) {
	decodedSignKey, err := crypto.DecodeKeyFromString(
		info.EncodedKey,
		crypto.UnmarshalEd25519PrivateKey,
		nil)
	ctrl, err := streamablespace.NewSpaceController(ctx, id, decodedSignKey, metadata, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) CreateMarketplaceSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error) {
	ctrl := marketplacespace.NewSpaceController(s.app, s.personalSpaceId)
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) CreateOneToOneSpace(ctx context.Context, spaceId string, description *spaceinfo.SpaceDescription, participantData spaceinfo.OneToOneParticipantData) (sp spacecontroller.SpaceController, err error) {
	oneToOneSpace, err := s.spaceCore.Get(ctx, spaceId)
	if err != nil {
		return
	}

	err = oneToOneSpace.Storage().(anystorage.ClientSpaceStorage).MarkSpaceCreated(ctx)
	if err != nil {
		return
	}

	info := spaceinfo.NewSpacePersistentInfo(spaceId)
	info.OneToOneIdentity = participantData.Identity
	info.Name = description.Name
	requestMetadataKeyStr := base64.StdEncoding.EncodeToString(participantData.RequestMetadataKey)
	info.OneToOneRequestMetadataKey = requestMetadataKeyStr
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown)

	spaceView, err := s.techSpace.GetSpaceView(ctx, spaceId)
	if err != nil {
		if !errors.Is(err, techspace.ErrSpaceViewNotExists) {
			return nil, fmt.Errorf("get space view: %w", err)
		}
	}

	// nolint: nestif
	if spaceView == nil {
		if err := s.techSpace.SpaceViewCreate(ctx, spaceId, true, info, description); err != nil {
			return nil, err
		}
	} else {
		// check if space is active
		existingLocalInfo := spaceView.GetLocalInfo()
		if existingLocalInfo.GetLocalStatus() != spaceinfo.LocalStatusOk {
			// space has been removed, reset statuses and recreate
			localInfo := spaceinfo.NewSpaceLocalInfo(spaceId)
			localInfo.SetLocalStatus(spaceinfo.LocalStatusUnknown)
			localInfo.SetRemoteStatus(spaceinfo.RemoteStatusUnknown)
			if err := spaceView.SetSpaceLocalInfo(localInfo); err != nil {
				return nil, err
			}
			if err := spaceView.SetSpacePersistentInfo(info); err != nil {
				return nil, err
			}
		}
	}

	ctrl, err := shareablespace.NewSpaceController(spaceId, info, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) Name() (name string) {
	return CName
}
