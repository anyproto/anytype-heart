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

func (s *service) createOneToOne(ctx context.Context, description *spaceinfo.SpaceDescription) (sp clientspace.Space, err error) {
	log.Warn("-- createOneToOne")

	// id1: AASZXTchV87HnZU1yujiM74GF2unsez4MRPSjKmgU1Vtmiid
	// id2: A5sAtJ6i4Z6465mtPff6m2xyN7SvmCWwBfcUCsR6zmoiNs1J

	bobAccountAddress := loadenv.Get("BOB_ACCOUNT")
	fmt.Printf("-- bob: %s\n", bobAccountAddress)

	bPk, err := crypto.DecodeAccountAddress(bobAccountAddress)
	if err != nil {
		return
	}

	fmt.Printf("-- ctrl: \n")
	ctrl, err := s.factory.CreateOneToOneSpace(ctx, bPk)
	if err != nil {
		return nil, err
	}

	fmt.Printf("-- ctrl wait load: \n")
	sp, err = ctrl.Current().(loader.LoadWaiter).WaitLoad(ctx)
	s.spaceControllers[ctrl.SpaceId()] = ctrl

	s.updater.UpdateCoordinatorStatus()

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
