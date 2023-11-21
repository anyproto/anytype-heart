package objectcreator

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) createFile(ctx context.Context, space space.Space, details *types.Struct) (id string, object *types.Struct, err error) {
	hash := pbtypes.GetString(details, bundle.RelationKeyFileHash.String())
	if hash == "" {
		return "", nil, fmt.Errorf("file hash is empty")
	}
	detailsFromMetadata, typeKey, err := s.getDetailsForFileOrImage(ctx, domain.FullID{
		SpaceID:  space.Id(),
		ObjectID: hash,
	})
	details = pbtypes.StructMerge(details, detailsFromMetadata, false)
	createState := state.NewDoc("", nil).(*state.State)
	createState.SetDetails(details)

	return s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{typeKey}, createState)
}

func (s *service) getDetailsForFileOrImage(ctx context.Context, id domain.FullID) (*types.Struct, domain.TypeKey, error) {
	file, err := s.fileService.FileByHash(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if mill.IsImage(file.Info().Media) {
		image, err := s.fileService.ImageByHash(ctx, id)
		if err != nil {
			return nil, "", err
		}
		details, err := image.Details(ctx)
		if err != nil {
			return nil, "", err
		}
		return details, bundle.TypeKeyImage, nil
	}

	d, typeKey, err := file.Details(ctx)
	if err != nil {
		return nil, "", err
	}
	return d, typeKey, nil
}
