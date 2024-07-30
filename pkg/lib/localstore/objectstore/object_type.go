package objectstore

import (
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *dsObjectStore) GetObjectType(id string) (*model.ObjectType, error) {
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return bundle.GetTypeByUrl(id)
	}

	details, err := s.GetDetails(id)
	if err != nil {
		return nil, err
	}

	if details.Len() == 0 {
		return nil, ErrObjectNotFound
	}

	if details.GetBoolOrDefault(bundle.RelationKeyIsDeleted, false) {
		return nil, fmt.Errorf("type was removed")
	}

	rawUniqueKey := details.GetStringOrDefault(bundle.RelationKeyUniqueKey, "")
	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(rawUniqueKey)
	if err != nil {
		return nil, fmt.Errorf("get type key from raw unique key: %w", err)
	}
	ot := s.extractObjectTypeFromDetails(details, id, objectTypeKey)
	return ot, nil
}

func (s *dsObjectStore) extractObjectTypeFromDetails(details *domain.Details, url string, objectTypeKey domain.TypeKey) *model.ObjectType {
	return &model.ObjectType{
		Url:        url,
		Key:        string(objectTypeKey),
		Name:       details.GetStringOrDefault(bundle.RelationKeyName, ""),
		Layout:     model.ObjectTypeLayout(details.GetInt64OrDefault(bundle.RelationKeyRecommendedLayout, 0)),
		IconEmoji:  details.GetStringOrDefault(bundle.RelationKeyIconEmoji, ""),
		IsArchived: details.GetBoolOrDefault(bundle.RelationKeyIsArchived, false),
		// we use Page for all custom object types
		Types:         []model.SmartBlockType{model.SmartBlockType_Page},
		RelationLinks: s.getRelationLinksForRecommendedRelations(details),
	}
}

func (s *dsObjectStore) getRelationLinksForRecommendedRelations(details *domain.Details) []*model.RelationLink {
	recommendedRelationIDs := details.GetStringListOrDefault(bundle.RelationKeyRecommendedRelations, nil)
	relationLinks := make([]*model.RelationLink, 0, len(recommendedRelationIDs))
	for _, relationID := range recommendedRelationIDs {
		relation, err := s.GetRelationByID(relationID)
		if err != nil {
			log.Errorf("failed to get relation %s: %s", relationID, err)
		} else {
			relationModel := &relationutils.Relation{Relation: relation}
			relationLinks = append(relationLinks, relationModel.RelationLink())
		}
	}
	return relationLinks
}
