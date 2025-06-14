# YAML Front Matter Implementation Summary

## Overview
We've implemented YAML front matter support for markdown import that parses metadata at the beginning of markdown files and converts it to Anytype object properties.

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

1. **yamlfrontmatter.go** (Created)
   - Main YAML parsing logic
   - `extractYAMLFrontMatter()`: Extracts YAML block
   - `parseYAMLFrontMatter()`: Parses YAML and returns structured result
   - `processYAMLProperty()`: Handles individual property conversion
   - `tryParseDate()`: Date parsing with multiple format support

2. **blockconverter.go** (Modified)
   - Integrated YAML parsing into the conversion flow
   - Stores YAML properties in FileInfo structure

3. **import.go** (Modified)
   - Creates relation snapshots from YAML properties
   - Handles object type assignment
   - Updated `getRelationDetails()` to support date include time flag

4. **yamlfrontmatter_test.go** (Created)
   - Comprehensive tests for YAML parsing
   - Tests for date handling with time.Time values

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