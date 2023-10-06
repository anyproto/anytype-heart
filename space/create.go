package space

import (
	"context"
	"fmt"

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
		return nil, fmt.Errorf("DeriveObjectIDs error: %v; spaceId: %v", err, coreSpace.Id())
	}
	err = s.provider.CreateMandatoryObjects(ctx, coreSpace.Id(), sbTypes)
	if err != nil {
		return nil, fmt.Errorf("CreateMandatoryObjects error: %v; spaceId: %v", err, coreSpace.Id())
	}

	if err = s.techSpace.SpaceViewCreate(ctx, coreSpace.Id()); err != nil {
		return nil, err
	}

	// load
	if err = s.startLoad(ctx, coreSpace.Id()); err != nil {
		return nil, err
	}
	return s.waitLoad(ctx, coreSpace.Id())
}
