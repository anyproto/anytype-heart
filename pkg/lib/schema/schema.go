package schema

import (
	"fmt"
	"io"
	"slices"
)

// Schema represents a single type with its relations
type Schema struct {
	Type      *Type                `json:"type"`
	Relations map[string]*Relation `json:"relations"`
}

// NewSchema creates a new empty schema
func NewSchema() *Schema {
	return &Schema{
		Relations: make(map[string]*Relation),
	}
}

// SetType sets the type for the schema
func (s *Schema) SetType(t *Type) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("invalid type: %w", err)
	}
	s.Type = t
	return nil
}

// AddRelation adds a relation to the schema
func (s *Schema) AddRelation(r *Relation) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid relation: %w", err)
	}
	s.Relations[r.Key] = r
	return nil
}

// GetType returns the schema's type
func (s *Schema) GetType() *Type {
	return s.Type
}

// GetRelation returns a relation by key
func (s *Schema) GetRelation(key string) (*Relation, bool) {
	r, ok := s.Relations[key]
	return r, ok
}

// GetTypeByName returns the type if the name matches
func (s *Schema) GetTypeByName(name string) (*Type, bool) {
	if s.Type != nil && s.Type.Name == name {
		return s.Type, true
	}
	return nil, false
}

// GetRelationByName returns a relation by name
func (s *Schema) GetRelationByName(name string) (*Relation, bool) {
	for _, r := range s.Relations {
		if r.Name == name {
			return r, true
		}
	}
	return nil, false
}

// Validate validates the entire schema
func (s *Schema) Validate() error {
	// Validate type
	if s.Type != nil {
		if err := s.Type.Validate(); err != nil {
			return fmt.Errorf("invalid type: %w", err)
		}
	}

	// Validate all relations
	for key, r := range s.Relations {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("invalid relation %s: %w", key, err)
		}
	}

	// Validate type-relation references
	if s.Type != nil {
		// Check featured relations exist
		for _, relId := range s.Type.FeaturedRelations {
			if _, ok := s.Relations[relId]; !ok {
				// Could be a bundled relation, so we don't error
				continue
			}
		}

		// Check recommended relations exist
		for _, relId := range s.Type.RecommendedRelations {
			if _, ok := s.Relations[relId]; !ok {
				// Could be a bundled relation, so we don't error
				continue
			}
		}
	}

	return nil
}

// Merge merges another schema into this one
func (s *Schema) Merge(other *Schema) error {
	if other == nil {
		return nil
	}

	// Merge type
	if other.Type != nil {
		if s.Type == nil {
			s.Type = other.Type
		} else if s.Type.Key == other.Type.Key {
			// Same type, merge relations
			for _, rel := range other.Type.FeaturedRelations {
				if !s.Type.HasRelation(rel) {
					s.Type.FeaturedRelations = append(s.Type.FeaturedRelations, rel)
				}
			}
			for _, rel := range other.Type.RecommendedRelations {
				if !s.Type.HasRelation(rel) {
					s.Type.RecommendedRelations = append(s.Type.RecommendedRelations, rel)
				}
			}
		}
		// If different types, keep the existing one
	}

	// Merge relations
	for key, r := range other.Relations {
		if _, ok := s.Relations[key]; !ok {
			s.Relations[key] = r
		}
	}

	return nil
}

// Clone creates a deep copy of the schema
func (s *Schema) Clone() *Schema {
	clone := NewSchema()

	// Clone type
	if s.Type != nil {
		clonedType := &Type{
			Key:                  s.Type.Key,
			Name:                 s.Type.Name,
			Description:          s.Type.Description,
			PluralName:           s.Type.PluralName,
			IconEmoji:            s.Type.IconEmoji,
			IconName:             s.Type.IconName,
			IsArchived:           s.Type.IsArchived,
			IsHidden:             s.Type.IsHidden,
			Layout:               s.Type.Layout,
			FeaturedRelations:    slices.Clone(s.Type.FeaturedRelations),
			RecommendedRelations: slices.Clone(s.Type.RecommendedRelations),
			HiddenRelations:      slices.Clone(s.Type.HiddenRelations),
			Extension:            make(map[string]interface{}),
		}
		// Clone extensions
		for k, v := range s.Type.Extension {
			clonedType.Extension[k] = v
		}
		clone.Type = clonedType
	}

	// Clone relations
	for key, r := range s.Relations {
		clonedRelation := &Relation{
			Key:         r.Key,
			Name:        r.Name,
			Format:      r.Format,
			Description: r.Description,
			IsHidden:    r.IsHidden,
			IsReadOnly:  r.IsReadOnly,
			IsMulti:     r.IsMulti,
			ObjectTypes: slices.Clone(r.ObjectTypes),
			Options:     slices.Clone(r.Options),
			Examples:    slices.Clone(r.Examples),
			IncludeTime: r.IncludeTime,
			MaxLength:   r.MaxLength,
			Extension:   make(map[string]interface{}),
		}
		// Clone extensions
		for k, v := range r.Extension {
			clonedRelation.Extension[k] = v
		}
		clone.Relations[key] = clonedRelation
	}

	return clone
}

// Parser interface for parsing schemas
type Parser interface {
	Parse(reader io.Reader) (*Schema, error)
}

// Exporter interface for exporting schemas
type Exporter interface {
	Export(schema *Schema, writer io.Writer) error
}
