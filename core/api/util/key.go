package util

import (
	"regexp"
	"strings"

	"github.com/iancoleman/strcase"
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
