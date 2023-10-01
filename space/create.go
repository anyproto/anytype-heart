package space

import (
	"context"

	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

func (s *service) create(ctx context.Context, coreSpace *spacecore.AnySpace) (Space, error) {
	var sbTypes []coresb.SmartBlockType
	if s.IsPersonal(coreSpace.Id()) {
		sbTypes = threads.PersonalSpaceTypes
	} else {
		sbTypes = threads.SpaceTypes
	}

	// create mandatory objects
	_, err := s.provider.DeriveObjectIDs(ctx, coreSpace.Id(), sbTypes)
	if err != nil {
		return nil, err
	}
	err = s.provider.CreateMandatoryObjects(ctx, coreSpace.Id(), sbTypes)
	if err != nil {
		return nil, err
	}

	// create space view
	if _, err = s.techSpace.CreateSpaceView(ctx, coreSpace.Id()); err != nil {
		return nil, err
	}

	// load
	if err = s.startLoad(ctx, coreSpace.Id()); err != nil {
		return nil, err
	}
	return s.waitLoad(ctx, coreSpace.Id())
}
