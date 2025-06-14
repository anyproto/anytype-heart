package schema

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestRelation_ToDetails(t *testing.T) {
	r := &Relation{
		Key:         "custom_status",
		Name:        "Status",
		Format:      model.RelationFormat_status,
		Description: "Task status",
		Options:     []string{"Open", "In Progress", "Done"},
		Extension: map[string]interface{}{
			"id": "rel_123",
		},
	}
	
	details := r.ToDetails()
	
	assert.Equal(t, "custom_status", details.GetString(bundle.RelationKeyRelationKey))
	assert.Equal(t, "Status", details.GetString(bundle.RelationKeyName))
	assert.Equal(t, int64(model.RelationFormat_status), details.GetInt64(bundle.RelationKeyRelationFormat))
	assert.Equal(t, "Task status", details.GetString(bundle.RelationKeyDescription))
	assert.Equal(t, "rel_123", details.GetString(bundle.RelationKeyId))
	assert.NotEmpty(t, details.GetString(bundle.RelationKeyUniqueKey))
}

func TestRelation_FromDetails(t *testing.T) {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyRelationKey, "custom_date")
	details.SetString(bundle.RelationKeyName, "Due Date")
	details.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_date))
	details.SetBool(bundle.RelationKeyRelationFormatIncludeTime, true)
	details.SetString(bundle.RelationKeyDescription, "Task due date")
	details.SetString(bundle.RelationKeyId, "rel_456")
	
	r, err := RelationFromDetails(details)
	require.NoError(t, err)
	
	assert.Equal(t, "custom_date", r.Key)
	assert.Equal(t, "Due Date", r.Name)
	assert.Equal(t, model.RelationFormat_date, r.Format)
	assert.True(t, r.IncludeTime)
	assert.Equal(t, "Task due date", r.Description)
	assert.Equal(t, "rel_456", r.Extension["id"])
}

func TestType_ToDetails(t *testing.T) {
	typ := &Type{
		Key:               "task",
		Name:              "Task",
		Description:       "Task object type",
		PluralName:        "Tasks",
		IconEmoji:         "✅",
		FeaturedRelations: []string{"name", "status", "due_date"},
		RecommendedRelations: []string{"description", "assignee"},
		Extension: map[string]interface{}{
			"id": "type_789",
		},
	}
	
	details := typ.ToDetails()
	
	assert.Equal(t, "Task", details.GetString(bundle.RelationKeyName))
	assert.Equal(t, "Task object type", details.GetString(bundle.RelationKeyDescription))
	assert.Equal(t, "Tasks", details.GetString(bundle.RelationKeyPluralName))
	assert.Equal(t, "✅", details.GetString(bundle.RelationKeyIconEmoji))
	assert.Equal(t, []string{"name", "status", "due_date"}, details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
	assert.Equal(t, []string{"description", "assignee"}, details.GetStringList(bundle.RelationKeyRecommendedRelations))
	assert.Equal(t, "type_789", details.GetString(bundle.RelationKeyId))
	assert.NotEmpty(t, details.GetString(bundle.RelationKeyUniqueKey))
	assert.Equal(t, int64(model.ObjectType_objectType), details.GetInt64(bundle.RelationKeyLayout))
}

func TestType_FromDetails(t *testing.T) {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, "Project")
	details.SetString(bundle.RelationKeyDescription, "Project management")
	details.SetString(bundle.RelationKeyPluralName, "Projects")
	details.SetString(bundle.RelationKeyIconEmoji, "📁")
	details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, []string{"name", "status"})
	details.SetStringList(bundle.RelationKeyRecommendedRelations, []string{"description", "owner"})
	details.SetString(bundle.RelationKeyId, "type_101")
	
	// Create unique key
	uniqueKey, _ := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, "project")
	details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	
	typ, err := TypeFromDetails(details)
	require.NoError(t, err)
	
	assert.Equal(t, "project", typ.Key)
	assert.Equal(t, "Project", typ.Name)
	assert.Equal(t, "Project management", typ.Description)
	assert.Equal(t, "Projects", typ.PluralName)
	assert.Equal(t, "📁", typ.IconEmoji)
	assert.Equal(t, []string{"name", "status"}, typ.FeaturedRelations)
	assert.Equal(t, []string{"description", "owner"}, typ.RecommendedRelations)
	assert.Equal(t, "type_101", typ.Extension["id"])
}

func TestJSONSchemaParser_Parse(t *testing.T) {
	schemaJSON := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"title": "Task",
		"x-type-key": "task",
		"x-icon-emoji": "✅",
		"properties": {
			"id": {
				"type": "string",
				"x-order": 0,
				"x-key": "id"
			},
			"Type": {
				"const": "Task",
				"x-order": 1,
				"x-key": "type"
			},
			"Name": {
				"type": "string",
				"x-featured": true,
				"x-order": 2,
				"x-key": "name"
			},
			"Status": {
				"type": "string",
				"enum": ["Open", "In Progress", "Done"],
				"x-featured": true,
				"x-order": 3,
				"x-key": "status",
				"x-format": "RelationFormat_status"
			},
			"Due Date": {
				"type": "string",
				"format": "date-time",
				"x-order": 4,
				"x-key": "due_date"
			}
		}
	}`
	
	parser := NewJSONSchemaParser()
	schema, err := parser.Parse(strings.NewReader(schemaJSON))
	require.NoError(t, err)
	
	// Check type
	typ := schema.GetType()
	require.NotNil(t, typ)
	assert.Equal(t, "Task", typ.Name)
	assert.Equal(t, "✅", typ.IconEmoji)
	assert.Contains(t, typ.FeaturedRelations, "name")
	assert.Contains(t, typ.FeaturedRelations, "status")
	
	// Check relations
	statusRel, ok := schema.GetRelation("status")
	require.True(t, ok)
	assert.Equal(t, "Status", statusRel.Name)
	assert.Equal(t, model.RelationFormat_status, statusRel.Format)
	assert.Equal(t, []string{"Open", "In Progress", "Done"}, statusRel.Options)
	
	dateRel, ok := schema.GetRelation("due_date")
	require.True(t, ok)
	assert.Equal(t, "Due Date", dateRel.Name)
	assert.Equal(t, model.RelationFormat_date, dateRel.Format)
	assert.True(t, dateRel.IncludeTime)
}

func TestJSONSchemaExporter_Export(t *testing.T) {
	// Create schema
	schema := NewSchema()
	
	// Add type
	typ := &Type{
		Key:               "task",
		Name:              "Task",
		IconEmoji:         "✅",
		FeaturedRelations: []string{"name", "status"},
		RecommendedRelations: []string{"due_date"},
	}
	schema.SetType(typ)
	
	// Add relations
	nameRel := &Relation{
		Key:    "name",
		Name:   "Name",
		Format: model.RelationFormat_shorttext,
	}
	schema.AddRelation(nameRel)
	
	statusRel := &Relation{
		Key:     "status",
		Name:    "Status",
		Format:  model.RelationFormat_status,
		Options: []string{"Open", "Done"},
	}
	schema.AddRelation(statusRel)
	
	dateRel := &Relation{
		Key:         "due_date",
		Name:        "Due Date",
		Format:      model.RelationFormat_date,
		IncludeTime: true,
	}
	schema.AddRelation(dateRel)
	
	// Export
	var buf bytes.Buffer
	exporter := NewJSONSchemaExporter("  ")
	err := exporter.Export(schema, &buf)
	require.NoError(t, err)
	
	output := buf.String()
	
	// Check output contains expected elements
	assert.Contains(t, output, `"title": "Task"`)
	assert.Contains(t, output, `"x-type-key": "task"`)
	assert.Contains(t, output, `"x-icon-emoji": "✅"`)
	assert.Contains(t, output, `"x-featured": true`)
	assert.Contains(t, output, `"Open"`)
	assert.Contains(t, output, `"Done"`)
	assert.Contains(t, output, `"format": "date-time"`)
	assert.Contains(t, output, `"x-format": "RelationFormat_status"`)
}

func TestSchema_Merge(t *testing.T) {
	// Create first schema
	schema1 := NewSchema()
	typ1 := &Type{
		Key:               "task",
		Name:              "Task",
		FeaturedRelations: []string{"name"},
	}
	schema1.SetType(typ1)
	
	rel1 := &Relation{
		Key:    "name",
		Name:   "Name",
		Format: model.RelationFormat_shorttext,
	}
	schema1.AddRelation(rel1)
	
	// Create second schema
	schema2 := NewSchema()
	typ2 := &Type{
		Key:                  "task",
		Name:                 "Task",
		RecommendedRelations: []string{"status"},
	}
	schema2.SetType(typ2)
	
	rel2 := &Relation{
		Key:     "status",
		Name:    "Status",
		Format:  model.RelationFormat_status,
		Options: []string{"Open", "Done"},
	}
	schema2.AddRelation(rel2)
	
	// Merge
	err := schema1.Merge(schema2)
	require.NoError(t, err)
	
	// Check merged schema
	typ := schema1.GetType()
	require.NotNil(t, typ)
	assert.Contains(t, typ.FeaturedRelations, "name")
	assert.Contains(t, typ.RecommendedRelations, "status")
	
	_, ok := schema1.GetRelation("name")
	assert.True(t, ok)
	
	statusRel, ok := schema1.GetRelation("status")
	require.True(t, ok)
	assert.Equal(t, []string{"Open", "Done"}, statusRel.Options)
}

func TestRelation_CreateOptionDetails(t *testing.T) {
	r := &Relation{
		Key:    "priority",
		Name:   "Priority",
		Format: model.RelationFormat_status,
	}
	
	details := r.CreateOptionDetails("High", "red")
	
	assert.Equal(t, "High", details.GetString(bundle.RelationKeyName))
	assert.Equal(t, "priority", details.GetString(bundle.RelationKeyRelationKey))
	assert.Equal(t, "red", details.GetString(bundle.RelationKeyRelationOptionColor))
	assert.Equal(t, int64(model.ObjectType_relationOption), details.GetInt64(bundle.RelationKeyLayout))
	assert.NotEmpty(t, details.GetString(bundle.RelationKeyUniqueKey))
}