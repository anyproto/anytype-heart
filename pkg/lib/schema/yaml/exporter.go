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
)

// ExportOptions configures YAML export behavior
type ExportOptions struct {
	// SkipProperties is a list of property keys to skip during export
	SkipProperties []string
	// PropertyNameMap maps property keys to custom names for export
	PropertyNameMap map[string]string
	// SchemaReference adds yaml-language-server schema reference comment
	SchemaReference string
}

// ExportToYAML exports properties to YAML front matter format
func ExportToYAML(properties []Property, options *ExportOptions) ([]byte, error) {
	if options == nil {
		options = &ExportOptions{}
	}

	// Process properties with deduplication first
	skipMap := make(map[string]bool)
	for _, skip := range options.SkipProperties {
		skipMap[skip] = true
	}

	// Filter properties and collect names for deduplication
	validProps := make([]Property, 0, len(properties))
	propNames := make([]string, 0, len(properties))
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

		validProps = append(validProps, prop)
		propNames = append(propNames, name)
	}

	// Deduplicate property names with awareness of reserved names
	deduplicatedNames := deduplicateYAMLPropertyNamesWithReserved(validProps, propNames)

	// Create ordered YAML node to preserve property order
	var rootNode yaml.Node
	rootNode.Kind = yaml.MappingNode

	// Add properties in order
	for i, prop := range validProps {
		// Convert value based on format
		value := convertValueForExport(prop)
		if value != nil {
			// Add key node
			keyNode := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: deduplicatedNames[i],
			}

			// Add value node
			valueNode := &yaml.Node{}
			valueNode.Encode(value)

			rootNode.Content = append(rootNode.Content, keyNode, valueNode)
		}
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(&rootNode)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Format as front matter
	var result strings.Builder
	result.WriteString(YAMLDelimiter)
	result.WriteString("\n")

	// Add schema reference if provided
	if options.SchemaReference != "" {
		result.WriteString("# yaml-language-server: $schema=")
		result.WriteString(options.SchemaReference)
		result.WriteString("\n")
	}

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
		return value.WrapToStringList()
	case model.RelationFormat_status:
		v := value.WrapToStringList()
		if len(v) > 0 {
			// For status, we return the first value as string
			return v[0]
		}
		return ""
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

// deduplicateYAMLPropertyNamesWithReserved ensures no duplicate property names in YAML export
// by sorting properties by key and adding index suffixes when needed
func deduplicateYAMLPropertyNamesWithReserved(properties []Property, names []string) []string {
	// Create a map to track names and their property indices
	nameToProperties := make(map[string][]struct {
		index int
		key   string
	})

	// Group properties by name
	for i, name := range names {
		nameToProperties[name] = append(nameToProperties[name], struct {
			index int
			key   string
		}{i, properties[i].Key})
	}

	result := make([]string, len(names))

	// Process each name group
	for name, props := range nameToProperties {
		if len(props) == 1 {
			// No duplication, use original name
			result[props[0].index] = name
		} else {
			// Normal deduplication: sort by key, but give priority to bundled relations
			sort.Slice(props, func(i, j int) bool {
				// Bundled relations come first
				iIsBundled := bundle.HasRelation(domain.RelationKey(props[i].key))
				jIsBundled := bundle.HasRelation(domain.RelationKey(props[j].key))

				if iIsBundled && !jIsBundled {
					return true
				}
				if !iIsBundled && jIsBundled {
					return false
				}

				// Otherwise sort by key
				return props[i].key < props[j].key
			})

			// Add index suffix to duplicated names
			for idx, prop := range props {
				if idx == 0 {
					// First occurrence keeps original name
					result[prop.index] = name
				} else {
					// Subsequent occurrences get index suffix
					result[prop.index] = fmt.Sprintf("%s %d", name, idx+1)
				}
			}
		}
	}

	return result
}
