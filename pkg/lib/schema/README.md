# Schema Package

The schema package provides a flexible way to define and work with object types and their properties (relations) in Anytype. It supports JSON schema definitions, YAML front matter parsing, and integration with the import/export system.

## Key Components

### Core Types

- **Schema**: Container for a type and its relations
- **Type**: Defines an object type (e.g., Task, Project)
- **Relation**: Defines a property/field (e.g., Title, Status, Due Date)

### Interfaces

The package defines several interfaces for extensibility:

- **PropertyResolver**: Resolves property names to keys and formats
- **SchemaProvider**: Provides access to schemas
- **SchemaRegistry**: Manages multiple schemas with registration/lookup

### Parsers and Exporters

- **JSONSchemaParser**: Parses JSON schema format
- **JSONSchemaExporter**: Exports to JSON schema format
- **YAML Integration**: Parse YAML front matter with schema awareness

## Usage Examples

### Creating a Schema

```go
// Create a new schema
schema := schema.NewSchema()

// Define a type
taskType := &schema.Type{
    Key:         "task",
    Name:        "Task",
    Description: "A task or todo item",
    IconEmoji:   "✅",
}
schema.SetType(taskType)

// Add relations
titleRel := &schema.Relation{
    Key:         "task_title",
    Name:        "Title",
    Format:      model.RelationFormat_shorttext,
    Description: "Task title",
}
schema.AddRelation(titleRel)

statusRel := &schema.Relation{
    Key:         "task_status", 
    Name:        "Status",
    Format:      model.RelationFormat_status,
    Options:     []string{"Todo", "In Progress", "Done"},
}
schema.AddRelation(statusRel)
```

### Using with YAML Parser

```go
// Create a schema registry
registry := NewSchemaRegistry()
registry.RegisterSchema(taskSchema)

// Parse YAML with schema awareness
yamlContent := []byte(`---
type: Task
Title: Complete integration
Status: In Progress
Tags: [urgent, feature]
---`)

frontMatter, content, _ := yaml.ExtractYAMLFrontMatter(yamlContent)
result, _ := yaml.ParseYAMLFrontMatterWithResolver(frontMatter, registry)

// Properties are resolved to schema keys
// result.Properties[0].Key == "task_title"
// result.Properties[1].Key == "task_status"
```

### JSON Schema Import/Export

```go
// Export schema to JSON
exporter := schema.NewJSONSchemaExporter("  ")
var buf bytes.Buffer
exporter.Export(mySchema, &buf)

// Import from JSON
parser := schema.NewJSONSchemaParser()
importedSchema, _ := parser.Parse(&buf)
```

## Integration with Markdown Import

The schema package integrates seamlessly with the markdown import system:

1. Load schema files from import directory
2. Use schemas to resolve property names in YAML front matter
3. Create proper relation snapshots with correct formats
4. Handle option values for status/tag relations

```go
// In markdown import
importer := NewSchemaImporter()
importer.LoadSchemas(source, errors)

// Parse markdown with schema resolver
yamlResult, _ := yaml.ParseYAMLFrontMatterWithResolver(
    frontMatter, 
    importer, // implements PropertyResolver
)
```

## Schema File Format

Schemas are defined in JSON format:

```json
{
  "type": {
    "key": "task",
    "name": "Task",
    "description": "A task or todo item",
    "iconEmoji": "✅",
    "featuredRelations": ["task_title", "task_status"],
    "recommendedRelations": ["task_priority", "task_assignee"]
  },
  "relations": {
    "task_title": {
      "key": "task_title",
      "name": "Title",
      "format": "shorttext",
      "description": "The title of the task"
    },
    "task_status": {
      "key": "task_status", 
      "name": "Status",
      "format": "status",
      "options": ["Todo", "In Progress", "Done"]
    }
  }
}
```

## Supported Relation Formats

The schema system supports all Anytype relation formats:

- Text: `shorttext`, `longtext`
- Numbers: `number` 
- Dates: `date` (with optional time)
- Selection: `status` (single), `tag` (multiple)
- References: `object`, `file`
- Contact: `email`, `phone`, `url`
- Other: `checkbox`

## Testing

The package includes comprehensive tests:

- Unit tests for core types
- Integration tests with YAML parser
- Round-trip tests for import/export
- Testdata files with real examples

Run tests with:
```bash
go test ./pkg/lib/schema/...
```

# Property Name Deduplication Example

This document demonstrates the name deduplication feature that prevents conflicts when exporting types and properties with duplicate names.

## Problem

When exporting schemas to JSON Schema or YAML, multiple relations can have the same display name, causing conflicts:

```go
// Multiple relations with the same "Name" display name
relations := []*Relation{
    {Key: "user_name", Name: "Name", Format: model.RelationFormat_shorttext},
    {Key: "company_name", Name: "Name", Format: model.RelationFormat_shorttext}, 
    {Key: "project_name", Name: "Name", Format: model.RelationFormat_shorttext},
}
```

## Solution

The deduplication system:
1. **Sorts relations by their key** for consistent ordering
2. **Keeps the first occurrence** with the original name
3. **Adds index suffixes** to subsequent duplicates (e.g., "Name 2", "Name 3")

## Example Output

### JSON Schema Export
```json
{
  "properties": {
    "Name": {
      "type": "string",
      "x-key": "company_name",
      "x-format": "shorttext"
    },
    "Name 2": {
      "type": "string", 
      "x-key": "project_name",
      "x-format": "shorttext",
      "x-hidden": true
    },
    "Name 3": {
      "type": "string",
      "x-key": "user_name", 
      "x-format": "shorttext",
      "x-featured": true
    }
  }
}
```

### YAML Export
```yaml
---
Object type: Entity
Name: Acme Corp         # company_name (first alphabetically)
Name 2: Project Alpha   # project_name (second alphabetically)  
Name 3: John Doe        # user_name (third alphabetically)
Title: Main Title
Description: Detailed description
---
```
