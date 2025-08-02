package objectcreator

import (
	"fmt"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gosimple/unidecode"
	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// injectApiObjectKey sets a value for ApiObjectKey relation in priority:
// - User-provided ApiObjectKey
// - Key from relationKey/uniqueKey
// - Transliterated Name relation
func injectApiObjectKey(object *domain.Details, key string) {
	if strings.TrimSpace(object.GetString(bundle.RelationKeyApiObjectKey)) == "" {
		if key == "" {
			key = transliterate(object.GetString(bundle.RelationKeyName))
		}
		key = strcase.ToSnake(key)
		object.SetString(bundle.RelationKeyApiObjectKey, key)
	}
}

// ensureUniqueApiObjectKey checks if the ApiObjectKey already exists and generates a unique one with sequential suffix if needed
func (s *service) ensureUniqueApiObjectKey(spaceId string, object *domain.Details, objectType coresb.SmartBlockType) error {
	apiKey := object.GetString(bundle.RelationKeyApiObjectKey)
	if apiKey == "" {
		return nil
	}

	var filters []database.FilterRequest
	switch objectType {
	case coresb.SmartBlockTypeObjectType:
		filters = []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_objectType)),
			},
		}
	case coresb.SmartBlockTypeRelation:
		filters = []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
		}
	case coresb.SmartBlockTypeRelationOption:
		filters = []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relationOption)),
			},
		}
	default:
		return nil
	}

	baseKey := apiKey
	suffix := 1

	for {
		// Check if this key already exists
		filters := append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyApiObjectKey,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(apiKey),
		})

		records, err := s.objectStore.SpaceIndex(spaceId).Query(database.Query{
			Filters: filters,
			Limit:   1,
		})
		if err != nil {
			return fmt.Errorf("query existing apiObjectKey: %w", err)
		}

		// If no existing object with this key, we're good
		if len(records) == 0 {
			object.SetString(bundle.RelationKeyApiObjectKey, apiKey)
			return nil
		}

		// Key exists, try with suffix
		apiKey = fmt.Sprintf("%s%d", baseKey, suffix)
		suffix++

		filters = filters[:len(filters)-1]
	}
}

func transliterate(in string) string {
	return unidecode.Unidecode(strings.TrimSpace(in))
}

func getUniqueKeyOrGenerate(sbType coresb.SmartBlockType, details *domain.Details) (uk domain.UniqueKey, wasGenerated bool, err error) {
	uniqueKey := details.GetString(bundle.RelationKeyUniqueKey)
	if uniqueKey == "" {
		newUniqueKey, err := domain.NewUniqueKey(sbType, bson.NewObjectId().Hex())
		if err != nil {
			return nil, false, err
		}
		details.SetString(bundle.RelationKeyUniqueKey, newUniqueKey.Marshal())
		return newUniqueKey, true, err
	}
	uk, err = domain.UnmarshalUniqueKey(uniqueKey)
	return uk, false, err
}
