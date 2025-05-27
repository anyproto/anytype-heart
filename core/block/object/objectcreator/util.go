package objectcreator

import (
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/globalsign/mgo/bson"
	"github.com/gosimple/unidecode"
	"github.com/iancoleman/strcase"
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
