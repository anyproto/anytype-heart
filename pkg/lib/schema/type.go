package schema

import (
	"fmt"
	"slices"

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
	PluralName           string                 `json:"plural_name,omitempty"`
	IconEmoji            string                 `json:"icon_emoji,omitempty"`
	IconName             string                 `json:"icon_name,omitempty"`
	IsArchived           bool                   `json:"is_archived,omitempty"`
	IsHidden             bool                   `json:"is_hidden,omitempty"`
	Layout               model.ObjectTypeLayout `json:"layout,omitempty"`
	FeaturedRelations    []string               `json:"featured_relations,omitempty"`    // store keys, not ids
	RecommendedRelations []string               `json:"recommended_relations,omitempty"` // store keys, not ids
	HiddenRelations      []string               `json:"hidden_relations,omitempty"`      // store keys, not ids
	Extension            map[string]interface{} `json:"extension,omitempty"`             // x-* fields from schema
	KeyToIdFunc          func(string) string    `json:"-"`                               // function to convert type key to ID, used for relations
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

func cleanBundledHidden(keys []string) []string {
	return slices.DeleteFunc(keys, func(r string) bool {
		b, _ := bundle.GetRelation(domain.RelationKey(r))
		return b != nil && b.Hidden
	})
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

	if t.IconName != "" {
		details.SetString(bundle.RelationKeyIconName, t.IconName)
	}

	details.SetBool(bundle.RelationKeyIsArchived, t.IsArchived)
	details.SetBool(bundle.RelationKeyIsHidden, t.IsHidden)
	details.SetInt64(bundle.RelationKeyRecommendedLayout, int64(t.Layout))

	t.FeaturedRelations = cleanBundledHidden(t.FeaturedRelations)
	t.RecommendedRelations = cleanBundledHidden(t.RecommendedRelations)
	t.HiddenRelations = cleanBundledHidden(t.HiddenRelations)

	// Set featured and recommended relations
	if len(t.FeaturedRelations) > 0 {
		details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, convertKeysToIds(t.FeaturedRelations, t.KeyToIdFunc))
	}

	if len(t.RecommendedRelations) > 0 {
		details.SetStringList(bundle.RelationKeyRecommendedRelations, convertKeysToIds(t.RecommendedRelations, t.KeyToIdFunc))
	}

	if len(t.HiddenRelations) > 0 {
		details.SetStringList(bundle.RelationKeyRecommendedHiddenRelations, convertKeysToIds(t.HiddenRelations, t.KeyToIdFunc))
	}

	// Set source to indicate it's from import
	details.SetInt64(bundle.RelationKeySourceObject, int64(model.ObjectType_objectType))

	// Generate unique key
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, t.Key)
	if err == nil {
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	}

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
	return TypeFromDetailsWithResolver(details, nil)
}

// TypeFromDetailsWithResolver creates a Type from domain.Details with optional ID to key resolver
func TypeFromDetailsWithResolver(details *domain.Details, idToKeyResolver func(relationId string) (string, error)) (*Type, error) {
	if details == nil {
		return nil, fmt.Errorf("details is nil")
	}

	// Helper function to convert relation IDs to keys
	convertIdsToKeys := func(relationIds []string) []string {
		if idToKeyResolver == nil {
			// If no resolver, assume they're already keys (for backward compatibility)
			return relationIds
		}

		keys := make([]string, 0, len(relationIds))
		for _, id := range relationIds {
			// Try to resolve ID to key
			if key, err := idToKeyResolver(id); err == nil && key != "" {
				keys = append(keys, key)
			} else {
				// If resolution fails, check if it's already a key (for bundled relations)
				if _, err := bundle.GetRelation(domain.RelationKey(id)); err == nil {
					keys = append(keys, id)
				}
				// Otherwise skip this relation
			}
		}
		return keys
	}

	t := &Type{
		Name:                 details.GetString(bundle.RelationKeyName),
		Description:          details.GetString(bundle.RelationKeyDescription),
		PluralName:           details.GetString(bundle.RelationKeyPluralName),
		IconEmoji:            details.GetString(bundle.RelationKeyIconEmoji),
		IconName:             details.GetString(bundle.RelationKeyIconName),
		IsArchived:           details.GetBool(bundle.RelationKeyIsArchived),
		IsHidden:             details.GetBool(bundle.RelationKeyIsHidden),
		Layout:               model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyRecommendedLayout)),
		FeaturedRelations:    convertIdsToKeys(details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)),
		RecommendedRelations: convertIdsToKeys(details.GetStringList(bundle.RelationKeyRecommendedRelations)),
		HiddenRelations:      convertIdsToKeys(details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations)),
		Extension:            make(map[string]interface{}),
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
func (t *Type) AddRelation(relationKey string, featured, hidden bool) {
	if featured {
		// Check if already exists
		for _, r := range t.FeaturedRelations {
			if r == relationKey {
				return
			}
		}
		t.FeaturedRelations = append(t.FeaturedRelations, relationKey)
	} else if hidden {
		for _, r := range t.HiddenRelations {
			if r == relationKey {
				return
			}
		}
		t.HiddenRelations = append(t.HiddenRelations, relationKey)
	} else {
		for _, r := range t.RecommendedRelations {
			if r == relationKey {
				return
			}
		}
		t.RecommendedRelations = append(t.RecommendedRelations, relationKey)
	}
}

// HasRelation checks if the type includes a specific relation
func (t *Type) HasRelation(relationKey string) bool {
	for _, r := range t.FeaturedRelations {
		if r == relationKey {
			return true
		}
	}
	for _, r := range t.RecommendedRelations {
		if r == relationKey {
			return true
		}
	}
	for _, r := range t.HiddenRelations {
		if r == relationKey {
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
