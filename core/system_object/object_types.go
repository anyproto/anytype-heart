package system_object

import (
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) extractObjectTypeFromDetails(details *types.Struct, url string, objectTypeKey bundle.TypeKey) *model.ObjectType {
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

func (s *service) getRelationLinksForRecommendedRelations(details *types.Struct) []*model.RelationLink {
	recommendedRelationIDs := pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String())
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

func (s *service) GetObjectTypes(ids []string) (ots []*model.ObjectType, err error) {
	ots = make([]*model.ObjectType, 0, len(ids))
	for _, id := range ids {
		ot, e := s.GetObjectType(id)
		if e != nil {
			err = e
		} else {
			ots = append(ots, ot)
		}
	}
	return
}

func (s *service) GetObjectType(id string) (*model.ObjectType, error) {
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return bundle.GetTypeByUrl(id)
	}

	details, err := s.objectStore.GetDetails(id)
	if err != nil {
		return nil, err
	}

	if pbtypes.IsStructEmpty(details.GetDetails()) {
		return nil, objectstore.ErrObjectNotFound
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

func (s *service) HasObjectType(id string) (bool, error) {
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return bundle.HasObjectTypeID(id), nil
	}

	details, err := s.objectStore.GetDetails(id)
	if err != nil {
		return false, err
	}

	if pbtypes.IsStructEmpty(details.GetDetails()) {
		return false, nil
	}
	if pbtypes.GetBool(details.GetDetails(), bundle.RelationKeyIsDeleted.String()) {
		return false, nil
	}
	if pbtypes.GetString(details.Details, bundle.RelationKeyType.String()) != bundle.TypeKeyObjectType.URL() {
		return false, nil
	}
	return true, nil
}
