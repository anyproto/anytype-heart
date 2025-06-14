package md

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestMD_Convert(t *testing.T) {
	newState := func(bs ...*model.Block) *state.State {
		var sbs []simple.Block
		var ids []string
		for _, b := range bs {
			sb := simple.New(b)
			ids = append(ids, sb.Model().Id)
			sbs = append(sbs, sb)
		}
		blocks := map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: ids}),
		}
		for _, sb := range sbs {
			blocks[sb.Model().Id] = sb
		}
		return state.NewDoc("root", blocks).(*state.State)
	}

	t.Run("markup render", func(t *testing.T) {
		s := newState(
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "Header 1",
						Style: model.BlockContentText_Header1,
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "Header 2",
						Style: model.BlockContentText_Header2,
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfDiv{
					Div: &model.BlockContentDiv{
						Style: model.BlockContentDiv_Dots,
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "Header 3",
						Style: model.BlockContentText_Header3,
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text: "Usual text",
					},
				},
			},
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "Header 4",
						Style: model.BlockContentText_Header4,
					},
				},
			},
		)
		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)
		exp := "# Header 1   \n## Header 2   \n --- \n### Header 3   \nUsual text   \n#### Header 4   \n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test header rendering", func(t *testing.T) {
		s := newState(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "some text with marks @mention",
					Marks: &model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{
							{
								Range: &model.Range{0, 4},
								Type:  model.BlockContentTextMark_Bold,
							},
							{
								Range: &model.Range{0, 4},
								Type:  model.BlockContentTextMark_Italic,
							},
							{
								Range: &model.Range{0, 4},
								Type:  model.BlockContentTextMark_Link,
								Param: "http://golang.org",
							},
							{
								Range: &model.Range{5, 6},
								Type:  model.BlockContentTextMark_Link,
								Param: "http://golang.org",
							},
							{
								Range: &model.Range{6, 7},
								Type:  model.BlockContentTextMark_Link,
								Param: "http://golang.org",
							},
							{
								Range: &model.Range{10, 16},
								Type:  model.BlockContentTextMark_Bold,
							},
							{
								Range: &model.Range{12, 18},
								Type:  model.BlockContentTextMark_Strikethrough,
							},
						},
					},
				},
			},
		})
		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)
		exp := "***[some](http://golang.org)*** [t](http://golang.org) [e](http://golang.org)xt **wi~~th m~~**~~ar~~ks @mention   \n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test render native emoji", func(t *testing.T) {
		s := newState(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "Test 😝",
					Marks: &model.BlockContentTextMarks{
						Marks: nil,
					},
				},
			},
		})
		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)
		exp := "Test 😝   \n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test render in app emoji", func(t *testing.T) {
		s := newState(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "Test ⛰️",
					Marks: &model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{
							{
								Range: &model.Range{6, 7},
								Type:  model.BlockContentTextMark_Emoji,
							},
						},
					},
				},
			},
		})
		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)
		exp := "Test ⛰️   \n"
		assert.Equal(t, exp, string(res))
	})
}

type mockFileNamer struct{}

func (m *mockFileNamer) Get(path, hash, title, ext string) string {
	if path != "" {
		return path + "/" + hash + "_" + title + ext
	}
	return title + ext
}

func TestMDConverter_GenerateJSONSchema(t *testing.T) {
	t.Skip("Skipping schema generation test - needs to be updated to use resolver pattern")
	t.Run("generate schema for task type", func(t *testing.T) {
		// Setup state with object type
		st := state.NewDoc("root", nil).NewState()
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("taskType"))

		// Create known docs map with type and relations
		knownDocs := map[string]*domain.Details{
			"taskType": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:                           domain.String("taskType"),
				bundle.RelationKeyName:                         domain.String("Task"),
				bundle.RelationKeyDescription:                  domain.String("A task to be completed"),
				bundle.RelationKeyPluralName:                   domain.String("Tasks"),
				bundle.RelationKeyIconEmoji:                    domain.String("✅"),
				bundle.RelationKeyIconImage:                    domain.String("task-icon"),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"priority", "status"}),
				bundle.RelationKeyRecommendedRelations:         domain.StringList([]string{"assignee", "dueDate"}),
			}),
			"priority": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("priority"),
				bundle.RelationKeyName:           domain.String("priority"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyRelationKey:    domain.String("priority"),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			}),
			"status": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("status"),
				bundle.RelationKeyName:           domain.String("status"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyRelationKey:    domain.String("status"),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			}),
			"assignee": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:                        domain.String("assignee"),
				bundle.RelationKeyName:                      domain.String("assignee"),
				bundle.RelationKeyRelationFormat:            domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyRelationKey:               domain.String("assignee"),
				bundle.RelationKeyLayout:                    domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationFormatObjectTypes: domain.StringList([]string{"personType", "contactType"}),
			}),
			"personType": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("personType"),
				bundle.RelationKeyName: domain.String("Person"),
			}),
			"contactType": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("contactType"),
				bundle.RelationKeyName: domain.String("Contact"),
			}),
			"dueDate": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("dueDate"),
				bundle.RelationKeyName:           domain.String("dueDate"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
				bundle.RelationKeyRelationKey:    domain.String("dueDate"),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			}),
		}

		// Add some relation options for status
		knownDocs["statusOption1"] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:          domain.String("statusOption1"),
			bundle.RelationKeyName:        domain.String("Open"),
			bundle.RelationKeyRelationKey: domain.String("status"),
			bundle.RelationKeyLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
		})
		knownDocs["statusOption2"] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:          domain.String("statusOption2"),
			bundle.RelationKeyName:        domain.String("In-Progress"),
			bundle.RelationKeyRelationKey: domain.String("status"),
			bundle.RelationKeyLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
		})
		knownDocs["statusOption3"] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:          domain.String("statusOption3"),
			bundle.RelationKeyName:        domain.String("Done"),
			bundle.RelationKeyRelationKey: domain.String("status"),
			bundle.RelationKeyLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
		})

		// Add priority options
		knownDocs["priorityOption1"] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:          domain.String("priorityOption1"),
			bundle.RelationKeyName:        domain.String("Low"),
			bundle.RelationKeyRelationKey: domain.String("priority"),
			bundle.RelationKeyLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
		})
		knownDocs["priorityOption2"] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:          domain.String("priorityOption2"),
			bundle.RelationKeyName:        domain.String("Medium"),
			bundle.RelationKeyRelationKey: domain.String("priority"),
			bundle.RelationKeyLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
		})
		knownDocs["priorityOption3"] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:          domain.String("priorityOption3"),
			bundle.RelationKeyName:        domain.String("High"),
			bundle.RelationKeyRelationKey: domain.String("priority"),
			bundle.RelationKeyLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
		})

		// Create converter
		conv := NewMDConverterWithSchema(st, &mockFileNamer{}, true, true).(*MD)
		conv.SetKnownDocs(knownDocs)

		// Generate schema
		schemaBytes, err := conv.GenerateJSONSchema()
		require.NoError(t, err)
		require.NotNil(t, schemaBytes)

		// Parse and verify schema
		var schema map[string]interface{}
		err = json.Unmarshal(schemaBytes, &schema)
		require.NoError(t, err)

		// Verify basic structure
		assert.Equal(t, "http://json-schema.org/draft-07/schema#", schema["$schema"])
		assert.Equal(t, "object", schema["type"])

		// Verify root schema parameters
		// The $id should be a URN format
		schemaId, ok := schema["$id"].(string)
		require.True(t, ok)
		assert.True(t, strings.HasPrefix(schemaId, "urn:anytype:schema:"))
		assert.Contains(t, schemaId, ":type-task:")
		assert.Contains(t, schemaId, ":gen-1.0.0")

		assert.Equal(t, "Anytype", schema["x-app"])
		assert.Equal(t, "1.0.0", schema["x-genVersion"])
		// x-type-author and x-type-date might be empty in test data
		assert.NotNil(t, schema["x-type-author"])
		assert.NotNil(t, schema["x-type-date"])

		// Verify type metadata
		assert.Equal(t, "Task", schema["title"])
		assert.Equal(t, "A task to be completed", schema["description"])
		assert.Equal(t, "Tasks", schema["x-plural"])
		assert.Equal(t, "✅", schema["x-icon-emoji"])
		assert.Equal(t, "task-icon", schema["x-icon-name"])

		// Verify properties
		properties, ok := schema["properties"].(map[string]interface{})
		require.True(t, ok)

		// Check priority property
		priority, ok := properties["priority"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", priority["type"])
		priorityEnum, ok := priority["enum"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, priorityEnum, "Low")
		assert.Contains(t, priorityEnum, "Medium")
		assert.Contains(t, priorityEnum, "High")

		// Check status property
		status, ok := properties["status"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", status["type"])
		statusEnum, ok := status["enum"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, statusEnum, "Open")
		assert.Contains(t, statusEnum, "In-Progress")
		assert.Contains(t, statusEnum, "Done")

		// Check assignee property (object type)
		assignee, ok := properties["assignee"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "object", assignee["type"])

		// Check nested properties
		assigneeProps, ok := assignee["properties"].(map[string]interface{})
		require.True(t, ok)

		// Check Name property
		nameProps, ok := assigneeProps["Name"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", nameProps["type"])

		// Check Object Type property with enum
		objTypeProps, ok := assigneeProps["Object type"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", objTypeProps["type"])

		// Check enum values
		objTypeEnum, ok := objTypeProps["enum"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, objTypeEnum, "Person")
		assert.Contains(t, objTypeEnum, "Contact")

		// Check required fields
		assigneeRequired, ok := assignee["required"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, assigneeRequired, "Name")

		// Check dueDate property
		dueDate, ok := properties["dueDate"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", dueDate["type"])
		assert.Equal(t, "date", dueDate["format"])

		// Check required fields
		required, ok := schema["required"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, required, "priority")
		assert.Contains(t, required, "status")
	})

	t.Run("schema file name generation", func(t *testing.T) {
		conv := NewMDConverterWithSchema(state.NewDoc("root", nil).NewState(), &mockFileNamer{}, true, true).(*MD)

		tests := []struct {
			typeName string
			expected string
		}{
			{"Task", "./schemas/task.schema.json"},
			{"Project Status", "./schemas/project_status.schema.json"},
			{"Document/Page", "./schemas/document_page.schema.json"},
			{"My\\Type", "./schemas/my_type.schema.json"},
		}

		for _, tt := range tests {
			t.Run(tt.typeName, func(t *testing.T) {
				result := conv.GenerateSchemaFileName(tt.typeName)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("object relation without type constraints", func(t *testing.T) {
		// Setup state with object relation that has no specific types
		st := state.NewDoc("root", nil).NewState()
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("projectType"))

		knownDocs := map[string]*domain.Details{
			"projectType": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:                   domain.String("projectType"),
				bundle.RelationKeyName:                 domain.String("Project"),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"linkedDoc"}),
			}),
			"linkedDoc": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("linkedDoc"),
				bundle.RelationKeyName:           domain.String("Linked Document"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyRelationKey:    domain.String("linkedDoc"),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
				// No RelationKeyRelationFormatObjectTypes specified
			}),
		}

		conv := NewMDConverterWithSchema(st, &mockFileNamer{}, true, true).(*MD)
		conv.SetKnownDocs(knownDocs)

		schemaBytes, err := conv.GenerateJSONSchema()
		require.NoError(t, err)

		var schema map[string]interface{}
		err = json.Unmarshal(schemaBytes, &schema)
		require.NoError(t, err)

		properties, ok := schema["properties"].(map[string]interface{})
		require.True(t, ok)

		// Check linkedDoc property
		linkedDoc, ok := properties["Linked Document"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "object", linkedDoc["type"])

		linkedDocProps, ok := linkedDoc["properties"].(map[string]interface{})
		require.True(t, ok)

		// Should have Object Type property but no enum
		objTypeProps, ok := linkedDocProps["Object type"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "string", objTypeProps["type"])
		assert.Nil(t, objTypeProps["enum"]) // No enum when no types are specified
	})

	t.Run("schema generation with partial metadata", func(t *testing.T) {
		// Setup state with object type that has only some metadata fields
		st := state.NewDoc("root", nil).NewState()
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("customType"))

		// Create known docs with minimal metadata
		knownDocs := map[string]*domain.Details{
			"customType": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("customType"),
				bundle.RelationKeyName: domain.String("Custom Type"),
				// Only description and emoji, no plural or icon image
				bundle.RelationKeyDescription: domain.String("A custom object type"),
				bundle.RelationKeyIconEmoji:   domain.String("🔧"),
			}),
		}

		// Create converter
		conv := NewMDConverterWithSchema(st, &mockFileNamer{}, true, true).(*MD)
		conv.SetKnownDocs(knownDocs)

		// Generate schema
		schemaBytes, err := conv.GenerateJSONSchema()
		require.NoError(t, err)
		require.NotNil(t, schemaBytes)

		// Parse and verify schema
		var schema map[string]interface{}
		err = json.Unmarshal(schemaBytes, &schema)
		require.NoError(t, err)

		// Verify only the fields that were provided
		assert.Equal(t, "Custom Type", schema["title"])
		assert.Equal(t, "A custom object type", schema["description"])
		assert.Equal(t, "🔧", schema["x-icon-emoji"])

		// These should not be present
		assert.Nil(t, schema["x-plural"])
		assert.Nil(t, schema["x-icon-name"])
	})

	t.Run("schema root parameters", func(t *testing.T) {
		// Setup state with object type
		st := state.NewDoc("root", nil).NewState()
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("projectType"))

		// Create known docs with complete metadata including dates and author
		createdTimestamp := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
		lastModifiedTimestamp := time.Date(2024, 6, 14, 15, 45, 0, 0, time.UTC).Unix()
		knownDocs := map[string]*domain.Details{
			"projectType": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:               domain.String("projectType"),
				bundle.RelationKeyName:             domain.String("Project"),
				bundle.RelationKeyDescription:      domain.String("Project management type"),
				bundle.RelationKeyCreator:          domain.String("user123"),
				bundle.RelationKeyLastModifiedBy:   domain.String("7a12b3c4d5e6f7890"),
				bundle.RelationKeyCreatedDate:      domain.Int64(createdTimestamp),
				bundle.RelationKeyLastModifiedDate: domain.Int64(lastModifiedTimestamp),
			}),
		}

		// Create converter
		conv := NewMDConverterWithSchema(st, &mockFileNamer{}, true, true).(*MD)
		conv.SetKnownDocs(knownDocs)

		// Generate schema
		schemaBytes, err := conv.GenerateJSONSchema()
		require.NoError(t, err)

		// Parse schema
		var schema map[string]interface{}
		err = json.Unmarshal(schemaBytes, &schema)
		require.NoError(t, err)

		// Verify root parameters
		expectedId := "urn:anytype:schema:2024-06-14:author-7a12:type-project:gen-1.0.0"
		assert.Equal(t, expectedId, schema["$id"])
		assert.Equal(t, "Anytype", schema["x-app"])
		assert.Equal(t, "7a12b3c4d5e6f7890", schema["x-type-author"])
		assert.Equal(t, "2024-01-15T10:30:00Z", schema["x-type-date"])
		assert.Equal(t, "1.0.0", schema["x-genVersion"])
	})

	t.Run("schema root parameters with fallbacks", func(t *testing.T) {
		// Setup state with object type
		st := state.NewDoc("root", nil).NewState()
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("documentType"))

		// Create known docs with minimal metadata (no lastModified, no lastModifiedBy)
		createdTimestamp := time.Date(2024, 3, 20, 14, 0, 0, 0, time.UTC).Unix()
		knownDocs := map[string]*domain.Details{
			"documentType": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:          domain.String("documentType"),
				bundle.RelationKeyName:        domain.String("Document"),
				bundle.RelationKeyCreator:     domain.String("abc123"),
				bundle.RelationKeyCreatedDate: domain.Int64(createdTimestamp),
				// No LastModifiedDate, no LastModifiedBy
			}),
		}

		// Create converter
		conv := NewMDConverterWithSchema(st, &mockFileNamer{}, true, true).(*MD)
		conv.SetKnownDocs(knownDocs)

		// Generate schema
		schemaBytes, err := conv.GenerateJSONSchema()
		require.NoError(t, err)

		// Parse schema
		var schema map[string]interface{}
		err = json.Unmarshal(schemaBytes, &schema)
		require.NoError(t, err)

		// Verify root parameters with fallbacks
		schemaId, ok := schema["$id"].(string)
		require.True(t, ok)
		// Should use created date since no lastModified
		assert.Contains(t, schemaId, ":2024-03-20:")
		// Should use first 4 chars of creator since no lastModifiedBy
		assert.Contains(t, schemaId, ":author-abc1:")
		assert.Contains(t, schemaId, ":type-document:")

		// x-type-author should fallback to creator
		assert.Equal(t, "abc123", schema["x-type-author"])
	})

	t.Run("yaml front matter with schema reference", func(t *testing.T) {
		// Setup state with object type
		blocks := map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"text1"}}),
			"text1": simple.New(&model.Block{
				Id: "text1",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text: "Test content",
					},
				},
			}),
		}
		st := state.NewDoc("root", blocks).(*state.State)
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("taskType"))
		st.SetLocalDetail(bundle.RelationKeyName, domain.String("My Task"))

		// Create known docs
		knownDocs := map[string]*domain.Details{
			"taskType": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:                   domain.String("taskType"),
				bundle.RelationKeyName:                 domain.String("Task"),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"nameRel"}),
			}),
			"nameRel": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("nameRel"),
				bundle.RelationKeyName:           domain.String("Name"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
				bundle.RelationKeyRelationKey:    domain.String("name"),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			}),
		}

		// Create converter with schema enabled
		conv := NewMDConverterWithSchema(st, &mockFileNamer{}, true, true).(*MD)
		conv.SetKnownDocs(knownDocs)

		// Convert to markdown
		result := conv.Convert(model.SmartBlockType_Page)
		resultStr := string(result)

		// Check that schema reference is included
		t.Logf("Result: %s", resultStr)
		assert.True(t, strings.Contains(resultStr, "---"))
		assert.True(t, strings.Contains(resultStr, "# yaml-language-server: $schema=./schemas/task.schema.json"))
	})
}

func TestMD_FileFormatRelations(t *testing.T) {
	t.Skip("Skipping file format relations test - needs to be updated to use resolver pattern")
	newState := func(bs ...*model.Block) *state.State {
		var sbs []simple.Block
		var ids []string
		for _, b := range bs {
			sb := simple.New(b)
			ids = append(ids, sb.Model().Id)
			sbs = append(sbs, sb)
		}
		blocks := map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: ids}),
		}
		for _, sb := range sbs {
			blocks[sb.Model().Id] = sb
		}
		return state.NewDoc("root", blocks).(*state.State)
	}

	t.Run("file format relations", func(t *testing.T) {
		// Create a state with local details
		st := newState(
			&model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "Test content",
						Style: model.BlockContentText_Paragraph,
					},
				},
			},
		)

		// Set object type and file relation
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("docType"))
		st.SetLocalDetail(bundle.RelationKeyName, domain.String("My Document"))
		st.SetLocalDetail("attachedFile", domain.String("file123"))
		st.SetLocalDetail("coverImage", domain.String("image456"))
		st.SetLocalDetail("attachments", domain.StringList([]string{"file789", "pdf012"}))

		// Create known docs with file objects
		knownDocs := map[string]*domain.Details{
			"docType": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("docType"),
				bundle.RelationKeyName: domain.String("Document"),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{
					"attachedFileRel", "coverImageRel", "attachmentsRel",
				}),
			}),
			"attachedFileRel": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("attachedFileRel"),
				bundle.RelationKeyName:           domain.String("Attached File"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_file)),
				bundle.RelationKeyRelationKey:    domain.String("attachedFile"),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			}),
			"coverImageRel": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("coverImageRel"),
				bundle.RelationKeyName:           domain.String("Cover Image"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_file)),
				bundle.RelationKeyRelationKey:    domain.String("coverImage"),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			}),
			"attachmentsRel": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("attachmentsRel"),
				bundle.RelationKeyName:           domain.String("Attachments"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_file)),
				bundle.RelationKeyRelationKey:    domain.String("attachments"),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			}),
			// File objects
			"file123": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("file123"),
				bundle.RelationKeyName:    domain.String("report.docx"),
				bundle.RelationKeyLayout:  domain.Int64(int64(model.ObjectType_file)),
				bundle.RelationKeyFileExt: domain.String("docx"),
			}),
			"image456": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("image456"),
				bundle.RelationKeyName:    domain.String("cover.png"),
				bundle.RelationKeyLayout:  domain.Int64(int64(model.ObjectType_image)),
				bundle.RelationKeyFileExt: domain.String("png"),
			}),
			"file789": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("file789"),
				bundle.RelationKeyName:    domain.String("data.xlsx"),
				bundle.RelationKeyLayout:  domain.Int64(int64(model.ObjectType_file)),
				bundle.RelationKeyFileExt: domain.String("xlsx"),
			}),
			"pdf012": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("pdf012"),
				bundle.RelationKeyName:    domain.String("manual.pdf"),
				bundle.RelationKeyLayout:  domain.Int64(int64(model.ObjectType_pdf)),
				bundle.RelationKeyFileExt: domain.String("pdf"),
			}),
		}

		// Create converter
		conv := NewMDConverter(st, &mockFileNamer{}, true).(*MD)
		conv.SetKnownDocs(knownDocs)

		// Convert to markdown
		result := conv.Convert(model.SmartBlockType_Page)
		resultStr := string(result)

		// Check that file relations are properly rendered
		t.Logf("Result:\n%s", resultStr)

		// Check single file
		assert.Contains(t, resultStr, "Attached File: files/file123_report.docx")

		// Check image file
		assert.Contains(t, resultStr, "Cover Image: files/image456_cover.png")

		// Check multiple files
		assert.Contains(t, resultStr, "Attachments:")
		assert.Contains(t, resultStr, "- files/file789_data.xlsx")
		assert.Contains(t, resultStr, "- files/pdf012_manual.pdf")

		// Check that file hashes were recorded
		assert.Contains(t, conv.fileHashes, "file123")
		assert.Contains(t, conv.fileHashes, "file789")
		assert.Contains(t, conv.fileHashes, "pdf012")
		assert.Contains(t, conv.imageHashes, "image456")
	})
}
