package markdown

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	"github.com/globalsign/mgo/bson"
	"gopkg.in/yaml.v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	yamlDelimiter = "---"
)

var emailRe = regexp.MustCompile(
	`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`,
)

// extractYAMLFrontMatter extracts YAML front matter from markdown content
// Returns the front matter content, the markdown content without front matter, and any error
func extractYAMLFrontMatter(content []byte) (frontMatter []byte, markdownContent []byte, err error) {
	// Check if content starts with YAML delimiter
	contentStr := string(content)
	if !strings.HasPrefix(strings.TrimSpace(contentStr), yamlDelimiter) {
		return nil, content, nil
	}

	// Find the end of YAML front matter
	lines := strings.Split(contentStr, "\n")
	if len(lines) < 2 {
		return nil, content, nil
	}

	// Skip first --- line
	endIndex := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == yamlDelimiter {
			endIndex = i
			break
		}
	}

	if endIndex == -1 {
		// No closing delimiter found
		return nil, content, nil
	}

	// Extract front matter (between delimiters)
	frontMatterLines := lines[1:endIndex]
	frontMatter = []byte(strings.Join(frontMatterLines, "\n"))

	// Extract remaining markdown content
	markdownLines := lines[endIndex+1:]
	markdownContent = []byte(strings.Join(markdownLines, "\n"))

	return frontMatter, markdownContent, nil
}

// yamlProperty represents a parsed YAML property with its format information
type yamlProperty struct {
	name        string // Original property name from YAML
	key         string // Property key (from schema or generated)
	format      model.RelationFormat
	value       domain.Value
	includeTime bool // For date relations, whether to include time
}

// YAMLParseResult contains the parsed YAML data with format information
type YAMLParseResult struct {
	Details    *domain.Details
	Properties []yamlProperty
	ObjectType string // If "type" or "Object type" property is present
}

// YAMLPropertyResolver resolves property keys from names
type YAMLPropertyResolver interface {
	// ResolvePropertyKey returns the property key for a given name
	// Returns empty string if not found in schema
	ResolvePropertyKey(name string) string
	
	// GetRelationFormat returns the format for a given relation key
	GetRelationFormat(key string) model.RelationFormat
	
	// ResolveOptionValue converts option name to option ID
	ResolveOptionValue(relationKey string, optionName string) string
	
	// ResolveOptionValues converts option names to option IDs
	ResolveOptionValues(relationKey string, optionNames []string) []string
}

// parseYAMLFrontMatter parses YAML front matter and returns properties with their formats
func parseYAMLFrontMatter(frontMatter []byte) (*YAMLParseResult, error) {
	return parseYAMLFrontMatterWithResolver(frontMatter, nil)
}

// parseYAMLFrontMatterWithResolver parses YAML front matter using an optional property resolver
func parseYAMLFrontMatterWithResolver(frontMatter []byte, resolver YAMLPropertyResolver) (*YAMLParseResult, error) {
	if len(frontMatter) == 0 {
		return nil, nil
	}

	var data map[string]interface{}
	err := yaml.Unmarshal(frontMatter, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML front matter: %w", err)
	}

	result := &YAMLParseResult{
		Details:    domain.NewDetails(),
		Properties: make([]yamlProperty, 0),
	}

	// Check for object type property (case-insensitive)
	var typeKey string
	for k, v := range data {
		if strings.EqualFold(k, "object type") || strings.EqualFold(k, "type") {
			if typeStr, ok := v.(string); ok {
				result.ObjectType = typeStr
				typeKey = k
				break
			}
		}
	}
	
	// Remove the type key from data so it's not processed as a property
	if typeKey != "" {
		delete(data, typeKey)
	}

	// Process remaining properties in one pass
	for key, value := range data {
		// Process value and determine format in one go
		prop := processYAMLProperty(key, value)
		if prop == nil {
			continue
		}

		// Try to resolve property key from schema if resolver is available
		if resolver != nil {
			if schemaKey := resolver.ResolvePropertyKey(key); schemaKey != "" {
				prop.key = schemaKey
				// Get the actual format from schema
				schemaFormat := resolver.GetRelationFormat(schemaKey)
				if schemaFormat != model.RelationFormat_longtext {
					prop.format = schemaFormat
				}
			} else {
				// Generate BSON ID for this property if not in schema
				prop.key = bson.NewObjectId().Hex()
			}
		} else {
			// Generate BSON ID for this property
			prop.key = bson.NewObjectId().Hex()
		}

		// Now resolve option values if needed
		if resolver != nil && (prop.format == model.RelationFormat_status || prop.format == model.RelationFormat_tag) {
			prop.value = resolveOptionValue(prop, resolver)
		}

		// Store in details
		result.Details.Set(domain.RelationKey(prop.key), prop.value)
		result.Properties = append(result.Properties, *prop)
	}

	return result, nil
}

// processYAMLProperty processes a single YAML property and returns its configuration
func processYAMLProperty(key string, value interface{}) *yamlProperty {
	prop := &yamlProperty{
		name:        key,
		format:      model.RelationFormat_shorttext, // default
		includeTime: false,
	}

	switch v := value.(type) {
	case time.Time:
		// YAML parsed a date string
		prop.format = model.RelationFormat_date
		prop.value = domain.Int64(v.Unix())
		prop.includeTime = v.Hour() != 0 || v.Minute() != 0 || v.Second() != 0 || v.Nanosecond() != 0

	case string:
		// Try to parse as date if key suggests it or value looks like date
		lowerKey := strings.ToLower(key)
		if looksLikeDate(v) {
			if t, hasTime, err := parseDate(v); err == nil {
				prop.format = model.RelationFormat_date
				prop.value = domain.Int64(t.Unix())
				prop.includeTime = hasTime
				return prop
			}
		}

		// Check for special formats
		if isURL(v) {
			prop.format = model.RelationFormat_url
		} else if isEmail(v) {
			prop.format = model.RelationFormat_email
		} else if len(v) > 100 {
			prop.format = model.RelationFormat_longtext
		} else if containsStatusKeyword(lowerKey) && len(v) < 50 {
			prop.format = model.RelationFormat_status
		}
		prop.value = domain.String(v)

	case bool:
		prop.format = model.RelationFormat_checkbox
		prop.value = domain.Bool(v)

	case int:
		prop.format = model.RelationFormat_number
		prop.value = domain.Int64(int64(v))

	case int64:
		prop.format = model.RelationFormat_number
		prop.value = domain.Int64(v)

	case float64:
		prop.format = model.RelationFormat_number
		if v == float64(int64(v)) {
			prop.value = domain.Int64(int64(v))
		} else {
			prop.value = domain.Float64(v)
		}

	case []interface{}:
		prop.format = model.RelationFormat_tag
		strSlice := make([]string, 0, len(v))
		for _, item := range v {
			strSlice = append(strSlice, fmt.Sprintf("%v", item))
		}
		prop.value = domain.StringList(strSlice)

	default:
		// Skip unsupported types (like maps, interfaces, etc.)
		return nil
	}

	return prop
}

func containsStatusKeyword(key string) bool {
	// Only consider exact matches or common variations
	lowerKey := strings.ToLower(key)
	statusKeywords := []string{"status", "state", "stage", "phase"}
	for _, keyword := range statusKeywords {
		if lowerKey == keyword || lowerKey == keyword+"s" ||
			strings.HasSuffix(lowerKey, "_"+keyword) ||
			strings.HasSuffix(lowerKey, " "+keyword) {
			return true
		}
	}
	return false
}

func looksLikeDate(s string) bool {
	// Simple check for common date patterns
	datePatterns := []string{"-", "/", "jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}
	lower := strings.ToLower(s)
	for _, pattern := range datePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func isEmail(s string) bool {
	return emailRe.MatchString(s)
}

// parseDate tries to parse a string as a date and returns whether it includes time
func parseDate(dateStr string) (time.Time, bool, error) {
	// Try parsing with dateparse library which handles many formats
	t, err := dateparse.ParseAny(dateStr)
	if err != nil {
		return time.Time{}, false, err
	}

	// Check if the original string contains time indicators
	hasTime := false
	lowerStr := strings.ToLower(dateStr)

	// Check for explicit time patterns
	timePatterns := []string{
		":",        // Time separator (HH:MM)
		"am", "pm", // 12-hour format
		"t",      // ISO 8601 time separator
		"z",      // Timezone indicator
		"+", "-", // Timezone offset
	}

	for _, pattern := range timePatterns {
		if strings.Contains(lowerStr, pattern) {
			hasTime = true
			break
		}
	}

	// If no time patterns found, check if parsed time is not midnight
	if !hasTime && (t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0) {
		hasTime = true
	}

	return t, hasTime, nil
}

// resolveOptionValue converts option names to IDs for status/tag relations
func resolveOptionValue(prop *yamlProperty, resolver YAMLPropertyResolver) domain.Value {
	switch prop.format {
	case model.RelationFormat_status:
		// For status, we expect a single string value
		if strVal := prop.value.String(); strVal != "" {
			optionId := resolver.ResolveOptionValue(prop.key, strVal)
			return domain.String(optionId)
		}
	case model.RelationFormat_tag:
		// For tags, we expect a list of strings
		if strList := prop.value.StringList(); len(strList) > 0 {
			optionIds := resolver.ResolveOptionValues(prop.key, strList)
			return domain.StringList(optionIds)
		}
	}
	return prop.value
}