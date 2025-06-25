package yaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestGetVersionInfo(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		wantVersion  string
		wantFeatures map[string]bool
		wantErr      bool
	}{
		{
			name:        "current version",
			version:     VersionCurrent,
			wantVersion: VersionCurrent,
			wantFeatures: map[string]bool{
				"property_resolver":     true,
				"custom_property_names": true,
				"schema_export":         true,
				"file_path_resolution":  true,
			},
		},
		{
			name:        "version 1.0",
			version:     "1.0",
			wantVersion: VersionCurrent,
			wantFeatures: map[string]bool{
				"property_resolver":     true,
				"custom_property_names": true,
				"schema_export":         true,
				"file_path_resolution":  true,
			},
		},
		{
			name:        "empty version defaults to current",
			version:     "",
			wantVersion: VersionCurrent,
			wantFeatures: map[string]bool{
				"property_resolver":     true,
				"custom_property_names": true,
				"schema_export":         true,
				"file_path_resolution":  true,
			},
		},
		{
			name:    "unsupported version",
			version: "v99",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := GetVersionInfo(tt.version)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantVersion, info.Version)
			assert.Equal(t, tt.wantFeatures, info.Features)
		})
	}
}

func TestDetectVersion(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{
			name: "version header present",
			data: map[string]interface{}{
				VersionHeaderKey: VersionCurrent,
				"title":          "Test",
			},
			want: VersionCurrent,
		},
		{
			name: "no version header",
			data: map[string]interface{}{
				"title": "Test",
			},
			want: DefaultVersion,
		},
		{
			name: "invalid version type",
			data: map[string]interface{}{
				VersionHeaderKey: 123, // not a string
				"title":          "Test",
			},
			want: DefaultVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectVersion(tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		expected  *SemanticVersion
		expectErr bool
	}{
		{
			name:    "valid major.minor version",
			version: "1.0",
			expected: &SemanticVersion{
				Major: 1,
				Minor: 0,
				Patch: 0,
			},
		},
		{
			name:    "valid major.minor.patch version",
			version: "2.1.3",
			expected: &SemanticVersion{
				Major: 2,
				Minor: 1,
				Patch: 3,
			},
		},
		{
			name:    "empty version defaults to current",
			version: "",
			expected: &SemanticVersion{
				Major: 1,
				Minor: 0,
				Patch: 0,
			},
		},
		{
			name:      "invalid format - single number",
			version:   "1",
			expectErr: true,
		},
		{
			name:      "invalid format - non-numeric major",
			version:   "a.0",
			expectErr: true,
		},
		{
			name:      "invalid format - non-numeric minor",
			version:   "1.b",
			expectErr: true,
		},
		{
			name:      "invalid format - non-numeric patch",
			version:   "1.0.c",
			expectErr: true,
		},
		{
			name:    "extra parts are ignored",
			version: "1.2.3.4",
			expected: &SemanticVersion{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseVersion(tt.version)
			if tt.expectErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSemanticVersion_Compare(t *testing.T) {
	tests := []struct {
		name     string
		v1       *SemanticVersion
		v2       *SemanticVersion
		expected int
	}{
		{
			name:     "equal versions",
			v1:       &SemanticVersion{Major: 1, Minor: 0, Patch: 0},
			v2:       &SemanticVersion{Major: 1, Minor: 0, Patch: 0},
			expected: 0,
		},
		{
			name:     "v1 major < v2 major",
			v1:       &SemanticVersion{Major: 1, Minor: 5, Patch: 10},
			v2:       &SemanticVersion{Major: 2, Minor: 0, Patch: 0},
			expected: -1,
		},
		{
			name:     "v1 major > v2 major",
			v1:       &SemanticVersion{Major: 2, Minor: 0, Patch: 0},
			v2:       &SemanticVersion{Major: 1, Minor: 9, Patch: 99},
			expected: 1,
		},
		{
			name:     "same major, v1 minor < v2 minor",
			v1:       &SemanticVersion{Major: 1, Minor: 2, Patch: 10},
			v2:       &SemanticVersion{Major: 1, Minor: 3, Patch: 0},
			expected: -1,
		},
		{
			name:     "same major, v1 minor > v2 minor",
			v1:       &SemanticVersion{Major: 1, Minor: 3, Patch: 0},
			v2:       &SemanticVersion{Major: 1, Minor: 2, Patch: 99},
			expected: 1,
		},
		{
			name:     "same major.minor, v1 patch < v2 patch",
			v1:       &SemanticVersion{Major: 1, Minor: 2, Patch: 3},
			v2:       &SemanticVersion{Major: 1, Minor: 2, Patch: 4},
			expected: -1,
		},
		{
			name:     "same major.minor, v1 patch > v2 patch",
			v1:       &SemanticVersion{Major: 1, Minor: 2, Patch: 5},
			v2:       &SemanticVersion{Major: 1, Minor: 2, Patch: 4},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v1.Compare(tt.v2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSemanticVersion_String(t *testing.T) {
	tests := []struct {
		name     string
		version  *SemanticVersion
		expected string
	}{
		{
			name:     "major.minor only",
			version:  &SemanticVersion{Major: 1, Minor: 0, Patch: 0},
			expected: "1.0",
		},
		{
			name:     "with patch version",
			version:  &SemanticVersion{Major: 2, Minor: 1, Patch: 3},
			expected: "2.1.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.version.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCompatibleVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{
			name:    "version 1.0",
			version: "1.0",
			want:    true,
		},
		{
			name:    "current version",
			version: VersionCurrent,
			want:    true,
		},
		{
			name:    "empty version",
			version: "",
			want:    true,
		},
		{
			name:    "same major, higher minor",
			version: "1.1",
			want:    true,
		},
		{
			name:    "same major, higher patch",
			version: "1.0.1",
			want:    true,
		},
		{
			name:    "older major version",
			version: "0.9",
			want:    true,
		},
		{
			name:    "future major version 2.0",
			version: "2.0",
			want:    false, // Cannot read newer major versions
		},
		{
			name:    "invalid version",
			version: "invalid",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCompatibleVersion(tt.version)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckCompatibility(t *testing.T) {
	tests := []struct {
		name            string
		fromVersion     string
		toVersion       string
		expectCompat    bool
		expectWarnings  int
		warningContains []string
	}{
		{
			name:           "same version",
			fromVersion:    "1.0",
			toVersion:      "1.0",
			expectCompat:   true,
			expectWarnings: 0,
		},
		{
			name:            "minor upgrade",
			fromVersion:     "1.0",
			toVersion:       "1.1",
			expectCompat:    true,
			expectWarnings:  1,
			warningContains: []string{"Minor version upgrade", "may add new features"},
		},
		{
			name:            "minor downgrade",
			fromVersion:     "1.2",
			toVersion:       "1.0",
			expectCompat:    true,
			expectWarnings:  1,
			warningContains: []string{"Minor version downgrade", "may lose features"},
		},
		{
			name:            "major upgrade",
			fromVersion:     "1.0",
			toVersion:       "2.0",
			expectCompat:    false,
			expectWarnings:  2,
			warningContains: []string{"Major version upgrade", "breaking changes", "not compatible"},
		},
		{
			name:            "major downgrade",
			fromVersion:     "2.0",
			toVersion:       "1.0",
			expectCompat:    false,
			expectWarnings:  2,
			warningContains: []string{"Major version downgrade", "data loss", "not compatible"},
		},
		{
			name:           "patch upgrade",
			fromVersion:    "1.0.1",
			toVersion:      "1.0.2",
			expectCompat:   true,
			expectWarnings: 0,
		},
		{
			name:            "invalid from version",
			fromVersion:     "invalid",
			toVersion:       "1.0",
			expectCompat:    false,
			expectWarnings:  1,
			warningContains: []string{"Invalid from version"},
		},
		{
			name:            "invalid to version",
			fromVersion:     "1.0",
			toVersion:       "bad",
			expectCompat:    false,
			expectWarnings:  1,
			warningContains: []string{"Invalid to version"},
		},
		{
			name:           "empty versions",
			fromVersion:    "",
			toVersion:      "",
			expectCompat:   true,
			expectWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckCompatibility(tt.fromVersion, tt.toVersion)
			assert.Equal(t, tt.expectCompat, result.Compatible)
			assert.Len(t, result.Warnings, tt.expectWarnings)

			for _, contains := range tt.warningContains {
				found := false
				for _, warning := range result.Warnings {
					if strings.Contains(warning, contains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected warning containing '%s' not found in: %v", contains, result.Warnings)
				}
			}
		})
	}
}

// TestVersionSpecificParsing tests parsing with different versions
func TestVersionSpecificParsing(t *testing.T) {
	t.Run("version 1.0 parsing", func(t *testing.T) {
		yamlData := `
title: Test Document
status: active
tags:
  - important
  - review
`
		result, err := ParseYAMLFrontMatter([]byte(yamlData))
		require.NoError(t, err)
		require.NotNil(t, result)

		// Legacy parsing generates BSON IDs for keys, so we check properties instead
		assert.Equal(t, 3, len(result.Properties))

		// Find properties by name
		var titleProp, statusProp, tagsProp *Property
		for i := range result.Properties {
			switch result.Properties[i].Name {
			case "title":
				titleProp = &result.Properties[i]
			case "status":
				statusProp = &result.Properties[i]
			case "tags":
				tagsProp = &result.Properties[i]
			}
		}

		require.NotNil(t, titleProp)
		require.NotNil(t, statusProp)
		require.NotNil(t, tagsProp)

		assert.Equal(t, "Test Document", titleProp.Value.String())
		assert.Equal(t, "active", statusProp.Value.String())
		assert.Equal(t, []string{"important", "review"}, tagsProp.Value.StringList())
	})

	t.Run("current version with resolver", func(t *testing.T) {
		yamlData := `
_schema_version: 1.0
title: Test Document
status: active
tags:
  - important
  - review
`
		// In a real scenario, this would use a PropertyResolver
		// For this test, we're just verifying the structure
		result, err := ParseYAMLFrontMatter([]byte(yamlData))
		require.NoError(t, err)
		require.NotNil(t, result)

		// Find title property by name
		var titleProp *Property
		for i := range result.Properties {
			if result.Properties[i].Name == "title" {
				titleProp = &result.Properties[i]
				break
			}
		}

		require.NotNil(t, titleProp)
		assert.Equal(t, "Test Document", titleProp.Value.String())

		// Verify version header is not included in properties
		for _, prop := range result.Properties {
			assert.NotEqual(t, VersionHeaderKey, prop.Name)
		}
	})
}

// TestVersionedExport tests exporting with version information
func TestVersionedExport(t *testing.T) {
	properties := []Property{
		{
			Name:   "title",
			Key:    "title",
			Format: 0, // shorttext
			Value:  domain.String("Test Document"),
		},
		{
			Name:   "status",
			Key:    "status",
			Format: 8, // status
			Value:  domain.String("active"),
		},
	}

	t.Run("export without version", func(t *testing.T) {
		result, err := ExportToYAML(properties, nil)
		require.NoError(t, err)

		// Should not contain version header
		assert.NotContains(t, string(result), VersionHeaderKey)
	})

	t.Run("export with version header", func(t *testing.T) {
		// This would require modifying ExportOptions to support version header
		// For now, we're just documenting the expected behavior
		result, err := ExportToYAML(properties, &ExportOptions{})
		require.NoError(t, err)
		assert.NotNil(t, result)
	})
}
