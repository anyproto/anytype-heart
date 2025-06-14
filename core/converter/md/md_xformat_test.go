package md

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestMD_JSONSchemaProperty_XFormat(t *testing.T) {
	// Test that x-format is added to property schemas
	
	h := &MD{}
	
	// Test file format
	fileDetails := domain.NewDetails()
	fileDetails.SetString(bundle.RelationKeyRelationKey, "attachments")
	fileDetails.SetString(bundle.RelationKeyName, "Attachments")
	
	fileProp := h.getJSONSchemaProperty(fileDetails, model.RelationFormat_file, "")
	assert.Equal(t, "RelationFormat_file", fileProp["x-format"])
	assert.Equal(t, "attachments", fileProp["x-key"])
	assert.Equal(t, "string", fileProp["type"])
	assert.Contains(t, fileProp["description"].(string), "Path to the file")
	
	// Test tag format
	tagDetails := domain.NewDetails()
	tagDetails.SetString(bundle.RelationKeyRelationKey, "tags")
	tagDetails.SetString(bundle.RelationKeyName, "Tags")
	
	tagProp := h.getJSONSchemaProperty(tagDetails, model.RelationFormat_tag, "")
	assert.Equal(t, "RelationFormat_tag", tagProp["x-format"])
	assert.Equal(t, "tags", tagProp["x-key"])
	assert.Equal(t, "array", tagProp["type"])
	
	// Test object format
	objDetails := domain.NewDetails()
	objDetails.SetString(bundle.RelationKeyRelationKey, "assignee")
	objDetails.SetString(bundle.RelationKeyName, "Assignee")
	
	objProp := h.getJSONSchemaProperty(objDetails, model.RelationFormat_object, "")
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

func TestMD_JSONSchema_AllFormatsHaveXFormat(t *testing.T) {
	// Test all relation formats get x-format
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
	
	h := &MD{}
	
	// Test each format
	for i, format := range formats {
		key := fmt.Sprintf("rel_%d", i)
		
		// Create relation details
		details := domain.NewDetails()
		details.SetString(bundle.RelationKeyRelationKey, key)
		details.SetString(bundle.RelationKeyName, fmt.Sprintf("Relation %d", i))
		
		prop := h.getJSONSchemaProperty(details, format, "")
		
		// All properties should have x-format
		assert.Equal(t, "RelationFormat_"+format.String(), prop["x-format"], "Format %s should have x-format", format.String())
		assert.Equal(t, key, prop["x-key"], "Format %s should have x-key", format.String())
	}
}