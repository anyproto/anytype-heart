package space

import (
	"context"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	space "github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) create(ctx context.Context, description *spaceinfo.SpaceDescription) (sp clientspace.Space, err error) {
	var spaceType = space.SpaceType
	if description != nil && description.SpaceUxType == model.SpaceUxType_Chat {
		spaceType = space.ChatSpaceType
	}
	coreSpace, err := s.spaceCore.Create(ctx, spaceType, s.repKey, s.AccountMetadataPayload())
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	wait := make(chan struct{})
	s.waiting[coreSpace.Id()] = controllerWaiter{
		wait: wait,
	}
	s.mu.Unlock()
	ctrl, err := s.factory.CreateShareableSpace(ctx, coreSpace.Id(), description)
	if err != nil {
		s.mu.Lock()
		close(wait)
		s.waiting[coreSpace.Id()] = controllerWaiter{
			wait: wait,
			err:  err,
		}
		s.mu.Unlock()
		return nil, err
	}
	sp, err = ctrl.Current().(loader.LoadWaiter).WaitLoad(ctx)
	s.mu.Lock()
	close(wait)
	if err != nil {
		s.waiting[coreSpace.Id()] = controllerWaiter{
			wait: wait,
			err:  err,
		}
		s.mu.Unlock()
		return nil, err
	}
	s.spaceControllers[ctrl.SpaceId()] = ctrl
	s.mu.Unlock()
	s.updater.UpdateCoordinatorStatus()

	// todo: use CreateAndSetOneToOneSpace
	if bPriv, bPk, genErr := crypto.GenerateRandomEd25519KeyPair(); genErr == nil {
		_ = bPriv // not used; only need public key
		if spOne, derr := s.spaceCore.DeriveOneToOneSpace(ctx, bPk); derr == nil {
			_ = spOne.Storage().(anystorage.ClientSpaceStorage).MarkSpaceCreated(ctx)
			info := spaceinfo.NewSpacePersistentInfo(spOne.Id())
			info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
			_ = s.techSpace.SpaceViewCreate(ctx, spOne.Id(), true, info, nil)
		}
	}
	return
}
