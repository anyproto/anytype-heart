package block

import (
	"context"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *Service) UpdateBundledObjects(ctx context.Context, spaceId string) error {
	if spaceId == "" {
		personal, err := s.spaceService.GetPersonalSpace(ctx)
		if err != nil {
			return err
		}
		spaceId = personal.Id()
	}

	marketRels, err := s.objectStore.ListAllRelations(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return err
	}

	personalRels, err := s.objectStore.ListAllRelations(spaceId)
	if err != nil {
		return err
	}

	for _, rel := range personalRels.Models() {
		marketRel := marketRels.GetModelByKey(rel.Key)
		if marketRel == nil || !lo.Contains(bundle.SystemRelations, domain.RelationKey(rel.Key)) {
			continue
		}
		details := buildDiffDetails(marketRel, rel)
		if len(details) != 0 {
			if err = DoState(s, rel.Id, func(st *state.State, sb basic.DetailsSettable) error {

				//TODO: we need to add analysis on whether relation was modified by user

				return sb.SetDetails(nil, details, false)
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

func buildDiffDetails(origin, custom *model.Relation) (details []*pb.RpcObjectSetDetailsDetail) {
	if origin.Description != custom.Description {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyDescription.String(),
			Value: pbtypes.String(origin.Description),
		})
	}

	if origin.Name != custom.Name {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(origin.Name),
		})
	}

	if origin.Hidden != custom.Hidden {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyIsHidden.String(),
			Value: pbtypes.Bool(origin.Hidden),
		})
	}

	return
}
