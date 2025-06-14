package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

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
	
	assert.Equal(t, "RelationFormat_file", fileProp["x-format"])
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
	
	assert.Equal(t, "RelationFormat_tag", tagProp["x-format"])
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
	
	assert.Equal(t, "RelationFormat_object", objProp["x-format"])
	assert.Equal(t, "assignee", objProp["x-key"])
	assert.Equal(t, "array", objProp["type"])
	
	// Verify object relation has proper items schema
	items := objProp["items"].(map[string]interface{})
	assert.Equal(t, "object", items["type"])
	itemProps := items["properties"].(map[string]interface{})
	assert.Contains(t, itemProps, "Name")
	assert.Contains(t, itemProps, "File")
	assert.Contains(t, itemProps, "Id")
	assert.Contains(t, itemProps, "Object type")
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
		assert.Equal(t, "RelationFormat_"+format.String(), prop["x-format"], "Format %s should have x-format", format.String())
		assert.Equal(t, key, prop["x-key"], "Format %s should have x-key", format.String())
	}
}