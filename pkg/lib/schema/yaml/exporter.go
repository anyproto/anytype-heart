package yaml

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

// ExportOptions configures YAML export behavior
type ExportOptions struct {
	// IncludeObjectType adds the object type to the front matter
	IncludeObjectType bool
	// ObjectTypeName is the name of the object type
	ObjectTypeName string
	// SkipProperties is a list of property keys to skip during export
	SkipProperties []string
	// PropertyNameMap maps property keys to custom names for export
	PropertyNameMap map[string]string
}

// ExportToYAML exports properties to YAML front matter format
func ExportToYAML(properties []Property, options *ExportOptions) ([]byte, error) {
	if options == nil {
		options = &ExportOptions{}
	}

	options.SkipProperties = append(options.SkipProperties, bundle.RelationKeyId.String())
	// Create a map for YAML marshaling
	data := make(map[string]interface{})

	// Add object type if requested
	if options.IncludeObjectType && options.ObjectTypeName != "" {
		data["Object type"] = options.ObjectTypeName
		options.SkipProperties = append(options.SkipProperties, bundle.RelationKeyType.String())
	}

	// Process properties
	skipMap := make(map[string]bool)
	for _, skip := range options.SkipProperties {
		skipMap[skip] = true
	}

	for _, prop := range properties {
		// Skip if in skip list
		if skipMap[prop.Key] {
			continue
		}

		// Determine property name
		name := prop.Name
		if customName, ok := options.PropertyNameMap[prop.Key]; ok {
			name = customName
		}

		// Convert value based on format
		value := convertValueForExport(prop)
		if value != nil {
			data[name] = value
		}
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Format as front matter
	var result strings.Builder
	result.WriteString(YAMLDelimiter)
	result.WriteString("\n")
	result.Write(yamlData)
	if !strings.HasSuffix(string(yamlData), "\n") {
		result.WriteString("\n")
	}
	result.WriteString(YAMLDelimiter)
	result.WriteString("\n")

	return []byte(result.String()), nil
}

// ExportDetailsToYAML exports a Details map to YAML front matter format
func ExportDetailsToYAML(details *domain.Details, formats map[string]model.RelationFormat, options *ExportOptions) ([]byte, error) {
	if details == nil {
		return nil, nil
	}

	// Convert Details to Properties
	properties := make([]Property, 0, details.Len())

	for key, value := range details.Iterate() {
		keyStr := string(key)

		// Determine format
		format := model.RelationFormat_shorttext
		if f, ok := formats[keyStr]; ok {
			format = f
		}

		// Create property
		prop := Property{
			Name:   keyStr,
			Key:    keyStr,
			Format: format,
			Value:  value,
		}

		// For dates, check if time should be included
		if format == model.RelationFormat_date && value.IsInt64() {
			t := time.Unix(value.Int64(), 0)
			prop.IncludeTime = t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0
		}

		properties = append(properties, prop)
	}

	return ExportToYAML(properties, options)
}

// ExportSchemaToYAML exports a Schema to YAML format
func ExportSchemaToYAML(s *schema.Schema, options *ExportOptions) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("schema is nil")
	}

	if options == nil {
		options = &ExportOptions{}
	}

	// Prepare properties from schema relations
	properties := make([]Property, 0)

	// Add object type if requested
	if options.IncludeObjectType && s.Type != nil {
		// Type is not included as a property but can be set in ObjectTypeName
		options.ObjectTypeName = s.Type.Name
	}

	// Sort relation keys for consistent output
	var relationKeys []string
	for key := range s.Relations {
		relationKeys = append(relationKeys, key)
	}
	sort.Strings(relationKeys)

	// Process relations from schema
	for _, key := range relationKeys {
		relation := s.Relations[key]
		// Skip if in skip list
		skip := false
		for _, skipKey := range options.SkipProperties {
			if skipKey == key {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Use custom name if provided
		name := relation.Name
		if customName, ok := options.PropertyNameMap[key]; ok {
			name = customName
		}

		// Create property based on relation
		prop := Property{
			Name:        name,
			Key:         key,
			Format:      relation.Format,
			IncludeTime: relation.IncludeTime,
		}

		// Set example values based on format
		switch relation.Format {
		case model.RelationFormat_date:
			// Example date
			t := time.Now()
			if !relation.IncludeTime {
				// Set to midnight for date-only
				t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			}
			prop.Value = domain.Int64(t.Unix())

		case model.RelationFormat_checkbox:
			prop.Value = domain.Bool(false)

		case model.RelationFormat_number:
			prop.Value = domain.Float64(0)

		case model.RelationFormat_status:
			// Use first option if available
			if len(relation.Options) > 0 {
				prop.Value = domain.String(relation.Options[0])
			} else {
				prop.Value = domain.String("")
			}

		case model.RelationFormat_tag:
			// Use example tags if available
			if len(relation.Examples) > 0 {
				prop.Value = domain.StringList(relation.Examples)
			} else if len(relation.Options) > 0 {
				// Use first few options as example
				examples := relation.Options
				if len(examples) > 3 {
					examples = examples[:3]
				}
				prop.Value = domain.StringList(examples)
			} else {
				prop.Value = domain.StringList([]string{})
			}

		case model.RelationFormat_object:
			// For object relations, include empty array or example references
			if len(relation.ObjectTypes) > 0 {
				// Add empty array but property should still be included
				prop.Value = domain.StringList([]string{})
			} else {
				prop.Value = domain.StringList([]string{})
			}

		case model.RelationFormat_file:
			// For file relations, include empty array
			prop.Value = domain.StringList([]string{})

		default:
			// For text formats, use description or empty string
			if relation.Description != "" {
				prop.Value = domain.String(relation.Description)
			} else {
				prop.Value = domain.String("")
			}
		}

		properties = append(properties, prop)
	}

	return ExportToYAML(properties, options)
}

// convertValueForExport converts a domain.Value to a suitable YAML representation
func convertValueForExport(prop Property) interface{} {
	value := prop.Value

	switch prop.Format {
	case model.RelationFormat_date:
		if value.IsInt64() {
			t := time.Unix(value.Int64(), 0).UTC()
			if prop.IncludeTime {
				return t.Format(time.RFC3339)
			}
			// Return date-only as string to ensure it's quoted in YAML
			return t.Format("2006-01-02")
		}

	case model.RelationFormat_checkbox:
		if value.IsBool() {
			return value.Bool()
		}

	case model.RelationFormat_number:
		if value.IsInt64() {
			return value.Int64()
		} else if value.IsFloat64() {
			return value.Float64()
		}

	case model.RelationFormat_url, model.RelationFormat_email, model.RelationFormat_phone:
		if value.IsString() {
			return value.String()
		}

	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		if value.IsString() {
			return value.String()
		}

	case model.RelationFormat_tag, model.RelationFormat_object, model.RelationFormat_file:
		if value.IsStringList() {
			list := value.StringList()
			// Return empty list as empty array instead of nil
			return list
		} else if value.IsString() && value.String() != "" {
			// Single value as string
			return value.String()
		}

	case model.RelationFormat_status:
		if value.IsString() {
			return value.String()
		}

	default:
		// For unknown formats, try to export as string
		if value.IsString() {
			return value.String()
		} else if value.IsStringList() {
			list := value.StringList()
			if len(list) > 0 {
				return list
			}
		}
	}

	return nil
}
