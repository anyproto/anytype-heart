package schema

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
)

// CollectionPropertyKey is a workaround for storing collection ids as a fake property
const CollectionPropertyKey = "_collection"

// Relation represents a property/relation in the schema
type Relation struct {
	Key         string                 `json:"key"`
	Name        string                 `json:"name"`
	Format      model.RelationFormat   `json:"format"`
	Description string                 `json:"description,omitempty"`
	IsHidden    bool                   `json:"is_hidden,omitempty"`
	IsReadOnly  bool                   `json:"is_read_only,omitempty"`
	IsMulti     bool                   `json:"is_multi,omitempty"`
	ObjectTypes []string               `json:"object_types,omitempty"` // For object relations
	Options     []string               `json:"options,omitempty"`      // For status relations
	Examples    []string               `json:"examples,omitempty"`     // For tag relations
	IncludeTime bool                   `json:"include_time,omitempty"` // For date relations
	MaxLength   int                    `json:"max_length,omitempty"`   // For text relations
	Extension   map[string]interface{} `json:"extension,omitempty"`    // x-* fields from schema
}

// ToDetails converts Relation to domain.Details
func (r *Relation) ToDetails() *domain.Details {
	details := domain.NewDetails()
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relation))
	details.SetString(bundle.RelationKeyRelationKey, r.Key)
	details.SetString(bundle.RelationKeyName, r.Name)
	details.SetInt64(bundle.RelationKeyRelationFormat, int64(r.Format))

	if r.Description != "" {
		details.SetString(bundle.RelationKeyDescription, r.Description)
	}

	details.SetBool(bundle.RelationKeyIsHidden, r.IsHidden)
	details.SetBool(bundle.RelationKeyIsReadonly, r.IsReadOnly)

	// Set format-specific fields
	switch r.Format {
	case model.RelationFormat_date:
		details.SetBool(bundle.RelationKeyRelationFormatIncludeTime, r.IncludeTime)
	case model.RelationFormat_object:
		if len(r.ObjectTypes) > 0 {
			details.SetStringList(bundle.RelationKeyRelationFormatObjectTypes, r.ObjectTypes)
		}
	case model.RelationFormat_number:
		// Could add min/max support here
	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		if r.MaxLength > 0 {
			details.SetInt64(bundle.RelationKeyRelationMaxCount, int64(r.MaxLength))
		}
	}

	// Set source to indicate it's from import
	details.SetInt64(bundle.RelationKeySourceObject, int64(model.ObjectType_relation))

	// Generate unique key
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, r.Key)
	if err != nil {
		// If unique key generation fails, we can still continue without it
		// as the system will generate one if needed
	} else {
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	}

	// Set ID (will be generated if not provided)
	if id, ok := r.Extension["id"].(string); ok && id != "" {
		details.SetString(bundle.RelationKeyId, id)
	}

	return details
}

// FromDetails creates a Relation from domain.Details
func RelationFromDetails(details *domain.Details) (*Relation, error) {
	if details == nil {
		return nil, fmt.Errorf("details is nil")
	}

	r := &Relation{
		Key:         details.GetString(bundle.RelationKeyRelationKey),
		Name:        details.GetString(bundle.RelationKeyName),
		Format:      model.RelationFormat(details.GetInt64(bundle.RelationKeyRelationFormat)),
		Description: details.GetString(bundle.RelationKeyDescription),
		IsHidden:    details.GetBool(bundle.RelationKeyIsHidden),
		IsReadOnly:  details.GetBool(bundle.RelationKeyIsReadonly),
		Extension:   make(map[string]interface{}),
	}

	// Get format-specific fields
	switch r.Format {
	case model.RelationFormat_date:
		r.IncludeTime = details.GetBool(bundle.RelationKeyRelationFormatIncludeTime)
	case model.RelationFormat_object:
		r.ObjectTypes = details.GetStringList(bundle.RelationKeyRelationFormatObjectTypes)
	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		if maxCount := details.GetInt64(bundle.RelationKeyRelationMaxCount); maxCount > 0 {
			r.MaxLength = int(maxCount)
		}
	}

	// Store ID in extension if present
	if id := details.GetString(bundle.RelationKeyId); id != "" {
		r.Extension["id"] = id
	}

	return r, nil
}

// Validate checks if the relation is valid
func (r *Relation) Validate() error {
	if r.Key == "" {
		return fmt.Errorf("relation key is required")
	}
	if r.Name == "" {
		return fmt.Errorf("relation name is required")
	}
	// Note: Format 0 (RelationFormat_longtext) is valid, so we don't check for == 0
	if int(r.Format) < 0 {
		return fmt.Errorf("invalid relation format")
	}
	return nil
}

// IsBundled checks if this relation key is a bundled relation
func (r *Relation) IsBundled() bool {
	_, err := bundle.GetRelation(domain.RelationKey(r.Key))
	return err == nil
}

// CreateOptionDetails creates domain.Details for relation options (status/tag)
func (r *Relation) CreateOptionDetails(optionName string, color string) *domain.Details {
	details := domain.NewDetails()

	details.SetString(bundle.RelationKeyName, optionName)
	details.SetString(bundle.RelationKeyRelationKey, r.Key)

	if color != "" {
		details.SetString(bundle.RelationKeyRelationOptionColor, color)
	} else {
		// randomly assign a color if not provided
		details.SetString(bundle.RelationKeyRelationOptionColor, constant.RandomOptionColor().String())
	}

	// Set layout for relation option
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))

	// Generate unique key for the option
	optionKey := fmt.Sprintf("%s_%s", r.Key, optionName)
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, optionKey)
	if err != nil {
		// If unique key generation fails, we can still continue without it
		// as the system will generate one if needed
	} else {
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	}

	return details
}
