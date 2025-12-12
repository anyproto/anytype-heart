package space

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

// for initiator (e.g. from ui avatar, qrcode)
func (s *service) CreateOneToOneSendInbox(ctx context.Context, description *spaceinfo.SpaceDescription) (sp clientspace.Space, err error) {
	if description.OneToOneIdentity == "" {
		return nil, fmt.Errorf("create onetoone: details, OneToOneIdentity is missing")
	}

	bobProfile, err := s.identityService.WaitProfileWithKey(ctx, description.OneToOneIdentity)
	if err != nil {
		return
	}

	description.Name = bobProfile.IdentityProfile.Name
	description.IconImage = bobProfile.IdentityProfile.IconCid
	description.OneToOneInboxSentStatus = spaceinfo.OneToOneInboxSentStatusToSend
	sp, err = s.CreateOneToOne(ctx, description, bobProfile)
	if err != nil {
		err = fmt.Errorf("create onetoone: %w", err)
		return
	}

	err = s.onetoone.ResendFailedOneToOneInvites(ctx)
	if err != nil {
		log.Error("failed to reschedule onetoone inbox resend", zap.Error(err))
	}

	return sp, nil
}

func (s *service) CreateOneToOne(ctx context.Context, description *spaceinfo.SpaceDescription, bobProfile *model.IdentityProfileWithKey) (sp clientspace.Space, err error) {
	myIdentity := s.accountService.Account().SignKey.GetPublic().Account()
	if description.OneToOneIdentity == myIdentity {
		return nil, fmt.Errorf("can't create OneToOne chat with self")
	}

	bPk, err := crypto.DecodeAccountAddress(bobProfile.IdentityProfile.Identity)
	if err != nil {
		return
	}

	coreSpace, err := s.spaceCore.CreateOneToOneSpace(ctx, bPk)
	if err != nil {
		err = fmt.Errorf("spacecore: create onetoone: %w", err)
		return
	}
	s.mu.Lock()
	wait := make(chan struct{})
	s.waiting[coreSpace.Id()] = controllerWaiter{
		wait: wait,
	}
	s.mu.Unlock()

	participantData := spaceinfo.OneToOneParticipantData{
		Identity:           bobProfile.IdentityProfile.Identity,
		RequestMetadataKey: bobProfile.RequestMetadata,
	}
	ctrl, err := s.factory.CreateOneToOneSpace(ctx, coreSpace.Id(), description, participantData)
	if err != nil {
		s.mu.Lock()
		close(wait)
		s.waiting[coreSpace.Id()] = controllerWaiter{
			wait: wait,
			err:  err,
		}
		s.mu.Unlock()
		err = fmt.Errorf("factory: create onetoone: %w", err)
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
		err = fmt.Errorf("loader: create onetoone: %w", err)
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
