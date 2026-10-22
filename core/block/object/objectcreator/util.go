package objectcreator

import (
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/globalsign/mgo/bson"
)

// injectApiObjectKey sets a value for ApiObjectKey relation in priority:
// - User-provided ApiObjectKey
// - Key from relationKey/uniqueKey
// - Name relation, transliterated
//
// The derived value is MINTED, not merely snake-cased. What is stored here is
// the spelling callers address the object by, and the api promises that
// spelling is snake_case — but snake-casing leaves every character it does not
// understand in place. Measured over a 38,123-object account, 27 of 1,530
// stored api keys sit outside the advertised grammar `^[a-zA-Z0-9_]+$`: the
// name `Lists [in work]` had minted `lists_[in_work]`, `Manual export &
// import` had minted `manual_export_&_import`, and `➡️ Medium` had minted
// `[?]_medium`, the emoji arriving as unidecode's literal `[?]`.
//
// Nothing is stored when the mint comes back empty — a name of only emoji, a
// key of only punctuation. An object with no derivable slug is addressed by
// its internal key, which is a real address; an empty apiObjectKey is not.
func injectApiObjectKey(object *domain.Details, key string) {
	if strings.TrimSpace(object.GetString(bundle.RelationKeyApiObjectKey)) != "" {
		return
	}
	var slug string
	if key == "" {
		slug = bundle.MintApiSlugFromName(object.GetString(bundle.RelationKeyName))
	} else {
		// no transliteration on this arm: the key already IS an internal key,
		// and the api derives a slug from a stored key with the same
		// transform, so transliterating one side would make the two disagree.
		slug = bundle.MintApiSlug(key)
	}
	if slug == "" {
		return
	}
	object.SetString(bundle.RelationKeyApiObjectKey, slug)
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
