package util

import (
	"regexp"
	"strings"

	"github.com/iancoleman/strcase"
)

// Internal 						-> API
// "dueDate"             		    -> "due_date"
// "67b0d3e3cda913b84c1299b1" 	    -> "67b0d3e3cda913b84c1299b1"
// "ot-page"                 		-> "page"
// "ot-67b0d3e3cda913b84c1299b1"   	-> "67b0d3e3cda913b84c1299b1"
// "opt-67b0d3e3cda913b84c1299b1"  	-> "67b0d3e3cda913b84c1299b1"

const (
	propPrefix                   = ""
	typePrefix                   = ""
	tagPrefix                    = ""
	internalRelationPrefix       = "" // interally, we're using rk instead of uk when working with relations from api, where no "rel-" prefix exists
	internalObjectTypePrefix     = "ot-"
	internalRelationOptionPrefix = "opt-"
)

var (
	hex24Pattern = regexp.MustCompile(`^[a-f\d]{24}$`)
	digitPattern = regexp.MustCompile(`\d`)
)

func ToPropertyApiKey(internalKey string) string {
	return toApiKey(propPrefix, internalRelationPrefix, internalKey)
}

// func FromPropertyApiKey(apiKey string) string {
// 	return fromApiKey(propPrefix, internalRelationPrefix, apiKey)
// }

func ToTypeApiKey(internalKey string) string {
	return toApiKey(typePrefix, internalObjectTypePrefix, internalKey)
}

// func FromTypeApiKey(apiKey string) string {
// 	return fromApiKey(typePrefix, internalObjectTypePrefix, apiKey)
// }

func ToTagApiKey(internalKey string) string {
	return toApiKey(tagPrefix, internalRelationOptionPrefix, internalKey)
}

// func FromTagApiKey(apiKey string) string {
// 	return fromApiKey(tagPrefix, internalRelationOptionPrefix, apiKey)
// }

// IsCustomKey returns true if key is exactly 24 letters and contains at least a digit.
// Non-custom properties never contain a digit.
func IsCustomKey(key string) bool {
	return len(key) == 24 && hex24Pattern.MatchString(key) && digitPattern.MatchString(key)
}

// toApiKey converts an internal key into API format by stripping any existing internal prefixes and adding the API prefix.
func toApiKey(prefix, internalPrefix, internalKey string) string {
	var k string
	internalKey = strings.TrimPrefix(internalKey, internalPrefix)
	if IsCustomKey(internalKey) {
		k = internalKey
	} else {
		k = strcase.ToSnake(internalKey)
	}
	return prefix + k
}

// fromApiKey converts an API key back into internal format by stripping the API prefix and re-adding the internal prefix.
func fromApiKey(prefix, internalPrefix, apiKey string) string {
	k := strings.TrimPrefix(apiKey, prefix)
	if IsCustomKey(k) {
		return internalPrefix + k
	}
	return internalPrefix + strcase.ToLowerCamel(k)
}
