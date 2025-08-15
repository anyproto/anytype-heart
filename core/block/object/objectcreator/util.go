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

// injectAndEnsureUniqueApiObjectKey sets a value for ApiObjectKey relation in priority:
// - User-provided ApiObjectKey
// - Key from relationKey/uniqueKey
// - Transliterated Name relation
// Then ensures the key is unique by adding sequential suffixes if needed
func (s *service) injectAndEnsureUniqueApiObjectKey(spaceId string, object *domain.Details, key string, objectType coresb.SmartBlockType) error {
	if strings.TrimSpace(object.GetString(bundle.RelationKeyApiObjectKey)) == "" {
		if key == "" {
			key = transliterate(object.GetString(bundle.RelationKeyName))
		}
		key = strcase.ToSnake(key)
		object.SetString(bundle.RelationKeyApiObjectKey, key)
	}

	return s.ensureUniqueApiObjectKey(spaceId, object, objectType)
}

// ensureUniqueApiObjectKey checks if the ApiObjectKey already exists and generates a unique one with sequential suffix if needed
func (s *service) ensureUniqueApiObjectKey(spaceId string, object *domain.Details, objectType coresb.SmartBlockType) error {
	apiKey := object.GetString(bundle.RelationKeyApiObjectKey)
	if apiKey == "" {
		return nil
	}

	var baseFilters []database.FilterRequest
	switch objectType {
	case coresb.SmartBlockTypeObjectType:
		baseFilters = []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_objectType)),
			},
		}
	case coresb.SmartBlockTypeRelation:
		baseFilters = []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
		}
	case coresb.SmartBlockTypeRelationOption:
		baseFilters = []database.FilterRequest{
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
	const maxIterations = 100

	records, err := s.objectStore.SpaceIndex(spaceId).Query(database.Query{
		Filters: baseFilters,
		Limit:   0,
	})
	if err != nil {
		return fmt.Errorf("query existing apiObjectKey: %w", err)
	}

	existingKeys := make(map[string]struct{})
	for _, rec := range records {
		if key := rec.Details.GetString(bundle.RelationKeyApiObjectKey); key != "" {
			// Only add keys that could conflict (baseKey or baseKey with numeric suffix)
			if key == baseKey || (len(key) > len(baseKey) && key[:len(baseKey)] == baseKey) {
				existingKeys[key] = struct{}{}
			}
		}
	}

	// Try baseKey first, then baseKey1, baseKey2, ..., up to maxIterations
	for i := 0; i <= maxIterations; i++ {
		var candidate string
		if i == 0 {
			candidate = baseKey
		} else {
			candidate = fmt.Sprintf("%s%d", baseKey, i)
		}

		if _, exists := existingKeys[candidate]; !exists {
			object.SetString(bundle.RelationKeyApiObjectKey, candidate)
			return nil
		}
	}

	return fmt.Errorf("failed to find unique apiObjectKey after %d attempts for key: %s", maxIterations, baseKey)
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
