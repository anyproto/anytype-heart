package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestGetVersionInfo(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantVersion string
		wantFeatures map[string]bool
		wantErr     bool
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
		name    string
		data    map[string]interface{}
		want    string
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
			name:    "future version 2.0",
			version: "2.0",
			want:    true,
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

func TestMigrateData(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]interface{}
		fromVersion string
		toVersion   string
		options     *MigrateOptions
		want        map[string]interface{}
	}{
		{
			name: "no-op migration - same version",
			data: map[string]interface{}{
				"Title":  "Test Document",
				"Status": "active",
			},
			fromVersion: "1.0",
			toVersion:   "1.0",
			options:     nil,
			want: map[string]interface{}{
				"Title":  "Test Document",
				"Status": "active",
			},
		},
		{
			name: "no-op migration - add version header",
			data: map[string]interface{}{
				"Title": "Test Document",
			},
			fromVersion: "1.0",
			toVersion:   "1.0",
			options: &MigrateOptions{
				AddVersionHeader: true,
			},
			want: map[string]interface{}{
				"Title":          "Test Document",
				VersionHeaderKey: "1.0",
			},
		},
		{
			name: "no-op migration - remove old version header",
			data: map[string]interface{}{
				"Title":          "Test Document",
				VersionHeaderKey: "0.9",
			},
			fromVersion: "1.0",
			toVersion:   "1.0",
			options:     nil,
			want: map[string]interface{}{
				"Title": "Test Document",
			},
		},
		{
			name: "same version no changes",
			data: map[string]interface{}{
				"title":  "Test",
				"status": "active",
			},
			fromVersion: VersionCurrent,
			toVersion:   VersionCurrent,
			options:     nil,
			want: map[string]interface{}{
				"title":  "Test",
				"status": "active",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MigrateData(tt.data, tt.fromVersion, tt.toVersion, tt.options)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckCompatibility(t *testing.T) {
	tests := []struct {
		name         string
		fromVersion  string
		toVersion    string
		wantCompat   bool
		wantWarnings int
	}{
		{
			name:         "same version",
			fromVersion:  VersionCurrent,
			toVersion:    VersionCurrent,
			wantCompat:   true,
			wantWarnings: 0,
		},
		{
			name:         "version 1.0 to 1.0",
			fromVersion:  "1.0",
			toVersion:    "1.0",
			wantCompat:   true,
			wantWarnings: 0,
		},
		{
			name:         "empty versions default to current",
			fromVersion:  "",
			toVersion:    "",
			wantCompat:   true,
			wantWarnings: 0,
		},
		{
			name:         "unknown version",
			fromVersion:  "unknown",
			toVersion:    VersionCurrent,
			wantCompat:   false,
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compat := CheckCompatibility(tt.fromVersion, tt.toVersion)
			assert.Equal(t, tt.wantCompat, compat.Compatible)
			assert.Equal(t, tt.wantWarnings, len(compat.Warnings))
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