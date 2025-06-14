package markdown

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)


func TestExtractYAMLFrontMatter(t *testing.T) {
	tests := []struct {
		name                string
		input               string
		wantFrontMatter     string
		wantMarkdownContent string
		wantErr             bool
	}{
		{
			name: "valid yaml front matter",
			input: `---
title: Test Page
author: John Doe
date: 2024-01-01
tags: [test, markdown]
---

# Heading

This is content.`,
			wantFrontMatter: `title: Test Page
author: John Doe
date: 2024-01-01
tags: [test, markdown]`,
			wantMarkdownContent: `
# Heading

This is content.`,
		},
		{
			name: "no yaml front matter",
			input: `# Heading

This is content.`,
			wantFrontMatter:     "",
			wantMarkdownContent: `# Heading

This is content.`,
		},
		{
			name: "empty yaml front matter",
			input: `---
---

# Heading`,
			wantFrontMatter:     "",
			wantMarkdownContent: `
# Heading`,
		},
		{
			name: "yaml without closing delimiter",
			input: `---
title: Test
author: John

# Heading`,
			wantFrontMatter:     "",
			wantMarkdownContent: `---
title: Test
author: John

# Heading`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontMatter, markdownContent, err := extractYAMLFrontMatter([]byte(tt.input))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantFrontMatter, string(frontMatter))
			assert.Equal(t, tt.wantMarkdownContent, string(markdownContent))
		})
	}
}

func TestParseYAMLFrontMatter(t *testing.T) {
	tests := []struct {
		name         string
		frontMatter  string
		wantProps    map[string]string // property name -> expected format name
		wantValues   map[string]interface{}
		wantObjType  string
		wantErr      bool
	}{
		{
			name: "simple properties",
			frontMatter: `title: Test Page
author: John Doe
published: true
views: 1000`,
			wantProps: map[string]string{
				"title":     "shorttext",
				"author":    "shorttext",
				"published": "checkbox",
				"views":     "number",
			},
			wantValues: map[string]interface{}{
				"title":     "Test Page",
				"author":    "John Doe",
				"published": true,
				"views":     int64(1000),
			},
		},
		{
			name: "with type property",
			frontMatter: `title: Test
type: Task
status: in-progress`,
			wantProps: map[string]string{
				"title":  "shorttext",
				"status": "status",
			},
			wantValues: map[string]interface{}{
				"title":  "Test",
				"status": "in-progress",
			},
			wantObjType: "Task",
		},
		{
			name: "with date properties",
			frontMatter: `Start Date: 2023-06-01
End Time: 2023-06-01T14:30:00Z
version: 1.2.3`,
			wantProps: map[string]string{
				"Start Date": "date",
				"End Time":   "date",
				"version":    "shorttext",
			},
			wantValues: map[string]interface{}{
				"Start Date": int64(time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC).Unix()),
				"End Time":   int64(time.Date(2023, 6, 1, 14, 30, 0, 0, time.UTC).Unix()),
				"version":    "1.2.3",
			},
		},
		{
			name:        "invalid yaml",
			frontMatter: `[not valid yaml`,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseYAMLFrontMatter([]byte(tt.frontMatter))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)

			// Check object type
			assert.Equal(t, tt.wantObjType, result.ObjectType)

			// Check properties
			propMap := make(map[string]yamlProperty)
			for _, prop := range result.Properties {
				propMap[prop.name] = prop
			}

			for propName, expectedFormat := range tt.wantProps {
				prop, ok := propMap[propName]
				assert.True(t, ok, "Property %s not found", propName)
				
				actualFormat := ""
				switch prop.format {
				case model.RelationFormat_shorttext:
					actualFormat = "shorttext"
				case model.RelationFormat_checkbox:
					actualFormat = "checkbox"
				case model.RelationFormat_number:
					actualFormat = "number"
				case model.RelationFormat_date:
					actualFormat = "date"
				case model.RelationFormat_status:
					actualFormat = "status"
				}
				assert.Equal(t, expectedFormat, actualFormat, "Wrong format for property %s", propName)
			}

			// Check values
			for propName, expectedValue := range tt.wantValues {
				prop, ok := propMap[propName]
				assert.True(t, ok, "Property %s not found", propName)
				
				switch expected := expectedValue.(type) {
				case string:
					assert.Equal(t, expected, prop.value.String())
				case bool:
					assert.Equal(t, expected, prop.value.Bool())
				case int64:
					assert.Equal(t, expected, prop.value.Int64())
				}
			}
		})
	}
}





func TestParseYAMLWithTimeTimeValues(t *testing.T) {
	// Test that YAML parser correctly handles date strings that YAML converts to time.Time
	yamlContent := `Start Date: 2023-06-01
End Time: 2023-06-01T14:30:00Z
created: 2024-01-01
version: 1.2.3
type: Task
`
	result, err := parseYAMLFrontMatter([]byte(yamlContent))
	require.NoError(t, err)
	require.NotNil(t, result)
	
	// Check object type
	assert.Equal(t, "Task", result.ObjectType)
	
	// Check properties
	propMap := make(map[string]yamlProperty)
	for _, prop := range result.Properties {
		propMap[prop.name] = prop
	}
	
	// All date fields should be detected as date format
	assert.Equal(t, model.RelationFormat_date, propMap["Start Date"].format)
	assert.Equal(t, model.RelationFormat_date, propMap["End Time"].format)
	assert.Equal(t, model.RelationFormat_date, propMap["created"].format)
	assert.Equal(t, model.RelationFormat_shorttext, propMap["version"].format)
	
	// Check includeTime flags
	assert.False(t, propMap["Start Date"].includeTime)
	assert.True(t, propMap["End Time"].includeTime)
	assert.False(t, propMap["created"].includeTime)
	
	// All date values should be timestamps
	assert.True(t, propMap["Start Date"].value.IsInt64())
	assert.True(t, propMap["End Time"].value.IsInt64())
	assert.True(t, propMap["created"].value.IsInt64())
	
	// Version should remain as string
	assert.True(t, propMap["version"].value.IsString())
	
	// Check actual timestamp values
	startDate := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, startDate.Unix(), propMap["Start Date"].value.Int64())
}