package schema

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// PropertyResolver resolves property keys and formats from names.
// objectTypeName is the display Name of the type declared by the current
// file (e.g. via `Object type:` in YAML frontmatter). Implementations
// should prefer a relation defined by the matching schema before falling
// back to other schemas, so two schemas declaring properties with the
// same display name but different keys do not collide. An empty
// objectTypeName means "no scope" and implementations must behave as a
// global search.
type PropertyResolver interface {
	// ResolvePropertyKey returns the property key for a given name, scoped
	// to the schema of objectTypeName when possible. Returns empty string
	// if not found in any schema.
	ResolvePropertyKey(objectTypeName, name string) string

	// GetRelationFormat returns the format for a given relation key, scoped
	// to the schema of objectTypeName when possible.
	GetRelationFormat(objectTypeName, key string) model.RelationFormat

	// ResolveOptionValue converts option name to option ID
	ResolveOptionValue(relationKey string, optionName string) string

	// ResolveOptionValues converts option names to option IDs
	ResolveOptionValues(relationKey string, optionNames []string) []string
}

// SchemaProvider provides access to schemas
type SchemaProvider interface {
	// GetSchema returns a schema by type key
	GetSchema(typeKey string) (*Schema, bool)

	// GetSchemaByTypeName returns a schema by type name
	GetSchemaByTypeName(typeName string) (*Schema, bool)

	// GetRelation returns a relation by key across all schemas
	GetRelation(relationKey string) (*Relation, bool)

	// GetRelationByName returns a relation by name across all schemas
	GetRelationByName(relationName string) (*Relation, bool)

	// ListSchemas returns all available schemas
	ListSchemas() []*Schema
}

// RelationResolver resolves relations by key or name
type RelationResolver interface {
	// GetRelation returns a relation by key
	GetRelation(key string) (*Relation, bool)

	// GetRelationByName returns a relation by name
	GetRelationByName(name string) (*Relation, bool)
}

// TypeResolver resolves types by key or name
type TypeResolver interface {
	// GetType returns a type by key
	GetType(key string) (*Type, bool)

	// GetTypeByName returns a type by name
	GetTypeByName(name string) (*Type, bool)
}

// SchemaRegistry manages multiple schemas
type SchemaRegistry interface {
	SchemaProvider
	PropertyResolver

	// RegisterSchema adds a schema to the registry
	RegisterSchema(schema *Schema) error

	// RemoveSchema removes a schema by type key
	RemoveSchema(typeKey string) error

	// Clear removes all schemas
	Clear()
}
