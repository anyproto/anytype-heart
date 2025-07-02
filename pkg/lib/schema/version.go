package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// Version constants for YAML schema compatibility
const (
	// VersionCurrent is the current version of the YAML schema format
	VersionCurrent = "1.0"

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

// SemanticVersion represents a parsed semantic version
type SemanticVersion struct {
	Major int
	Minor int
	Patch int
}

// ParseVersion parses a version string into its semantic components
func ParseVersion(version string) (*SemanticVersion, error) {
	if version == "" {
		version = DefaultVersion
	}

	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid version format: %s", version)
	}

	sv := &SemanticVersion{}

	// Parse major version
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", parts[0])
	}
	sv.Major = major

	// Parse minor version
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version: %s", parts[1])
	}
	sv.Minor = minor

	// Parse patch version if present
	if len(parts) >= 3 {
		patch, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid patch version: %s", parts[2])
		}
		sv.Patch = patch
	}

	return sv, nil
}

// Compare compares two semantic versions
// Returns -1 if v < other, 0 if v == other, 1 if v > other
func (v *SemanticVersion) Compare(other *SemanticVersion) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// String returns the string representation of the version
func (v *SemanticVersion) String() string {
	if v.Patch > 0 {
		return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// IsCompatibleVersion checks if a version is compatible with the current implementation
func IsCompatibleVersion(version string) bool {
	targetVersion, err := ParseVersion(version)
	if err != nil {
		return false
	}

	currentVersion, err := ParseVersion(VersionCurrent)
	if err != nil {
		// This should never happen with a valid VersionCurrent
		return false
	}

	// We support the current version and any earlier versions
	// For future versions, we only support same major version
	if targetVersion.Major == currentVersion.Major {
		return true
	}

	// We can read older major versions
	if targetVersion.Major < currentVersion.Major {
		return true
	}

	// Cannot read newer major versions (breaking changes)
	return false
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

	// Parse versions
	from, err := ParseVersion(fromVersion)
	if err != nil {
		compat.Compatible = false
		compat.Warnings = append(compat.Warnings, fmt.Sprintf("Invalid from version: %s", err))
		return compat
	}

	to, err := ParseVersion(toVersion)
	if err != nil {
		compat.Compatible = false
		compat.Warnings = append(compat.Warnings, fmt.Sprintf("Invalid to version: %s", err))
		return compat
	}

	comparison := from.Compare(to)

	switch {
	case comparison == 0:
		// Same version, fully compatible
		return compat

	case from.Major != to.Major:
		// Major version change
		if from.Major < to.Major {
			// Upgrading major version
			compat.Warnings = append(compat.Warnings,
				fmt.Sprintf("Major version upgrade from %s to %s may introduce breaking changes", fromVersion, toVersion))
		} else {
			// Downgrading major version
			compat.Warnings = append(compat.Warnings,
				fmt.Sprintf("Major version downgrade from %s to %s may cause data loss", fromVersion, toVersion))
		}

	case from.Minor != to.Minor:
		// Minor version change within same major
		if from.Minor < to.Minor {
			// Upgrading minor version
			compat.Warnings = append(compat.Warnings,
				fmt.Sprintf("Minor version upgrade from %s to %s may add new features", fromVersion, toVersion))
		} else {
			// Downgrading minor version
			compat.Warnings = append(compat.Warnings,
				fmt.Sprintf("Minor version downgrade from %s to %s may lose features", fromVersion, toVersion))
		}

	default:
		// Patch version change only
		// No warnings needed for patch version changes
	}

	// Check if both versions are compatible with current implementation
	if !IsCompatibleVersion(fromVersion) {
		compat.Compatible = false
		compat.Warnings = append(compat.Warnings,
			fmt.Sprintf("Version %s is not compatible with current implementation", fromVersion))
	}
	if !IsCompatibleVersion(toVersion) {
		compat.Compatible = false
		compat.Warnings = append(compat.Warnings,
			fmt.Sprintf("Version %s is not compatible with current implementation", toVersion))
	}

	return compat
}
