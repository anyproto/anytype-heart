package space

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

// for initiator (e.g. from ui avatar, qrcode)
func (s *service) createOneToOneSendInbox(ctx context.Context, description *spaceinfo.SpaceDescription) (sp clientspace.Space, err error) {
	sp, err = s.createOneToOne(ctx, description)
	if err != nil {
		return
	}
	var bobAccountAddress string
	if description.OneToOneParticipantIdentity != "" {
		bobAccountAddress = description.OneToOneParticipantIdentity
	} else {
		// for testing
		bobAccountAddress = loadenv.Get("BOB_ACCOUNT")
	}
	idProfile := s.identityService.WaitProfile(ctx, bobAccountAddress)
	err = s.inboxClient.SendOneToOneInvite(ctx, idProfile)
	return
}

// for acceptor (e.g. inbox message)
func (s *service) createOneToOne(ctx context.Context, description *spaceinfo.SpaceDescription) (sp clientspace.Space, err error) {
	var bobAccountAddress string
	if description.OneToOneParticipantIdentity != "" {
		bobAccountAddress = description.OneToOneParticipantIdentity
	} else {
		// for testing
		bobAccountAddress = loadenv.Get("BOB_ACCOUNT")
	}

	fmt.Printf("-- bob: %s\n", bobAccountAddress)

	bPk, err := crypto.DecodeAccountAddress(bobAccountAddress)
	if err != nil {
		return
	}

	// TODO: add space type to space view to diffirentiate between onetoone/nononetoone
	coreSpace, err := s.spaceCore.CreateOneToOneSpace(ctx, bPk)
	if err != nil {
		return
	}
	s.mu.Lock()
	wait := make(chan struct{})
	s.waiting[coreSpace.Id()] = controllerWaiter{
		wait: wait,
	}
	s.mu.Unlock()

	bobProfile := s.identityService.WaitProfile(ctx, bobAccountAddress)
	bobKey, err := crypto.UnmarshallAESKeyProto(bobProfile.RequestMetadataKey)
	if err != nil {
		return
	}

	participantData := spaceinfo.OneToOneParticipantData{
		Identity:           bPk,
		RequestMetadataKey: bobKey,
	}
	ctrl, err := s.factory.CreateOneToOneSpace(ctx, coreSpace.Id(), participantData)
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

	return
}

func (s *service) create(ctx context.Context, description *spaceinfo.SpaceDescription) (sp clientspace.Space, err error) {
	var spaceType = spacedomain.SpaceTypeRegular
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
	return
}
