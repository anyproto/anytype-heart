package yaml

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestExportToYAML(t *testing.T) {
	tests := []struct {
		name       string
		properties []Property
		options    *ExportOptions
		want       string
	}{
		{
			name: "simple properties",
			properties: []Property{
				{
					Name:   "title",
					Key:    "title",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Test Page"),
				},
				{
					Name:   "author",
					Key:    "author",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("John Doe"),
				},
				{
					Name:   "published",
					Key:    "published",
					Format: model.RelationFormat_checkbox,
					Value:  domain.Bool(true),
				},
			},
			want: `---
author: John Doe
published: true
title: Test Page
---
`,
		},
		{
			name: "with object type",
			properties: []Property{
				{
					Name:   "name",
					Key:    "name",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("My Task"),
				},
			},
			options: &ExportOptions{
				IncludeObjectType: true,
				ObjectTypeName:    "Task",
			},
			want: `---
Object type: Task
name: My Task
---
`,
		},
		{
			name: "with custom property names",
			properties: []Property{
				{
					Name:   "task_title",
					Key:    "task_title",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Complete integration"),
				},
				{
					Name:   "task_status",
					Key:    "task_status",
					Format: model.RelationFormat_status,
					Value:  domain.String("in-progress"),
				},
			},
			options: &ExportOptions{
				PropertyNameMap: map[string]string{
					"task_title":  "Title",
					"task_status": "Status",
				},
			},
			want: `---
Status: in-progress
Title: Complete integration
---
`,
		},
		{
			name: "with skip properties",
			properties: []Property{
				{
					Name:   "title",
					Key:    "title",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Test"),
				},
				{
					Name:   "internal_id",
					Key:    "internal_id",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("12345"),
				},
				{
					Name:   "author",
					Key:    "author",
					Format: model.RelationFormat_shorttext,
					Value:  domain.String("Jane"),
				},
			},
			options: &ExportOptions{
				SkipProperties: []string{"internal_id"},
			},
			want: `---
author: Jane
title: Test
---
`,
		},
		{
			name: "date properties",
			properties: []Property{
				{
					Name:        "created",
					Key:         "created",
					Format:      model.RelationFormat_date,
					Value:       domain.Int64(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Unix()),
					IncludeTime: false,
				},
				{
					Name:        "updated",
					Key:         "updated",
					Format:      model.RelationFormat_date,
					Value:       domain.Int64(time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC).Unix()),
					IncludeTime: true,
				},
			},
			want: `---
created: "2024-01-15"
updated: "2024-01-15T14:30:00Z"
---
`,
		},
		{
			name: "array properties",
			properties: []Property{
				{
					Name:   "tags",
					Key:    "tags",
					Format: model.RelationFormat_tag,
					Value:  domain.StringList([]string{"test", "markdown", "yaml"}),
				},
				{
					Name:   "files",
					Key:    "files",
					Format: model.RelationFormat_object,
					Value:  domain.StringList([]string{"doc1.md", "doc2.md"}),
				},
			},
			want: `---
files:
    - doc1.md
    - doc2.md
tags:
    - test
    - markdown
    - yaml
---
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExportToYAML(tt.properties, tt.options)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(result))
		})
	}
}

func TestExportDetailsToYAML(t *testing.T) {
	details := domain.NewDetails()
	details.Set("title", domain.String("Test Document"))
	details.Set("author", domain.String("John Doe"))
	details.Set("created", domain.Int64(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Unix()))
	details.Set("tags", domain.StringList([]string{"test", "yaml"}))
	details.Set("published", domain.Bool(true))

	formats := map[string]model.RelationFormat{
		"title":     model.RelationFormat_shorttext,
		"author":    model.RelationFormat_shorttext,
		"created":   model.RelationFormat_date,
		"tags":      model.RelationFormat_tag,
		"published": model.RelationFormat_checkbox,
	}

	options := &ExportOptions{
		IncludeObjectType: true,
		ObjectTypeName:    "Page",
		PropertyNameMap: map[string]string{
			"title":  "Title",
			"author": "Author",
		},
	}

	result, err := ExportDetailsToYAML(details, formats, options)
	require.NoError(t, err)

	// Parse back to verify
	parsed, _, err := ExtractYAMLFrontMatter(result)
	require.NoError(t, err)

	parsedResult, err := ParseYAMLFrontMatter(parsed)
	require.NoError(t, err)

	// Verify object type
	assert.Equal(t, "Page", parsedResult.ObjectType)

	// Verify properties exist with correct names
	propNames := make(map[string]bool)
	for _, prop := range parsedResult.Properties {
		propNames[prop.Name] = true
	}

	assert.True(t, propNames["Title"])
	assert.True(t, propNames["Author"])
	assert.True(t, propNames["created"])
	assert.True(t, propNames["tags"])
	assert.True(t, propNames["published"])
}