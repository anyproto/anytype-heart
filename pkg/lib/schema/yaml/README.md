# YAML Schema Package

This package provides YAML parsing and exporting functionality for Anytype's schema system, with a focus on YAML front matter support for markdown files.

## Features

- **YAML Front Matter Extraction**: Extract YAML front matter from markdown content
- **Smart Property Detection**: Automatically detect property formats (dates, URLs, emails, etc.)
- **Schema Integration**: Use property resolvers to map YAML fields to schema-defined properties
- **Export Support**: Convert properties back to YAML front matter format
- **Type Detection**: Support for "type" and "Object type" fields to determine object types

## Usage

### Parsing YAML Front Matter

```go
// Extract YAML front matter from markdown content
frontMatter, markdownContent, err := yaml.ExtractYAMLFrontMatter(content)

// Parse the front matter
result, err := yaml.ParseYAMLFrontMatter(frontMatter)

// With a property resolver for schema mapping
result, err := yaml.ParseYAMLFrontMatterWithResolver(frontMatter, resolver)
```

### Exporting to YAML

```go
// Export properties to YAML front matter
options := &yaml.ExportOptions{
    IncludeObjectType: true,
    ObjectTypeName:    "Task",
    PropertyNameMap: map[string]string{
        "task_title": "Title",
    },
}
yamlData, err := yaml.ExportToYAML(properties, options)

// Export from Details
yamlData, err := yaml.ExportDetailsToYAML(details, formats, options)
```

## Property Format Detection

The parser automatically detects the following formats:
- **Date/Time**: Various date formats with optional time inclusion
- **Numbers**: Integer and floating-point values
- **Booleans**: True/false values
- **URLs**: HTTP/HTTPS links
- **Emails**: Valid email addresses
- **File Paths**: Paths to documents with common extensions
- **Arrays**: Lists of values (treated as tags or object relations)
- **Status**: Properties with status-related keywords
- **Long Text**: Strings longer than 100 characters

## Integration with Schema System

The package supports the `PropertyResolver` interface for integrating with Anytype's schema system:

```go
type PropertyResolver interface {
    ResolvePropertyKey(name string) string
    GetRelationFormat(key string) model.RelationFormat
    ResolveOptionValue(relationKey string, optionName string) string
    ResolveOptionValues(relationKey string, optionNames []string) []string
    ResolveObjectValues(objectNames []string) []string
}
```

This allows mapping YAML field names to schema-defined property keys and handling option values for status/tag relations.