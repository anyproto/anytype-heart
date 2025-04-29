package util

import (
	"strings"

	"github.com/iancoleman/strcase"
)

// Internal 						-> API
// "rel-dueDate"             		-> "prop_due_date"
// "rel-67b0d3e3cda913b84c1299b1" 	-> "prop_67b0d3e3cda913b84c1299b1"
// "ot-page"                 		-> "type_page"
// "ot-67b0d3e3cda913b84c1299b1"   	-> "type_67b0d3e3cda913b84c1299b1"
// "opt-67b0d3e3cda913b84c1299b1"  	-> "tag_67b0d3e3cda913b84c1299b1"

const (
	propPrefix                   = ""
	typePrefix                   = ""
	tagPrefix                    = ""
	internalRelationPrefix       = "rel-"
	internalObjectTypePrefix     = "ot-"
	internalRelationOptionPrefix = "opt-"
)

func ToPropertyApiKey(internalKey string) string {
	return toApiKey(propPrefix, internalRelationPrefix, internalKey)
}

func FromPropertyApiKey(apiKey string) string {
	return fromApiKey(propPrefix, "", apiKey) // interally, we don't prefix relation keys
}

func ToTypeApiKey(internalKey string) string {
	return toApiKey(typePrefix, internalObjectTypePrefix, internalKey)
}

func FromTypeApiKey(apiKey string) string {
	return fromApiKey(typePrefix, internalObjectTypePrefix, apiKey)
}

func ToTagApiKey(internalKey string) string {
	return toApiKey(tagPrefix, internalRelationOptionPrefix, internalKey)
}

func FromTagApiKey(apiKey string) string {
	return fromApiKey(tagPrefix, internalRelationOptionPrefix, apiKey)
}

// IsCustomPropertyKey returns true if key is exactly 24 letters and contains at least a digit.
// Non-custom properties never contain a digit.
func IsCustomPropertyKey(key string) bool {
	if len(key) != 24 && !strings.ContainsAny(key, "0123456789") {
		return false
	}
	return true
}

// toApiKey converts an internal key into API format by stripping any existing internal prefixes and adding the API prefix.
func toApiKey(prefix, internalPrefix, internalKey string) string {
	var k string
	internalKey = strings.TrimPrefix(internalKey, internalPrefix)
	if IsCustomPropertyKey(internalKey) {
		k = internalKey
	} else {
		k = strcase.ToSnake(internalKey)
	}
	return prefix + k
}

// fromApiKey converts an API key back into internal format by stripping the API prefix and re-adding the internal prefix.
func fromApiKey(prefix, internalPrefix, apiKey string) string {
	k := strings.TrimPrefix(apiKey, prefix)
	if IsCustomPropertyKey(k) {
		return internalPrefix + k
	}
	return internalPrefix + strcase.ToLowerCamel(k)
}
