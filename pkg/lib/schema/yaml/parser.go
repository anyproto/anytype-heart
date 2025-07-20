package yaml

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	"github.com/globalsign/mgo/bson"
	"gopkg.in/yaml.v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

const (
	YAMLDelimiter = "---"
)

var emailRe = regexp.MustCompile(
	`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`,
)

// ExtractYAMLFrontMatter extracts YAML front matter from markdown content
// Returns the front matter content, the markdown content without front matter, and any error
func ExtractYAMLFrontMatter(content []byte) (frontMatter []byte, markdownContent []byte, err error) {
	// Check if content starts with YAML delimiter
	contentStr := string(content)
	if !strings.HasPrefix(strings.TrimSpace(contentStr), YAMLDelimiter) {
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
		if strings.TrimSpace(lines[i]) == YAMLDelimiter {
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

// Property represents a parsed YAML property with its format information
type Property struct {
	Name        string // Original property name from YAML
	Key         string // Property key (from schema or generated)
	Format      model.RelationFormat
	Value       domain.Value
	IncludeTime bool // For date relations, whether to include time
}

// ParseResult contains the parsed YAML data with format information
type ParseResult struct {
	Details    *domain.Details
	Properties []Property
	ObjectType string // If "type" or "Object type" property is present
}

// ParseYAMLFrontMatter parses YAML front matter and returns properties with their formats
func ParseYAMLFrontMatter(frontMatter []byte) (*ParseResult, error) {
	return ParseYAMLFrontMatterWithResolver(frontMatter, nil)
}

// ParseYAMLFrontMatterWithResolver parses YAML front matter using an optional property resolver
func ParseYAMLFrontMatterWithResolver(frontMatter []byte, resolver schema.PropertyResolver) (*ParseResult, error) {
	return ParseYAMLFrontMatterWithResolverAndPath(frontMatter, resolver, "")
}

// ParseYAMLFrontMatterWithFormats parses YAML front matter with pre-defined formats
// should be removed
func ParseYAMLFrontMatterWithFormats(frontMatter []byte, formats map[string]model.RelationFormat) (*ParseResult, error) {
	if len(frontMatter) == 0 {
		return nil, nil
	}

	var data map[string]interface{}
	err := yaml.Unmarshal(frontMatter, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML front matter: %w", err)
	}

	result := &ParseResult{
		Details:    domain.NewDetails(),
		Properties: make([]Property, 0),
	}

	// Check for object type property (case-insensitive)
	var typeKey string
	for k, v := range data {
		if strings.EqualFold(k, "object type") || strings.EqualFold(k, "type") {
			// if array of strings, take the first one
			if arr, ok := v.([]interface{}); ok && len(arr) > 0 {
				v = arr[0]
			}
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

		// Use provided format if available
		if format, ok := formats[prop.Key]; ok {
			prop.Format = format
		}

		// Store in details
		result.Details.Set(domain.RelationKey(prop.Key), prop.Value)
		result.Properties = append(result.Properties, *prop)
	}

	return result, nil
}

// ParseYAMLFrontMatterWithResolverAndPath parses YAML front matter using an optional property resolver and base file path
func ParseYAMLFrontMatterWithResolverAndPath(frontMatter []byte, resolver schema.PropertyResolver, baseFilePath string) (*ParseResult, error) {
	if len(frontMatter) == 0 {
		return nil, nil
	}

	var data map[string]interface{}
	err := yaml.Unmarshal(frontMatter, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML front matter: %w", err)
	}

	result := &ParseResult{
		Details:    domain.NewDetails(),
		Properties: make([]Property, 0),
	}

	// Check for object type property (case-insensitive)
	var typeKey string
	for k, v := range data {
		if strings.EqualFold(k, "object type") || strings.EqualFold(k, "type") {
			// if array of strings, take the first one
			if arr, ok := v.([]interface{}); ok && len(arr) > 0 {
				v = arr[0]
			}
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

		if prop.Key == "" {
			// Try to resolve property key from schema if resolver is available
			if resolver != nil {
				if schemaKey := resolver.ResolvePropertyKey(prop.Name); schemaKey != "" {
					prop.Key = schemaKey
					// Get the actual format from schema
					schemaFormat := resolver.GetRelationFormat(schemaKey)
					if schemaFormat != model.RelationFormat_longtext {
						prop.Format = schemaFormat
					}
				} else {
					// Generate BSON ID for this property if not in schema
					prop.Key = bson.NewObjectId().Hex()
				}
			} else {
				// Generate BSON ID for this property
				prop.Key = bson.NewObjectId().Hex()
			}
		}

		// Now resolve option values if needed
		if resolver != nil && (prop.Format == model.RelationFormat_status || prop.Format == model.RelationFormat_tag) {
			prop.Value = resolveOptionValue(prop, resolver)
		}

		// Resolve file paths for object relations
		if (prop.Format == model.RelationFormat_object || prop.Format == model.RelationFormat_file) && baseFilePath != "" {
			prop.Value = resolveFilePaths(prop.Value, baseFilePath)
		}

		// Store in details
		result.Details.Set(domain.RelationKey(prop.Key), prop.Value)
		result.Properties = append(result.Properties, *prop)
	}

	return result, nil
}

var replaceMap = map[string]domain.RelationKey{
	"tag":      bundle.RelationKeyTag,
	"status":   bundle.RelationKeyStatus,
	"tags":     bundle.RelationKeyTag,
	"created":  bundle.RelationKeyCreatedDate,
	"modified": bundle.RelationKeyLastModifiedDate,
}

// processYAMLProperty processes a single YAML property and returns its configuration
func processYAMLProperty(key string, value interface{}) *Property {
	prop := &Property{
		Name:        key,
		Format:      model.RelationFormat_shorttext, // default
		IncludeTime: false,
	}

	switch v := value.(type) {
	case time.Time:
		// YAML parsed a date string
		prop.Format = model.RelationFormat_date
		prop.Value = domain.Int64(v.Unix())
		prop.IncludeTime = v.Hour() != 0 || v.Minute() != 0 || v.Second() != 0 || v.Nanosecond() != 0

	case string:
		// Try to parse as date if key suggests it or value looks like date
		lowerKey := strings.ToLower(key)
		if looksLikeDate(v) {
			if t, hasTime, err := parseDate(v); err == nil {
				prop.Format = model.RelationFormat_date
				prop.Value = domain.Int64(t.Unix())
				prop.IncludeTime = hasTime
				return prop
			}
		}

		// Check for special formats
		if isURL(v) {
			prop.Format = model.RelationFormat_url
		} else if isEmail(v) {
			prop.Format = model.RelationFormat_email
		} else if len(v) > 100 {
			prop.Format = model.RelationFormat_longtext
		} else if containsStatusKeyword(lowerKey) && len(v) < 50 {
			prop.Format = model.RelationFormat_status
		} else if isFilePath(v) {
			// Detect object format for file paths
			prop.Format = model.RelationFormat_object
		}
		prop.Value = domain.String(v)

	case bool:
		prop.Format = model.RelationFormat_checkbox
		prop.Value = domain.Bool(v)

	case int:
		prop.Format = model.RelationFormat_number
		prop.Value = domain.Int64(int64(v))

	case int64:
		prop.Format = model.RelationFormat_number
		prop.Value = domain.Int64(v)

	case float64:
		prop.Format = model.RelationFormat_number
		if v == float64(int64(v)) {
			prop.Value = domain.Int64(int64(v))
		} else {
			prop.Value = domain.Float64(v)
		}

	case []interface{}:
		strSlice := make([]string, 0, len(v))
		hasFilePaths := false
		for _, item := range v {
			itemStr := fmt.Sprintf("%v", item)
			if isFilePath(itemStr) {
				hasFilePaths = true
			}
			strSlice = append(strSlice, itemStr)
		}

		// If array contains file paths, treat as object relation, otherwise as tag
		if hasFilePaths {
			prop.Format = model.RelationFormat_object
		} else {
			prop.Format = model.RelationFormat_tag
		}
		prop.Value = domain.StringList(strSlice)

	default:
		// Skip unsupported types (like maps, interfaces, etc.)
		return nil
	}

	if relKey, ok := replaceMap[strings.ToLower(key)]; ok {
		rel := bundle.MustGetRelation(relKey)
		if prop.Format == rel.Format {
			prop.Key = relKey.String()
			prop.Name = rel.Name
		}
	}

	return prop
}

func containsStatusKeyword(key string) bool {
	// Only consider exact matches or common variations
	lowerKey := strings.ToLower(key)
	statusKeywords := []string{"status", "state", "stage", "phase", "progress", "condition", "situation", "priority", "severity", "activity"}
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

func isFilePath(s string) bool {
	// Check if string looks like a file path
	// Must have an extension and not be a URL
	if isURL(s) {
		return false
	}

	// Check for file extension
	ext := filepath.Ext(s)
	if ext == "" {
		return false
	}

	// Common markdown and document extensions
	commonExts := []string{".md", ".txt", ".doc", ".docx", ".pdf", ".html", ".csv", ".json", ".xml"}
	for _, commonExt := range commonExts {
		if strings.EqualFold(ext, commonExt) {
			return true
		}
	}

	// Also check if it contains path separators which indicates it's likely a file path
	return strings.Contains(s, "/") || strings.Contains(s, "\\")
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
func resolveOptionValue(prop *Property, resolver schema.PropertyResolver) domain.Value {
	switch prop.Format {
	case model.RelationFormat_status:
		// if array choose the first value
		var strVal string
		if prop.Value.IsStringList() {
			strList := prop.Value.StringList()
			if len(strList) > 0 {
				strVal = strList[0]
			}
		} else if prop.Value.IsString() {
			strVal = prop.Value.String()
		}

		if strVal != "" {
			optionId := resolver.ResolveOptionValue(prop.Key, strVal)
			return domain.String(optionId)
		}
	case model.RelationFormat_tag:
		// For tags, we expect a list of strings
		if strList := prop.Value.StringList(); len(strList) > 0 {
			optionIds := resolver.ResolveOptionValues(prop.Key, strList)
			return domain.StringList(optionIds)
		}
	}
	return prop.Value
}

// resolveFilePaths prepends baseFilePath to relative file paths
func resolveFilePaths(value domain.Value, baseFilePath string) domain.Value {
	if value.IsString() {
		path := value.String()
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseFilePath, path)
		}
		return domain.String(path)
	} else if value.IsStringList() {
		paths := value.StringList()
		resolvedPaths := make([]string, len(paths))
		for i, path := range paths {
			if !filepath.IsAbs(path) {
				resolvedPaths[i] = filepath.Join(baseFilePath, path)
			} else {
				resolvedPaths[i] = path
			}
		}
		return domain.StringList(resolvedPaths)
	}
	return value
}
