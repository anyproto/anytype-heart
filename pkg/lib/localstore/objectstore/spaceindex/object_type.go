package spaceindex

import (
	"fmt"
	"strings"

	"github.com/samber/lo"

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

	if details.GetBool(bundle.RelationKeyIsDeleted) {
		return nil, fmt.Errorf("type was removed")
	}

	rawUniqueKey := details.GetString(bundle.RelationKeyUniqueKey)
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
		Name:       details.GetString(bundle.RelationKeyName),
		Layout:     model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyRecommendedLayout)),
		IconEmoji:  details.GetString(bundle.RelationKeyIconEmoji),
		IsArchived: details.GetBool(bundle.RelationKeyIsArchived),
		// we use Page for all custom object types
		Types:         []model.SmartBlockType{model.SmartBlockType_Page},
		RelationLinks: s.getRelationLinksForRecommendedRelations(details),
	}
}

func (s *dsObjectStore) getRelationLinksForRecommendedRelations(details *domain.Details) []*model.RelationLink {
	recommendedRelationIds := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	featuredRelationIds := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	fileRelationIds := details.GetStringList(bundle.RelationKeyRecommendedFileRelations)
	hiddenRelationIds := details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations)
	allRelationIds := lo.Uniq(append(append(recommendedRelationIds, featuredRelationIds...), append(fileRelationIds, hiddenRelationIds...)...))
	relationLinks := make([]*model.RelationLink, 0, len(allRelationIds))
	for _, relationID := range allRelationIds {
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
