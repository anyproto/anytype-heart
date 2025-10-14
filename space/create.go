package space

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func (s *service) createOneToOne(ctx context.Context, description *spaceinfo.SpaceDescription) (sp clientspace.Space, err error) {
	log.Warn("-- createOneToOne")

	bobAccountAddress := loadenv.Get("BOB_ACCOUNT")
	fmt.Printf("-- bob: %s\n", bobAccountAddress)

	bPk, err := crypto.DecodeAccountAddress(bobAccountAddress)
	if err != nil {
		return
	}

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

	ctrl, err := s.factory.CreateOneToOneSpace(ctx, coreSpace.Id())
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

	// inbox
	msg := &coordinatorproto.InboxMessage{
		PacketType: coordinatorproto.InboxPacketType_Default,
		Packet: &coordinatorproto.InboxPacket{
			KeyType:          coordinatorproto.InboxKeyType_ed25519,
			ReceiverIdentity: bobAccountAddress,
			Payload: &coordinatorproto.InboxPayload{
				PayloadType: coordinatorproto.InboxPayloadType_InboxPayloadSpaceInvite,
			},
		},
	}

	// - InboxAddMessage for bob
	log.Info("--inbox: add message")
	s.inboxClient.InboxAddMessage(ctx, bPk, msg)

	fmt.Printf("-- ctrl wait load ret: \n")
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
