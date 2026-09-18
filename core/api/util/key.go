package util

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// Key transformation from Internal to API format:
//
// Properties (relations):
//   "dueDate"                     -> "due_date"
//   "67b0d3e3cda913b84c1299b1" -> "67b0d3e3cda913b84c1299b1"
//
// Types:
//   "ot-page"                     -> "page"
//   "ot-67b0d3e3cda913b84c1299b1" -> "67b0d3e3cda913b84c1299b1"
//
// Tags (relation options):
//   "opt-color"                   -> "color"
//   "opt-67b0d3e3cda913b84c1299b1" -> "67b0d3e3cda913b84c1299b1"

const (
	internalObjectTypePrefix     = "ot-"  // Object types
	internalRelationOptionPrefix = "opt-" // Relation options (tags)
	bsonIdLength                 = 24
)

var (
	// Matches valid BSON ObjectID: 24 hexadecimal characters with at least one digit
	bsonIdPattern = regexp.MustCompile(`^[a-f\d]{24}$`)
	digitPattern  = regexp.MustCompile(`\d`)
)

// ToPropertyApiKey converts an internal property/relation key to API format
// Examples: "dueDate" -> "due_date", "67b0d3e3cda913b84c1299b1" -> "67b0d3e3cda913b84c1299b1"
func ToPropertyApiKey(internalKey string) (apiKey string) {
	// Properties work with relation keys (rk) directly, not unique keys (uk)
	if IsBsonId(internalKey) {
		return internalKey
	}

	return strcase.ToSnake(internalKey)
}

// ToTypeApiKey converts an internal type key to API format
// Examples: "ot-page" -> "page", "ot-67b0d..." -> "67b0d..."
func ToTypeApiKey(internalKey string) (apiKey string) {
	key := strings.TrimPrefix(internalKey, internalObjectTypePrefix)

	if IsBsonId(key) {
		return key
	}

	return strcase.ToSnake(key)
}

// ToTagApiKey converts an internal tag/option key to API format
// Examples: "opt-color" -> "color", "opt-67b0d..." -> "67b0d..."
func ToTagApiKey(internalKey string) (apiKey string) {
	key := strings.TrimPrefix(internalKey, internalRelationOptionPrefix)

	if IsBsonId(key) {
		return key
	}

	return strcase.ToSnake(key)
}

// IsBsonId checks if a key is a valid BSON ObjectID
func IsBsonId(key string) bool {
	return len(key) == bsonIdLength && bsonIdPattern.MatchString(key) && digitPattern.MatchString(key)
}

// MintApiObjectKey mints the apiObjectKey to store for a key an API caller
// supplied. It is the same mint objectcreator applies to a derived key, so a
// property created through the api and one created from a display name land
// in the same grammar and are addressed the same way.
//
// CONVERSION, not refusal, for anything that survives the grammar. Three
// reasons, in order of weight:
//
//   - Conversion is the advertised contract. The key field on every create
//     and update request documents that a key "should always be snake_case,
//     otherwise it will be converted to snake_case". Refusing would break
//     that for input that works today: a caller sending `Due Date` and
//     receiving `due_date` is not a caller in error.
//   - The caller is told. Create and update answer with the object, and its
//     key field carries the minted key, so a caller never has to guess the
//     spelling it must address. That is what refusal would have bought.
//   - Today's alternative is worse than either. Snake-casing alone stores the
//     caller's punctuation verbatim, so `Lists [in work]` becomes the stored
//     key `lists_[in_work]` — accepted, and then not the snake_case spelling
//     the same endpoint promised to store. 27 of 1,530 keys in a measured
//     38,123-object account sit on that path, and nobody was ever told.
//
// ok is false only when NOTHING of the key survives the grammar (`"➡️"`,
// `"!!!"`). That is not a conversion — there is no spelling to convert to —
// and storing no key at all would silently drop a key the caller explicitly
// asked for, so it is the one case the caller must hear about.
func MintApiObjectKey(key string) (apiKey string, ok bool) {
	apiKey = bundle.MintApiSlug(key)
	return apiKey, apiKey != ""
}

// ErrInvalidApiObjectKey is the bad-input error for a key MintApiObjectKey
// could mint nothing from. kind names the object ("property", "type", "tag")
// so the message reads like the key errors beside it.
func ErrInvalidApiObjectKey(kind string, key string) error {
	return ErrBadInput(fmt.Sprintf("%s key %q holds no character a key may contain; keys are made of letters, digits and underscores", kind, key))
}
