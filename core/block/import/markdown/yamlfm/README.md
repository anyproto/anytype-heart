# YAML Frontmatter Package

The `yamlfm` package provides functionality for parsing YAML frontmatter from Markdown files in Anytype imports.

## Overview

This package handles:
- Extraction of YAML frontmatter from Markdown content
- Parsing YAML data with automatic type detection
- Property resolution through schema integration
- Relation format inference
- Option value resolution for status/tag relations

## Main Types

### `Property`
Represents a parsed YAML property with format information:
```go
type Property struct {
    Name        string               // Original property name from YAML
    Key         string               // Property key (from schema or generated)
    Format      model.RelationFormat // Inferred or resolved format
    Value       domain.Value         // Parsed value
    IncludeTime bool                 // For date relations
}
```

### `ParseResult`
Contains the complete parsed YAML data:
```go
type ParseResult struct {
    Details    *domain.Details // Property key -> value mapping
    Properties []Property      // List of all properties
    ObjectType string          // Object type if specified
}
```

### `PropertyResolver`
Interface for schema-based property resolution:
```go
type PropertyResolver interface {
    ResolvePropertyKey(name string) string
    GetRelationFormat(key string) model.RelationFormat
    ResolveOptionValue(relationKey string, optionName string) string
    ResolveOptionValues(relationKey string, optionNames []string) []string
}
```

## Usage

### Basic Usage
```go
// Extract frontmatter from markdown
frontMatter, content, err := yamlfm.ExtractYAMLFrontMatter(markdownBytes)

// Parse without schema
result, err := yamlfm.ParseYAMLFrontMatter(frontMatter)

// Access parsed properties
for _, prop := range result.Properties {
    fmt.Printf("%s: %v (format: %v)\n", prop.Name, prop.Value, prop.Format)
}
```

### With Schema Resolution
```go
// Parse with schema resolver
result, err := yamlfm.ParseYAMLFrontMatterWithResolver(frontMatter, schemaImporter)

// Properties will have schema-defined keys instead of generated ones
```

## Format Detection

The package automatically detects relation formats based on:

1. **Value Type**:
   - `bool` → checkbox
   - `number` → number
   - `[]interface{}` → tag

2. **Content Analysis**:
   - URLs → url format
   - Email addresses → email format
   - Long text (>100 chars) → longtext
   - Date patterns → date format

3. **Key Analysis**:
   - Keys containing status/state/phase → status format

4. **Schema Override**:
   - When a resolver is provided, schema-defined formats take precedence

## Special Properties

### Object Type
Properties named "type" or "Object type" (case-insensitive) are treated specially:
- Extracted as `ObjectType` in the result
- Not included in the properties list
- Used to determine the Anytype object type

### Date Handling
The package intelligently detects whether dates include time:
- Checks for time patterns (colons, AM/PM, timezone)
- Sets `IncludeTime` flag accordingly
- Parses various date formats using the `dateparse` library

## Integration

This package is used by the Markdown importer to:
1. Extract metadata from Markdown files
2. Create relation snapshots for properties
3. Apply property values to imported objects
4. Resolve property keys when schemas are available