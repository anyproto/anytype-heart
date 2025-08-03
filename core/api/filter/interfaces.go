package filter

import (
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
)

// ApiService defines the interface for property-related operations
type ApiService interface {
	GetCachedProperties(spaceId string) map[string]*apimodel.Property
	ResolvePropertyApiKey(properties map[string]*apimodel.Property, key string) string
	SanitizeAndValidatePropertyValue(spaceId string, key string, format apimodel.PropertyFormat, value interface{}, property *apimodel.Property, propertyMap map[string]*apimodel.Property) (interface{}, error)
}
