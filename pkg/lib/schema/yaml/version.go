package yaml

import (
	"fmt"
	"strings"
)

// Version constants for YAML schema compatibility
const (
	// VersionCurrent is the current version of the YAML schema format
	VersionCurrent = "1.0"

	// VersionHeaderKey is the YAML property key for version information
	VersionHeaderKey = "_schema_version"

	// DefaultVersion is used when no version is specified
	DefaultVersion = VersionCurrent
)

// VersionInfo contains version metadata for a YAML document
type VersionInfo struct {
	Version string
	// Features supported by this version
	Features map[string]bool
}

// GetVersionInfo returns version information for a given version string
func GetVersionInfo(version string) (*VersionInfo, error) {
	switch version {
	case VersionCurrent, "":
		return &VersionInfo{
			Version: VersionCurrent,
			Features: map[string]bool{
				"property_resolver":     true,
				"custom_property_names": true,
				"schema_export":         true,
				"file_path_resolution":  true,
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported schema version: %s", version)
	}
}

// DetectVersion detects the version from YAML data
func DetectVersion(data map[string]interface{}) string {
	if version, ok := data[VersionHeaderKey]; ok {
		if versionStr, ok := version.(string); ok {
			return versionStr
		}
	}
	return DefaultVersion
}

// IsCompatibleVersion checks if a version is compatible with the current implementation
func IsCompatibleVersion(version string) bool {
	switch version {
	case VersionCurrent, "":
		return true
	default:
		// Check for future versions (1.1, 2.0, etc.)
		if strings.Contains(version, ".") {
			// We can read newer versions but might not support all features
			return true
		}
		return false
	}
}

// MigrateOptions contains options for migrating between versions
type MigrateOptions struct {
	// PreserveCustomNames preserves custom property names during migration
	PreserveCustomNames bool

	// AddVersionHeader adds version header to output
	AddVersionHeader bool

	// PropertyNameMap maps old property names to new ones
	PropertyNameMap map[string]string
}

// MigrateData migrates YAML data from one version to another
// Currently this is a no-op as we only have version 1.0
// This function demonstrates the migration pattern for future versions
func MigrateData(data map[string]interface{}, fromVersion, toVersion string, options *MigrateOptions) (map[string]interface{}, error) {
	if options == nil {
		options = &MigrateOptions{}
	}

	// Create a copy of the data
	result := make(map[string]interface{})
	for k, v := range data {
		result[k] = v
	}

	// Remove old version header if present
	delete(result, VersionHeaderKey)

	// Add new version header if requested
	if options.AddVersionHeader {
		result[VersionHeaderKey] = toVersion
	}

	// No-op example: future migrations would go here
	// Example for future version 2.0:
	/*
	switch {
	case fromVersion == "1.0" && toVersion == "2.0":
		// Example: Rename "Title" to "Name" in v2.0
		if title, exists := result["Title"]; exists {
			result["Name"] = title
			delete(result, "Title")
		}
		
		// Example: Convert single values to arrays in v2.0
		if assignee, exists := result["Assignee"]; exists {
			if str, ok := assignee.(string); ok {
				result["Assignees"] = []string{str}
				delete(result, "Assignee")
			}
		}
		
	case fromVersion == "2.0" && toVersion == "1.0":
		// Downgrade example: reverse the changes
		if name, exists := result["Name"]; exists {
			result["Title"] = name
			delete(result, "Name")
		}
	}
	*/

	// Currently no migrations needed
	return result, nil
}

// VersionCompatibility represents compatibility between versions
type VersionCompatibility struct {
	FromVersion string
	ToVersion   string
	Compatible  bool
	Warnings    []string
}

// CheckCompatibility checks compatibility between two versions
func CheckCompatibility(fromVersion, toVersion string) *VersionCompatibility {
	compat := &VersionCompatibility{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Compatible:  true,
		Warnings:    []string{},
	}

	// Normalize empty version to default
	if fromVersion == "" {
		fromVersion = DefaultVersion
	}
	if toVersion == "" {
		toVersion = DefaultVersion
	}

	// Check specific version combinations
	switch {
	case fromVersion == toVersion:
		// Same version, fully compatible
		return compat

	default:
		// Currently all versions are compatible as we only have 1.0
		// Future version checks would go here
		// Example:
		/*
		case fromVersion == "1.0" && toVersion == "2.0":
			compat.Warnings = append(compat.Warnings,
				"Some property names may change in version 2.0",
				"Arrays will be used for multi-value properties")
		
		case fromVersion == "2.0" && toVersion == "1.0":
			compat.Warnings = append(compat.Warnings,
				"Downgrading may lose array values",
				"Only first value will be preserved for multi-value properties")
		*/
		
		if !IsCompatibleVersion(fromVersion) || !IsCompatibleVersion(toVersion) {
			compat.Compatible = false
			compat.Warnings = append(compat.Warnings,
				fmt.Sprintf("Unknown version combination: %s to %s", fromVersion, toVersion))
		}
	}

	return compat
}