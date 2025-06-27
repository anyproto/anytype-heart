package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		Key:                  "task",
		Name:                 "Task",
		Description:          "Task object type",
		PluralName:           "Tasks",
		IconEmoji:            "✅",
		FeaturedRelations:    []string{"project", "status", "due_date"},
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
	assert.Equal(t, []string{"project", "status", "due_date"}, details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
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

func TestJSONSchemaParser_BSONKeyGeneration(t *testing.T) {
	t.Run("generates BSON ID for type without x-type-key", func(t *testing.T) {
		schemaJSON := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "My Custom Type",
			"properties": {
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-format": "shorttext"
				}
			}
		}`

		parser := NewJSONSchemaParser()
		schema, err := parser.Parse(bytes.NewReader([]byte(schemaJSON)))
		require.NoError(t, err)
		require.NotNil(t, schema)
		require.NotNil(t, schema.Type)

		// Type key should be a generated BSON ID (24 characters)
		assert.Len(t, schema.Type.Key, 24, "Type key should be a BSON ID (24 characters)")
		assert.Regexp(t, "^[0-9a-f]{24}$", schema.Type.Key, "Type key should be a valid BSON ID")
		assert.Equal(t, "My Custom Type", schema.Type.Name)
	})

	t.Run("generates BSON ID for properties without x-key", func(t *testing.T) {
		schemaJSON := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "Test Type",
			"x-type-key": "test_type",
			"properties": {
				"Title": {
					"type": "string",
					"x-format": "shorttext"
				},
				"Description": {
					"type": "string",
					"x-format": "longtext",
					"x-key": "desc"
				}
			}
		}`

		parser := NewJSONSchemaParser()
		schema, err := parser.Parse(bytes.NewReader([]byte(schemaJSON)))
		require.NoError(t, err)
		require.NotNil(t, schema)

		// Check that we have 2 relations
		assert.Len(t, schema.Relations, 2)

		// Find the relations
		var titleRel, descRel *Relation
		for _, rel := range schema.Relations {
			if rel.Name == "Title" {
				titleRel = rel
			} else if rel.Name == "Description" {
				descRel = rel
			}
		}

		require.NotNil(t, titleRel, "Title relation should exist")
		require.NotNil(t, descRel, "Description relation should exist")

		// Title should have a generated BSON ID (24 characters)
		assert.Len(t, titleRel.Key, 24, "Title key should be a BSON ID (24 characters)")
		assert.Regexp(t, "^[0-9a-f]{24}$", titleRel.Key, "Title key should be a valid BSON ID")

		// Description should have the specified key
		assert.Equal(t, "desc", descRel.Key, "Description should use the x-key value")
	})

	t.Run("preserves specified x-type-key", func(t *testing.T) {
		schemaJSON := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "Task",
			"x-type-key": "custom_task_key",
			"properties": {
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-format": "shorttext"
				}
			}
		}`

		parser := NewJSONSchemaParser()
		schema, err := parser.Parse(bytes.NewReader([]byte(schemaJSON)))
		require.NoError(t, err)
		require.NotNil(t, schema)
		require.NotNil(t, schema.Type)

		// Type key should use the specified x-type-key
		assert.Equal(t, "custom_task_key", schema.Type.Key)
		assert.Equal(t, "Task", schema.Type.Name)
	})
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
				"x-format": "status"
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
		Key:                  "task",
		Name:                 "Task",
		IconEmoji:            "✅",
		FeaturedRelations:    []string{"name", "status"},
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
	assert.Contains(t, output, `"x-format": "status"`)

	// Check version is included
	assert.Contains(t, output, `"x-schema-version": "1.0"`)
	assert.Contains(t, output, `:ver-1.0"`) // Check version in $id
}

func TestJSONSchemaExporter_VersionInOutput(t *testing.T) {
	// Create a simple schema
	schema := NewSchema()
	typ := &Type{
		Key:  "document",
		Name: "Document",
	}
	schema.SetType(typ)

	// Export
	var buf bytes.Buffer
	exporter := NewJSONSchemaExporter("  ")
	err := exporter.Export(schema, &buf)
	require.NoError(t, err)

	// Parse the output as JSON to verify structure
	var jsonOutput map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &jsonOutput)
	require.NoError(t, err)

	// Check x-schema-version field
	genVersion, ok := jsonOutput["x-schema-version"].(string)
	assert.True(t, ok, "x-schema-version should be a string")
	assert.Equal(t, "1.0", genVersion)

	// Check version in $id
	schemaId, ok := jsonOutput["$id"].(string)
	assert.True(t, ok, "$id should be a string")
	assert.Contains(t, schemaId, ":ver-1.0")
	assert.True(t, strings.HasSuffix(schemaId, ":ver-1.0"), "Schema ID should end with :ver-1.0")
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

func TestSchema_RelationProperty_XFormat(t *testing.T) {
	// Test that x-format is added to property schemas using the schema package

	// Test file format
	fileRel := &Relation{
		Key:    "attachments",
		Name:   "Attachments",
		Format: model.RelationFormat_file,
	}

	s := NewSchema()
	s.AddRelation(fileRel)

	typ := &Type{
		Key:               "test",
		Name:              "Test",
		FeaturedRelations: []string{"attachments"},
	}
	s.SetType(typ)

	// Export and parse JSON to check structure
	exporter := NewJSONSchemaExporter("  ")
	var buf bytes.Buffer
	err := exporter.Export(s, &buf)
	require.NoError(t, err)

	var jsonSchema map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &jsonSchema)
	require.NoError(t, err)

	properties := jsonSchema["properties"].(map[string]interface{})
	fileProp := properties["Attachments"].(map[string]interface{})

	assert.Equal(t, "file", fileProp["x-format"])
	assert.Equal(t, "attachments", fileProp["x-key"])
	assert.Equal(t, "string", fileProp["type"])
	assert.Contains(t, fileProp["description"].(string), "Path to the file")

	// Test tag format
	tagRel := &Relation{
		Key:    "tags",
		Name:   "Tags",
		Format: model.RelationFormat_tag,
	}

	s2 := NewSchema()
	s2.AddRelation(tagRel)

	typ2 := &Type{
		Key:               "test2",
		Name:              "Test2",
		FeaturedRelations: []string{"tags"},
	}
	s2.SetType(typ2)

	// Export and parse JSON
	var buf2 bytes.Buffer
	err = exporter.Export(s2, &buf2)
	require.NoError(t, err)

	var jsonSchema2 map[string]interface{}
	err = json.Unmarshal(buf2.Bytes(), &jsonSchema2)
	require.NoError(t, err)

	properties2 := jsonSchema2["properties"].(map[string]interface{})
	tagProp := properties2["Tags"].(map[string]interface{})

	assert.Equal(t, "tag", tagProp["x-format"])
	assert.Equal(t, "tags", tagProp["x-key"])
	assert.Equal(t, "array", tagProp["type"])

	// Test object format
	objRel := &Relation{
		Key:    "assignee",
		Name:   "Assignee",
		Format: model.RelationFormat_object,
	}

	s3 := NewSchema()
	s3.AddRelation(objRel)

	typ3 := &Type{
		Key:               "test3",
		Name:              "Test3",
		FeaturedRelations: []string{"assignee"},
	}
	s3.SetType(typ3)

	// Export and parse JSON
	var buf3 bytes.Buffer
	err = exporter.Export(s3, &buf3)
	require.NoError(t, err)

	var jsonSchema3 map[string]interface{}
	err = json.Unmarshal(buf3.Bytes(), &jsonSchema3)
	require.NoError(t, err)

	properties3 := jsonSchema3["properties"].(map[string]interface{})
	objProp := properties3["Assignee"].(map[string]interface{})

	assert.Equal(t, "object", objProp["x-format"])
	assert.Equal(t, "assignee", objProp["x-key"])
	assert.Equal(t, "array", objProp["type"])

	// Verify object relation has proper items schema
	items := objProp["items"].(map[string]interface{})
	assert.Equal(t, "string", items["type"])
}

func TestSchema_AllFormatsHaveXFormat(t *testing.T) {
	// Test all relation formats get x-format using the schema package
	formats := []model.RelationFormat{
		model.RelationFormat_shorttext,
		model.RelationFormat_longtext,
		model.RelationFormat_number,
		model.RelationFormat_checkbox,
		model.RelationFormat_date,
		model.RelationFormat_tag,
		model.RelationFormat_status,
		model.RelationFormat_email,
		model.RelationFormat_url,
		model.RelationFormat_phone,
		model.RelationFormat_file,
		model.RelationFormat_object,
	}

	exporter := NewJSONSchemaExporter("  ")

	// Test each format
	for i, format := range formats {
		key := fmt.Sprintf("rel_%d", i)

		// Create relation
		rel := &Relation{
			Key:    key,
			Name:   fmt.Sprintf("Relation %d", i),
			Format: format,
		}

		s := NewSchema()
		s.AddRelation(rel)

		typ := &Type{
			Key:               fmt.Sprintf("test_%d", i),
			Name:              fmt.Sprintf("Test %d", i),
			FeaturedRelations: []string{key},
		}
		s.SetType(typ)

		// Export and parse JSON
		var buf bytes.Buffer
		err := exporter.Export(s, &buf)
		require.NoError(t, err)

		var jsonSchema map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &jsonSchema)
		require.NoError(t, err)

		properties := jsonSchema["properties"].(map[string]interface{})
		relName := fmt.Sprintf("Relation %d", i)
		prop := properties[relName].(map[string]interface{})

		// All properties should have x-format
		assert.Equal(t, format.String(), prop["x-format"], "Format %s should have x-format", format.String())
		assert.Equal(t, key, prop["x-key"], "Format %s should have x-key", format.String())
	}
}
