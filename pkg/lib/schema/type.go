package schema

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Type represents an object type in the schema
type Type struct {
	Key                  string                 `json:"key"`
	Name                 string                 `json:"name"`
	Description          string                 `json:"description,omitempty"`
	PluralName           string                 `json:"pluralName,omitempty"`
	IconEmoji            string                 `json:"iconEmoji,omitempty"`
	IconImage            string                 `json:"iconImage,omitempty"`
	IsArchived           bool                   `json:"isArchived,omitempty"`
	IsHidden             bool                   `json:"isHidden,omitempty"`
	Layout               model.ObjectTypeLayout `json:"layout,omitempty"`
	FeaturedRelations    []string               `json:"featuredRelations,omitempty"`
	RecommendedRelations []string               `json:"recommendedRelations,omitempty"`
	RestrictedRelations  []string               `json:"restrictedRelations,omitempty"`
	Extension            map[string]interface{} `json:"extension,omitempty"` // x-* fields from schema
	KeyToIdFunc          func(string) string    `json:"-"`                   // function to convert type key to ID, used for relations
}

func convertKeysToIds(from []string, keyToIdFunc func(string) string) []string {
	if keyToIdFunc == nil {
		return from
	}
	to := make([]string, len(from))
	for i, key := range from {
		to[i] = keyToIdFunc(key)
	}
	return to
}

// ToDetails converts Type to domain.Details
func (t *Type) ToDetails() *domain.Details {
	details := domain.NewDetails()

	details.SetString(bundle.RelationKeyName, t.Name)

	if t.Description != "" {
		details.SetString(bundle.RelationKeyDescription, t.Description)
	}

	if t.PluralName != "" {
		details.SetString(bundle.RelationKeyPluralName, t.PluralName)
	}

	if t.IconEmoji != "" {
		details.SetString(bundle.RelationKeyIconEmoji, t.IconEmoji)
	}

	if t.IconImage != "" {
		details.SetString(bundle.RelationKeyIconImage, t.IconImage)
	}

	details.SetBool(bundle.RelationKeyIsArchived, t.IsArchived)
	details.SetBool(bundle.RelationKeyIsHidden, t.IsHidden)

	if t.Layout != model.ObjectType_basic {
		details.SetInt64(bundle.RelationKeyRecommendedLayout, int64(t.Layout))
	}

	// Set featured and recommended relations
	if len(t.FeaturedRelations) > 0 {
		details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, convertKeysToIds(t.FeaturedRelations, t.KeyToIdFunc))
	}

	if len(t.RecommendedRelations) > 0 {
		details.SetStringList(bundle.RelationKeyRecommendedRelations, convertKeysToIds(t.RecommendedRelations, t.KeyToIdFunc))
	}

	// Restricted relations not supported yet
	// if len(t.RestrictedRelations) > 0 {
	//	details.SetStringList(bundle.RelationKeyRestrictedRelations, t.RestrictedRelations)
	// }

	// Set source to indicate it's from import
	details.SetInt64(bundle.RelationKeySourceObject, int64(model.ObjectType_objectType))

	// Generate unique key
	uniqueKey, _ := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, t.Key)
	details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())

	// Set layout for object type
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_objectType))

	// Set ID (will be generated if not provided)
	if id, ok := t.Extension["id"].(string); ok && id != "" {
		details.SetString(bundle.RelationKeyId, id)
	}

	// Set the type relation to itself (object types are objects too)
	details.SetString(bundle.RelationKeyType, bundle.TypeKeyObjectType.URL())

	return details
}

// FromDetails creates a Type from domain.Details
func TypeFromDetails(details *domain.Details) (*Type, error) {
	if details == nil {
		return nil, fmt.Errorf("details is nil")
	}

	t := &Type{
		Name:                 details.GetString(bundle.RelationKeyName),
		Description:          details.GetString(bundle.RelationKeyDescription),
		PluralName:           details.GetString(bundle.RelationKeyPluralName),
		IconEmoji:            details.GetString(bundle.RelationKeyIconEmoji),
		IconImage:            details.GetString(bundle.RelationKeyIconImage),
		IsArchived:           details.GetBool(bundle.RelationKeyIsArchived),
		IsHidden:             details.GetBool(bundle.RelationKeyIsHidden),
		Layout:               model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyRecommendedLayout)),
		FeaturedRelations:    details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations),
		RecommendedRelations: details.GetStringList(bundle.RelationKeyRecommendedRelations),
		// RestrictedRelations:  details.GetStringList(bundle.RelationKeyRestrictedRelations),
		Extension: make(map[string]interface{}),
	}

	// Extract type key from unique key if available
	if uniqueKey := details.GetString(bundle.RelationKeyUniqueKey); uniqueKey != "" {
		if typeKey, err := domain.GetTypeKeyFromRawUniqueKey(uniqueKey); err == nil {
			t.Key = string(typeKey)
		}
	}

	// Store ID in extension if present
	if id := details.GetString(bundle.RelationKeyId); id != "" {
		t.Extension["id"] = id
	}

	return t, nil
}

// Validate checks if the type is valid
func (t *Type) Validate() error {
	if t.Key == "" {
		return fmt.Errorf("type key is required")
	}
	if t.Name == "" {
		return fmt.Errorf("type name is required")
	}
	return nil
}

// AddRelation adds a relation to the type's recommended or featured relations
func (t *Type) AddRelation(relationId string, featured bool) {
	if featured {
		// Check if already exists
		for _, r := range t.FeaturedRelations {
			if r == relationId {
				return
			}
		}
		t.FeaturedRelations = append(t.FeaturedRelations, relationId)
	} else {
		// Check if already exists
		for _, r := range t.RecommendedRelations {
			if r == relationId {
				return
			}
		}
		t.RecommendedRelations = append(t.RecommendedRelations, relationId)
	}
}

// HasRelation checks if the type includes a specific relation
func (t *Type) HasRelation(relationId string) bool {
	for _, r := range t.FeaturedRelations {
		if r == relationId {
			return true
		}
	}
	for _, r := range t.RecommendedRelations {
		if r == relationId {
			return true
		}
	}
	return false
}

// IsBundled checks if this type key is a bundled type
func (t *Type) IsBundled() bool {
	_, err := bundle.GetType(domain.TypeKey(t.Key))
	return err == nil
}
