package markdown

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
)

// YAMLPropertyResolver implements a simple resolver that maintains property name to key mappings
// This is used when importing without schemas to ensure consistent property keys across files
type YAMLPropertyResolver struct {
	nameToKey map[string]string // property name -> relation key
}

// NewYAMLPropertyResolver creates a new resolver for YAML properties
func NewYAMLPropertyResolver() *YAMLPropertyResolver {
	return &YAMLPropertyResolver{
		nameToKey: make(map[string]string),
	}
}

// ResolvePropertyKey returns the key for a property name, creating one if it doesn't exist
func (r *YAMLPropertyResolver) ResolvePropertyKey(name string) string {
	if key, exists := r.nameToKey[name]; exists {
		return key
	}

	// Generate a new key for this property name
	key := bson.NewObjectId().Hex()
	r.nameToKey[name] = key
	return key
}

// GetRelationFormat returns the format for a relation - not used for non-schema imports
func (r *YAMLPropertyResolver) GetRelationFormat(key string) model.RelationFormat {
	// Return longtext as default - the actual format will be determined by the YAML value type
	return model.RelationFormat_longtext
}

// GetRelationOptions returns empty map - not used for non-schema imports
func (r *YAMLPropertyResolver) GetRelationOptions(key string) map[string]string {
	return nil
}

// ResolveOptionValue returns the value as-is - not used for non-schema imports
func (r *YAMLPropertyResolver) ResolveOptionValue(relationKey string, optionValue string) string {
	return optionValue
}

// ResolveOptionValues returns the values as-is - not used for non-schema imports
func (r *YAMLPropertyResolver) ResolveOptionValues(relationKey string, optionValues []string) []string {
	return optionValues
}

// GetKeys returns all the keys that have been created
func (r *YAMLPropertyResolver) GetKeys() map[string]string {
	return r.nameToKey
}
