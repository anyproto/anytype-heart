# Markdown Import Implementation Summary

## Part 1: YAML Front Matter Support

### Overview
We've implemented YAML front matter support for markdown import that parses metadata at the beginning of markdown files and converts it to Anytype object properties. The YAML frontmatter functionality has been moved to a dedicated sub-package `yamlfm` for better modularity.

## Key Features

### 1. YAML Extraction
- Extracts content between `---` delimiters at the start of markdown files
- Handles both empty and populated YAML blocks
- Preserves original markdown content

### 2. Property Parsing
- Converts YAML key-value pairs to Anytype relations
- Generates unique BSON IDs for each property
- Supports multiple data types:
  - **Text**: Short text (<100 chars) and long text
  - **Numbers**: Integers and floats
  - **Dates**: With automatic time.Time detection
  - **Booleans**: Converted to checkbox relations
  - **Lists**: Converted to tag relations
  - **URLs**: Detected via regex pattern
  - **Emails**: Detected via regex pattern

### 3. Object Type Support
- Special handling for "Type" or "Object type" properties
- Sets the object type based on the value
- Searches for existing types by name

### 4. Date Handling
- **Fixed Issue**: YAML parser converts date strings like "2023-06-01" to time.Time objects
- Properly detects these as dates (not numbers)
- Handles both date-only and date-time values
- Sets `RelationKeyRelationFormatIncludeTime` based on time presence
- Supports multiple date formats:
  - ISO format: "2006-01-02"
  - With time: "2006-01-02T15:04:05"
  - And other common formats

## Implementation Structure

### Files Modified/Created

1. **yamlfm/yamlfrontmatter.go** (Created in sub-package)
   - Main YAML parsing logic
   - `ExtractYAMLFrontMatter()`: Extracts YAML block
   - `ParseYAMLFrontMatter()`: Parses YAML and returns structured result
   - `ParseYAMLFrontMatterWithResolver()`: Parses with schema-based property resolution
   - `processYAMLProperty()`: Handles individual property conversion
   - `parseDate()`: Date parsing with multiple format support

2. **yamlfm/yamlfrontmatter_test.go** (Created in sub-package)
   - Comprehensive tests for YAML parsing
   - Tests for date handling with time.Time values

3. **yamlfm/yaml_resolver_test.go** (Created in sub-package)
   - Tests for property resolver interface

4. **blockconverter.go** (Modified)
   - Integrated YAML parsing into the conversion flow
   - Uses yamlfm package for YAML extraction and parsing
   - Stores YAML properties in FileInfo structure

5. **import.go** (Modified)
   - Creates relation snapshots from YAML properties
   - Handles object type assignment
   - Updated to use yamlfm.Property type

6. **schema.go** (Modified)
   - Implements yamlfm.PropertyResolver interface
   - Provides schema-based property resolution

## Example Usage

Input markdown file:
```markdown
---
title: My Task
Type: Task
Start Date: 2023-06-01
End Date: 2023-06-01T14:30:00
priority: high
done: true
tags: [important, urgent]
---

# My Task Content
```

Result:
- Creates a Task object type
- Sets properties with appropriate formats:
  - title: short text
  - Start Date: date (without time)
  - End Date: date (with time)
  - priority: short text
  - done: checkbox
  - tags: tag relation

## Technical Details

### YAML Parsing
- Uses standard `gopkg.in/yaml.v3` library
- Handles YAML's automatic type conversion
- Special handling for time.Time values from YAML parser

### Property Format Detection
- Simple switch statement based on Go types
- Direct handling of time.Time for dates
- String analysis for URLs, emails, and text length

### Integration Points
- Plugs into existing markdown import pipeline
- Reuses existing relation creation logic
- Compatible with existing object type system

## Part 2: JSON Schema Support

### Overview
This implementation adds JSON schema support to the Markdown import workflow. When exporting objects with rich properties from Anytype and importing them back, the system now maintains the exact same structure using JSON schemas.

### Key Components

#### 1. Schema Parser (`schema.go`)
- **SchemaImporter**: Main struct that handles schema loading and parsing
- **SchemaInfo**: Stores parsed type information from JSON schemas
- **RelationInfo**: Stores relation metadata including format, options, and examples

#### 2. Enhanced Import Process (`import.go`)
- Integrated schema loading into the import workflow
- Falls back to YAML-based creation when no schemas are present
- Uses schema-defined types and relations when available

#### 3. Key Features Implemented

##### x-key Support
- All properties in exported schemas include `x-key` with the original RelationKey
- Import process uses `x-key` to match existing types/relations and avoid duplicates

##### Relation Option Creation
- **Status Relations**: Enum values are converted to relation option snapshots
- **Tag Relations**: Example values are converted to relation option snapshots
- Each option is created as a separate SmartBlock of type RelationOption

##### Type Creation
- Types are created with all properties from the schema
- Featured relations are marked using `x-featured` flag
- Properties are ordered using `x-order` attribute

### Schema Structure Example
```json
{
  "properties": {
    "Status": {
      "type": "string",
      "enum": ["Draft", "Published", "Archived"],
      "x-key": "custom_status_key",
      "x-featured": true,
      "x-order": 3
    },
    "Tags": {
      "type": "array",
      "items": {"type": "string"},
      "examples": ["urgent", "important"],
      "x-key": "custom_tags_key",
      "x-order": 4
    }
  }
}
```

### Methods Added

#### SchemaImporter Methods
- `LoadSchemas()`: Loads all JSON schema files from schemas/ folder
- `parseSchema()`: Parses individual schema file
- `parseRelationFromProperty()`: Extracts relation info from schema property
- `CreateRelationSnapshots()`: Creates snapshots for all discovered relations
- `CreateRelationOptionSnapshots()`: Creates option snapshots for status/tag relations
- `CreateTypeSnapshots()`: Creates type snapshots with all properties
- `GetTypeKeyByName()`: Returns type key for a given type name
- `GetRelationKeyByName()`: Returns relation key for a given property name

### Testing

#### Comprehensive Test Coverage
- `schema_test.go`: Basic schema parsing and snapshot creation
- `schema_comprehensive_test.go`: Tests all possible property types
- `schema_integration_test.go`: Integration tests with custom relations

#### Test Results
- ✅ Schema loading and parsing
- ✅ Relation format detection (all types)
- ✅ Status relation option creation
- ✅ Tag relation example creation
- ✅ Type snapshot creation with all properties
- ✅ Round-trip export/import compatibility

### Important Notes

#### Bundled Relations
- Bundled relations (like `email`, `status`, `type`) are skipped during import
- Only custom relations create new snapshots
- The system checks each relation key against the bundle to avoid duplicates

#### Relation Options
- Status options are created from `enum` values in the schema
- Tag examples are created from `examples` array in the schema
- Each option/example becomes a separate RelationOption snapshot

#### Import Workflow
1. Load schemas from `schemas/` folder if present
2. Parse each schema to extract types and relations
3. Create relation snapshots for non-bundled relations
4. Create relation option snapshots for status/tag relations
5. Create type snapshots with all properties
6. Use schema-defined keys when importing objects

### Recent Enhancements

#### x-format Support (Completed)
- Added `x-format` field to all relation properties in exported schemas
- Format is exported as `"RelationFormat_<format>"` (e.g., `"RelationFormat_file"`)
- Import now prioritizes x-format over schema structure inference
- Disambiguates between file and tag relations (both can be arrays)

#### Object Relation Schema Fix (Completed)
- Object relations now export as array of objects instead of single object
- Each object in the array has properties: Name, File, Id, Object type
- Properly represents multi-value object relations

### Import Format Detection Priority
1. **x-format** (if present) - Most reliable, explicitly specifies format
2. **Schema structure inference** - Falls back to analyzing type/format/enum

### Future Enhancements
- Support for relation constraints (min/max values, patterns)
- Import of relation colors and icons from schema
- Support for nested object schemas
- Validation of imported data against schemas