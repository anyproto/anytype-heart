package block

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var ErrBundledTypeIsReadonly = fmt.Errorf("can't modify bundled object type")

func (s *Service) ObjectTypeRelationAdd(ctx context.Context, req *pb.RpcObjectTypeRelationAddRequest) error {
	if strings.HasPrefix(req.ObjectTypeUrl, bundle.TypePrefix) {
		return ErrBundledTypeIsReadonly
	}
	spaceId, err := s.resolver.ResolveSpaceID(req.ObjectTypeUrl)
	if err != nil {
		return err
	}
	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	err = s.ModifyDetails(req.ObjectTypeUrl, func(current *types.Struct) (*types.Struct, error) {
		list := pbtypes.GetStringList(current, bundle.RelationKeyRecommendedRelations.String())
		for _, relKey := range req.RelationKeys {
			relId, err := space.GetRelationIdByKey(ctx, domain.RelationKey(relKey))
			if err != nil {
				return nil, err
			}

			if slice.FindPos(list, relId) == -1 {
				list = append(list, relId)
			}
		}
		detCopy := pbtypes.CopyStruct(current)
		detCopy.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(list)
		return detCopy, nil
	})
	return err
}
