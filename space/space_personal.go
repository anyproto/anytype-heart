package space

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/techspace"
)

func (s *service) initPersonalSpace() (err error) {
	s.personalSpaceID, err = s.spaceCore.DeriveID(s.ctx, spacecore.SpaceType)
	if err != nil {
		return
	}

	// TODO: move this logic to any-sync
	s.repKey, err = getRepKey(s.personalSpaceID)
	if err != nil {
		return
	}

	if s.newAccount {
		return s.createPersonalSpace(s.ctx)
	}
	return s.loadPersonalSpace(s.ctx)
}

func (s *service) createPersonalSpace(ctx context.Context) (err error) {
	coreSpace, err := s.spaceCore.Derive(ctx, spacecore.SpaceType)
	if err != nil {
		return
	}
	_, err = s.create(ctx, coreSpace)
	if err == nil {
		return
	}
	if errors.Is(err, techspace.ErrSpaceViewExists) {
		return s.loadPersonalSpace(ctx)
	}
	return
}

func (s *service) loadPersonalSpace(ctx context.Context) (err error) {
	// Check that space exists. If not, probably user is migrating from legacy version
	_, err = s.spaceCore.Get(ctx, s.personalSpaceID)
	if err != nil {
		return err
	}

	err = s.startLoad(ctx, s.personalSpaceID)
	// This could happen for old accounts
	if errors.Is(err, ErrSpaceNotExists) {
		err = s.techSpace.SpaceViewCreate(ctx, s.personalSpaceID)
		if err != nil {
			return err
		}
		err = s.startLoad(ctx, s.personalSpaceID)
		if err != nil {
			return err
		}
	}
	if err != nil {
		return
	}

	_, err = s.waitLoad(ctx, s.personalSpaceID)
	return err
}
