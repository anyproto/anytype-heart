# Schema-Based Markdown Import

## Overview

This feature enhances markdown import to use JSON schemas when available, providing a more deterministic way to create types and relations during import. When exporting from Anytype with schemas and then importing back, the structure will be preserved exactly.

## How It Works

### 1. Schema Detection
During import, the system checks for a `schemas/` folder containing `.schema.json` files. These files define:
- Object types with their properties
- Relation formats and metadata
- Property ordering and featured status
- Unique keys for deduplication

### 2. Schema Structure
Example schema file (`schemas/task.schema.json`):
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "urn:anytype:schema:2024-06-14:author-user:type-task:gen-1.0.0",
  "type": "object",
  "title": "Task",
  "x-type-key": "task",
  "properties": {
    "Name": {
      "type": "string",
      "x-featured": true,
      "x-order": 2,
      "x-key": "name"
    },
    "Status": {
      "type": "string",
      "enum": ["Todo", "In Progress", "Done"],
      "x-featured": true,
      "x-order": 3,
      "x-key": "status"
    }
  }
}
```

### 3. Key Features

#### Deterministic Import
- Uses `x-key` to match existing relations by their internal key
- Uses `x-type-key` to match existing object types
- Preserves property order using `x-order`
- Maintains featured/non-featured property distinction with `x-featured`

#### Format Detection
The schema parser automatically detects relation formats from JSON Schema properties:
- `boolean` → checkbox
- `number` → number
- `string` with `format: date` → date (without time)
- `string` with `format: date-time` → date (with time)
- `string` with `format: email` → email
- `string` with `format: uri` → url
- `string` with `enum` → status
- `array` of strings → tag
- `object` → object relation

#### Fallback Support
If no schemas are found, the import falls back to the original YAML-based property detection, ensuring backward compatibility.

## Implementation Details

### New Components

1. **SchemaImporter** (`schema.go`)
   - Loads and parses JSON schemas
   - Creates relation and type snapshots
   - Manages deduplication using x-key

2. **Integration** (`import.go`)
   - Modified to load schemas before processing files
   - Uses schema-defined relations when mapping YAML properties
   - Creates types and relations from schemas instead of YAML

### Round-Trip Support

When you:
1. Export objects with rich properties from Anytype (includes schemas)
2. Import the markdown files back into a new space
   - All types are recreated with the same structure
   - All relations maintain their formats and settings
   - No duplicate types/relations are created

When importing into the same space:
- Uses `x-key` to find existing relations
- Uses `x-type-key` to find existing types
- Reuses existing entities instead of creating duplicates

## Usage

1. **Export with schemas**: Use the markdown export with schema option enabled
2. **Import with schemas**: Place the exported files (including `schemas/` folder) in your import directory
3. **Automatic detection**: The importer will automatically use schemas when available

## Benefits

- **Preserves Structure**: Exact recreation of types and relations
- **No Duplicates**: Smart matching using x-key prevents duplicate creation
- **Rich Metadata**: Preserves all relation settings (format, include time, etc.)
- **Type Safety**: JSON Schema validation ensures correct property types
- **Extensible**: Easy to add new property formats and metadata

## Testing

Run the schema import tests:
```bash
go test ./core/block/import/markdown -run TestSchemaImporter -v
```

The tests verify:
- Schema loading and parsing
- Relation format detection
- Type and relation snapshot creation
- Key-based lookups for deduplication