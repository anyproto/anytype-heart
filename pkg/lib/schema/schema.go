package schema

import (
	"fmt"
	"io"
)

// Schema represents a collection of types and relations
type Schema struct {
	Types     map[string]*Type     `json:"types"`
	Relations map[string]*Relation `json:"relations"`
}

// NewSchema creates a new empty schema
func NewSchema() *Schema {
	return &Schema{
		Types:     make(map[string]*Type),
		Relations: make(map[string]*Relation),
	}
}

// AddType adds a type to the schema
func (s *Schema) AddType(t *Type) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("invalid type: %w", err)
	}
	s.Types[t.Key] = t
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

// GetType returns a type by key
func (s *Schema) GetType(key string) (*Type, bool) {
	t, ok := s.Types[key]
	return t, ok
}

// GetRelation returns a relation by key
func (s *Schema) GetRelation(key string) (*Relation, bool) {
	r, ok := s.Relations[key]
	return r, ok
}

// GetTypeByName returns a type by name
func (s *Schema) GetTypeByName(name string) (*Type, bool) {
	for _, t := range s.Types {
		if t.Name == name {
			return t, true
		}
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
	// Validate all types
	for key, t := range s.Types {
		if err := t.Validate(); err != nil {
			return fmt.Errorf("invalid type %s: %w", key, err)
		}
	}
	
	// Validate all relations
	for key, r := range s.Relations {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("invalid relation %s: %w", key, err)
		}
	}
	
	// Validate type-relation references
	for _, t := range s.Types {
		// Check featured relations exist
		for _, relId := range t.FeaturedRelations {
			if _, ok := s.Relations[relId]; !ok {
				// Could be a bundled relation, so we don't error
				continue
			}
		}
		
		// Check recommended relations exist
		for _, relId := range t.RecommendedRelations {
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
	
	// Merge types
	for key, t := range other.Types {
		if existing, ok := s.Types[key]; ok {
			// Type already exists, merge relations
			for _, rel := range t.FeaturedRelations {
				if !existing.HasRelation(rel) {
					existing.FeaturedRelations = append(existing.FeaturedRelations, rel)
				}
			}
			for _, rel := range t.RecommendedRelations {
				if !existing.HasRelation(rel) {
					existing.RecommendedRelations = append(existing.RecommendedRelations, rel)
				}
			}
		} else {
			s.Types[key] = t
		}
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
	
	// Clone types
	for key, t := range s.Types {
		clonedType := &Type{
			Key:                  t.Key,
			Name:                 t.Name,
			Description:          t.Description,
			PluralName:           t.PluralName,
			IconEmoji:            t.IconEmoji,
			IconImage:            t.IconImage,
			IsArchived:           t.IsArchived,
			IsHidden:             t.IsHidden,
			Layout:               t.Layout,
			FeaturedRelations:    append([]string{}, t.FeaturedRelations...),
			RecommendedRelations: append([]string{}, t.RecommendedRelations...),
			RestrictedRelations:  append([]string{}, t.RestrictedRelations...),
			Extension:            make(map[string]interface{}),
		}
		// Clone extensions
		for k, v := range t.Extension {
			clonedType.Extension[k] = v
		}
		clone.Types[key] = clonedType
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
			ObjectTypes: append([]string{}, r.ObjectTypes...),
			Options:     append([]string{}, r.Options...),
			Examples:    append([]string{}, r.Examples...),
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