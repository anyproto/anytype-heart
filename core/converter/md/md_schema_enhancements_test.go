package md

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestMD_GenerateJSONSchema_WithEnhancements(t *testing.T) {
	// Create test state
	st := state.NewDoc("root", nil).NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("test-object"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("test-type"))

	// Create mock resolver
	resolver := &testResolver{
		types: map[string]*domain.Details{
			"test-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:          domain.String("test-type"),
				bundle.RelationKeyName:        domain.String("Task"),
				bundle.RelationKeyUniqueKey:   domain.String("ot-task"), // UniqueKey for TypeKey extraction
				bundle.RelationKeyDescription: domain.String("Task management object"),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-name", "rel-status"}),
				bundle.RelationKeyRecommendedRelations:         domain.StringList([]string{"rel-desc"}),
			}),
		},
		relations: map[string]*domain.Details{
			"rel-name": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-name"),
				bundle.RelationKeyRelationKey:    domain.String("name"),
				bundle.RelationKeyName:           domain.String("Name"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			}),
			"rel-status": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-status"),
				bundle.RelationKeyRelationKey:    domain.String("status"),
				bundle.RelationKeyName:           domain.String("Status"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			}),
			"rel-desc": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-desc"),
				bundle.RelationKeyRelationKey:    domain.String("description"),
				bundle.RelationKeyName:           domain.String("Description"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
			}),
			"rel-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-type"),
				bundle.RelationKeyRelationKey:    domain.String("type"),
				bundle.RelationKeyName:           domain.String("Type"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			}),
		},
	}
	resolver.keyMapping = map[string]string{
		"type": "rel-type",
	}

	// Create converter
	conv := NewMDConverterWithResolver(st, &testFileNamer{}, true, true, resolver).(*MD)

	// Generate schema
	schemaBytes, err := conv.GenerateJSONSchema()
	require.NoError(t, err)
	require.NotNil(t, schemaBytes)

	// Parse schema
	var schema map[string]interface{}
	err = json.Unmarshal(schemaBytes, &schema)
	require.NoError(t, err)

	// Verify x-type-key is present
	assert.Equal(t, "task", schema["x-type-key"])

	// Verify properties
	properties := schema["properties"].(map[string]interface{})

	// Check id property exists
	idProp := properties["id"].(map[string]interface{})
	assert.Equal(t, "string", idProp["type"])
	assert.Equal(t, "Unique identifier of the Anytype object", idProp["description"])
	assert.Equal(t, true, idProp["readOnly"])
	assert.Equal(t, float64(0), idProp["x-order"]) // JSON numbers are float64

	// Check Type property comes first (after id)
	typeProp := properties["Type"].(map[string]interface{})
	assert.Equal(t, float64(1), typeProp["x-order"]) // Type is always first after id

	// Check featured properties have x-featured and correct order
	nameProp := properties["Name"].(map[string]interface{})
	assert.Equal(t, true, nameProp["x-featured"])
	assert.Equal(t, float64(2), nameProp["x-order"]) // Second property after Type

	statusProp := properties["Status"].(map[string]interface{})
	assert.Equal(t, true, statusProp["x-featured"])
	assert.Equal(t, float64(3), statusProp["x-order"]) // Third property

	// Check non-featured property doesn't have x-featured but has order
	descProp := properties["Description"].(map[string]interface{})
	_, hasFeatured := descProp["x-featured"]
	assert.False(t, hasFeatured, "Non-featured property should not have x-featured")
	assert.Equal(t, float64(4), descProp["x-order"]) // Fourth property

	// Verify required array is not present (since we don't add anything to it)
	_, hasRequired := schema["required"]
	assert.False(t, hasRequired, "Schema should not have required array when no properties are required")
}

func TestMD_RenderProperties_WithID(t *testing.T) {
	// Create test state with a block
	rootBlock := &model.Block{
		Id:          "root",
		ChildrenIds: []string{"text1"},
	}
	textBlock := &model.Block{
		Id: "text1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "Test content",
			},
		},
	}
	st := state.NewDoc("root", map[string]simple.Block{
		"root":  simple.New(rootBlock),
		"text1": simple.New(textBlock),
	}).NewState()

	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("obj-123-456"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("test-type"))
	st.SetDetail(domain.RelationKey("name"), domain.String("My Task"))

	// Create mock resolver
	resolver := &testResolver{
		types: map[string]*domain.Details{
			"test-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("test-type"),
				bundle.RelationKeyName: domain.String("Task"),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-name"}),
			}),
		},
		relations: map[string]*domain.Details{
			"rel-name": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-name"),
				bundle.RelationKeyRelationKey:    domain.String("name"),
				bundle.RelationKeyName:           domain.String("Name"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			}),
		},
	}
	resolver.keyMapping = map[string]string{
		"type": "rel-type",
		"name": "rel-name",
	}

	// Create converter with schema
	conv := NewMDConverterWithResolver(st, &testFileNamer{}, true, true, resolver)

	// Convert to markdown
	result := conv.Convert(model.SmartBlockType_Page)
	resultStr := string(result)

	// Verify ID is rendered in YAML front matter
	assert.Contains(t, resultStr, "id: obj-123-456")
	
	// Verify it comes after schema reference but before other properties
	lines := strings.Split(resultStr, "\n")
	var schemaLine, idLine, nameLine int
	for i, line := range lines {
		if strings.Contains(line, "# yaml-language-server:") {
			schemaLine = i
		}
		if strings.Contains(line, "id: obj-123-456") {
			idLine = i
		}
		if strings.Contains(line, "Name: My Task") {
			nameLine = i
		}
	}
	
	assert.Greater(t, idLine, schemaLine, "ID should come after schema reference")
	assert.Less(t, idLine, nameLine, "ID should come before other properties")
}

func TestMD_GenerateJSONSchema_PropertyOrder(t *testing.T) {
	// Create test state
	st := state.NewDoc("root", nil).NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("test-object"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("test-type"))

	// Create mock resolver with multiple properties
	resolver := &testResolver{
		types: map[string]*domain.Details{
			"test-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:          domain.String("test-type"),
				bundle.RelationKeyName:        domain.String("Complex Type"),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-1", "rel-2"}),
				bundle.RelationKeyRecommendedRelations:         domain.StringList([]string{"rel-3", "rel-4", "rel-5"}),
			}),
		},
		relations: map[string]*domain.Details{
			"rel-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-type"),
				bundle.RelationKeyRelationKey:    domain.String("type"),
				bundle.RelationKeyName:           domain.String("Type"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			}),
			"rel-1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-1"),
				bundle.RelationKeyRelationKey:    domain.String("prop1"),
				bundle.RelationKeyName:           domain.String("Property 1"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			}),
			"rel-2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-2"),
				bundle.RelationKeyRelationKey:    domain.String("prop2"),
				bundle.RelationKeyName:           domain.String("Property 2"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
			}),
			"rel-3": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-3"),
				bundle.RelationKeyRelationKey:    domain.String("prop3"),
				bundle.RelationKeyName:           domain.String("Property 3"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
			}),
			"rel-4": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-4"),
				bundle.RelationKeyRelationKey:    domain.String("prop4"),
				bundle.RelationKeyName:           domain.String("Property 4"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_checkbox)),
			}),
			"rel-5": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-5"),
				bundle.RelationKeyRelationKey:    domain.String("prop5"),
				bundle.RelationKeyName:           domain.String("Property 5"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
			}),
		},
	}
	resolver.keyMapping = map[string]string{
		"type": "rel-type",
	}

	// Create converter
	conv := NewMDConverterWithResolver(st, &testFileNamer{}, true, true, resolver).(*MD)

	// Generate schema
	schemaBytes, err := conv.GenerateJSONSchema()
	require.NoError(t, err)

	// Parse schema
	var schema map[string]interface{}
	err = json.Unmarshal(schemaBytes, &schema)
	require.NoError(t, err)

	properties := schema["properties"].(map[string]interface{})

	// Verify order of all properties
	expectedOrder := map[string]float64{
		"id":         0,
		"Type":       1,
		"Property 1": 2, // Featured properties come first
		"Property 2": 3,
		"Property 3": 4, // Regular properties follow
		"Property 4": 5,
		"Property 5": 6,
	}

	for propName, expectedPos := range expectedOrder {
		prop, exists := properties[propName].(map[string]interface{})
		assert.True(t, exists, "Property %s should exist", propName)
		assert.Equal(t, expectedPos, prop["x-order"], "Property %s should have order %v", propName, expectedPos)
	}

	// Verify all properties have x-order
	for propName, propValue := range properties {
		prop := propValue.(map[string]interface{})
		_, hasOrder := prop["x-order"]
		assert.True(t, hasOrder, "Property %s should have x-order", propName)
	}
}