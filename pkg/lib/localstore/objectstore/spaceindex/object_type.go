package spaceindex

import (
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *dsObjectStore) GetObjectType(id string) (*model.ObjectType, error) {
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return bundle.GetTypeByUrl(id)
	}

	details, err := s.GetDetails(id)
	if err != nil {
		return nil, err
	}

	if pbtypes.IsStructEmpty(details.GetDetails()) {
		return nil, ErrObjectNotFound
	}

	if pbtypes.GetBool(details.GetDetails(), bundle.RelationKeyIsDeleted.String()) {
		return nil, fmt.Errorf("type was removed")
	}

	rawUniqueKey := pbtypes.GetString(details.Details, bundle.RelationKeyUniqueKey.String())
	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(rawUniqueKey)
	if err != nil {
		return nil, fmt.Errorf("get type key from raw unique key: %w", err)
	}
	ot := s.extractObjectTypeFromDetails(details.Details, id, objectTypeKey)
	return ot, nil
}

func (s *dsObjectStore) extractObjectTypeFromDetails(details *types.Struct, url string, objectTypeKey domain.TypeKey) *model.ObjectType {
	return &model.ObjectType{
		Url:        url,
		Key:        string(objectTypeKey),
		Name:       pbtypes.GetString(details, bundle.RelationKeyName.String()),
		Layout:     model.ObjectTypeLayout(int(pbtypes.GetInt64(details, bundle.RelationKeyRecommendedLayout.String()))),
		IconEmoji:  pbtypes.GetString(details, bundle.RelationKeyIconEmoji.String()),
		IsArchived: pbtypes.GetBool(details, bundle.RelationKeyIsArchived.String()),
		// we use Page for all custom object types
		Types:         []model.SmartBlockType{model.SmartBlockType_Page},
		RelationLinks: s.getRelationLinksForRecommendedRelations(details),
	}
}

func (s *dsObjectStore) getRelationLinksForRecommendedRelations(details *types.Struct) []*model.RelationLink {
	recommendedRelationIDs := pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String())
	relationLinks := make([]*model.RelationLink, 0, len(recommendedRelationIDs))
	for _, relationID := range recommendedRelationIDs {
		relation, err := s.GetRelationById(relationID)
		if err != nil {
			log.Errorf("failed to get relation %s: %s", relationID, err)
		} else {
			relationModel := &relationutils.Relation{Relation: relation}
			relationLinks = append(relationLinks, relationModel.RelationLink())
		}
	}
	return relationLinks
}
