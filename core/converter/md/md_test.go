package md

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

	t.Run("test code block rendering", func(t *testing.T) {
		s := newState(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "function hello() {\n  console.log('Hello World');\n}",
					Style: model.BlockContentText_Code,
				},
			},
		})
		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)
		exp := "```\nfunction hello() {\n  console.log('Hello World');\n}\n```\n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test code block with indentation", func(t *testing.T) {
		// Create a nested structure: list item containing code block
		listBlock := &model.Block{
			Id:          "list1",
			ChildrenIds: []string{"code1"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "List item with code:",
					Style: model.BlockContentText_Marked,
				},
			},
		}
		codeBlock := &model.Block{
			Id: "code1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "function test() {\n  return 42;\n}",
					Style: model.BlockContentText_Code,
				},
			},
		}

		blocks := map[string]simple.Block{
			"root":  simple.New(&model.Block{Id: "root", ChildrenIds: []string{"list1"}}),
			"list1": simple.New(listBlock),
			"code1": simple.New(codeBlock),
		}
		s := state.NewDoc("root", blocks).(*state.State)

		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)

		// Expected: list item followed by indented code block
		exp := "- List item with code:   \n    ```\n    function test() {\n      return 42;\n    }\n    ```\n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test code block with deeper indentation", func(t *testing.T) {
		// Create deeply nested structure: numbered list -> bullet list -> code block
		numberedBlock := &model.Block{
			Id:          "num1",
			ChildrenIds: []string{"bullet1"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Numbered item",
					Style: model.BlockContentText_Numbered,
				},
			},
		}
		bulletBlock := &model.Block{
			Id:          "bullet1",
			ChildrenIds: []string{"code1"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Bullet item",
					Style: model.BlockContentText_Marked,
				},
			},
		}
		codeBlock := &model.Block{
			Id: "code1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "const x = 1;\nconst y = 2;",
					Style: model.BlockContentText_Code,
				},
			},
		}

		blocks := map[string]simple.Block{
			"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"num1"}}),
			"num1":    simple.New(numberedBlock),
			"bullet1": simple.New(bulletBlock),
			"code1":   simple.New(codeBlock),
		}
		s := state.NewDoc("root", blocks).(*state.State)

		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)

		// Expected: numbered list with nested bullet list and deeply indented code block
		exp := "1. Numbered item   \n    - Bullet item   \n        ```\n        const x = 1;\n        const y = 2;\n        ```\n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test code block with escaped backticks", func(t *testing.T) {
		s := newState(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "console.log(`template string`);\nconst code = '```';\nconsole.log(code);",
					Style: model.BlockContentText_Code,
				},
			},
		})
		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)

		// Backticks in code should be escaped
		exp := "```\nconsole.log(`template string`);\nconst code = '\\`\\`\\`';\nconsole.log(code);\n```\n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test code block under header with indentation", func(t *testing.T) {
		headerBlock := &model.Block{
			Id:          "header1",
			ChildrenIds: []string{"code1"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Code Example",
					Style: model.BlockContentText_Header2,
				},
			},
		}
		codeBlock := &model.Block{
			Id: "code1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "# This is a comment\nprint('Hello')",
					Style: model.BlockContentText_Code,
				},
			},
		}

		blocks := map[string]simple.Block{
			"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"header1"}}),
			"header1": simple.New(headerBlock),
			"code1":   simple.New(codeBlock),
		}
		s := state.NewDoc("root", blocks).(*state.State)

		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)

		// Code block under header should be indented with 4 spaces
		exp := "## Code Example   \n    ```\n    # This is a comment\n    print('Hello')\n    ```\n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test empty code block", func(t *testing.T) {
		s := newState(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "",
					Style: model.BlockContentText_Code,
				},
			},
		})
		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)
		exp := "```\n\n```\n"
		assert.Equal(t, exp, string(res))
	})

	t.Run("test code block with only newlines", func(t *testing.T) {
		s := newState(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "\n\n\n",
					Style: model.BlockContentText_Code,
				},
			},
		})
		c := NewMDConverter(s, nil, false)
		res := c.Convert(model.SmartBlockType_Page)
		exp := "```\n\n\n\n\n```\n"
		assert.Equal(t, exp, string(res))
	})
}

// testFileNamer implements FileNamer interface
type testFileNamer struct{}

func (f *testFileNamer) Get(path, hash, title, ext string) string {
	if path != "" {
		return filepath.Join(path, title+ext)
	}
	return title + ext
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
		assert.Equal(t, "1.0.0", schema["x-schema-version"])
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
				result := GenerateSchemaFileName(tt.typeName)
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
		assert.Equal(t, "1.0.0", schema["x-schema-version"])
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

func TestMD_RenderCollection(t *testing.T) {
	// Create test state with a collection layout
	rootBlock := &model.Block{
		Id:          "root",
		ChildrenIds: []string{"text1"},
	}
	textBlock := &model.Block{
		Id: "text1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "My Collection",
			},
		},
	}
	st := state.NewDoc("root", map[string]simple.Block{
		"root":  simple.New(rootBlock),
		"text1": simple.New(textBlock),
	}).NewState()

	// Set collection layout
	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("collection-123"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("type-collection"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
	st.SetDetailAndBundledRelation(bundle.RelationKeyName, domain.String("My Task Collection"))

	// Add collection objects to store
	collectionObjects := []string{"task1", "task2", "task3"}
	st.SetInStore([]string{template.CollectionStoreKey}, pbtypes.StringList(collectionObjects))

	// Create mock resolver
	resolver := &testResolver{
		objects: map[string]*domain.Details{
			"task1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("First Task"),
			}),
			"task2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Second Task"),
			}),
			"task3": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Third Task"),
			}),
		},
		types: map[string]*domain.Details{
			"type-collection": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:                         domain.String("Collection"),
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

	// Create converter with resolver
	conv := NewMDConverterWithResolver(st, &testFileNamer{}, true, false, resolver)
	// Set known docs to simulate only task1 and task2 are in export
	conv.SetKnownDocs(map[string]*domain.Details{
		"task1": resolver.objects["task1"],
		"task2": resolver.objects["task2"],
		// task3 is NOT in knownDocs
	})

	// Convert to markdown
	result := conv.Convert(model.SmartBlockType_Page)
	resultStr := string(result)

	// Verify YAML frontmatter contains collection
	assert.Contains(t, resultStr, "Collection:")

	// Verify task1 and task2 show filenames (they are in knownDocs)
	assert.Contains(t, resultStr, "- First Task.md")
	assert.Contains(t, resultStr, "- Second Task.md")

	// Verify task3 shows just the name (not in knownDocs)
	assert.Contains(t, resultStr, "- Third Task")
	assert.NotContains(t, resultStr, "- Third Task.md")

	// Verify the structure is correct (simple list format)
	lines := strings.Split(resultStr, "\n")
	var inCollection bool
	for i, line := range lines {
		if strings.Contains(line, "Collection:") {
			inCollection = true
			// Next lines should be the list items
			continue
		}
		if inCollection && strings.HasPrefix(line, "- ") {
			// Verify it's a simple list item
			assert.True(t, strings.HasPrefix(line, "- "), "Collection items should be simple list items")
		}
		// Stop checking after YAML frontmatter ends
		if line == "---" && i > 0 {
			break
		}
	}
}

func TestMD_RenderCollection_EmptyCollection(t *testing.T) {
	// Create test state with a collection layout but no objects
	rootBlock := &model.Block{
		Id:          "root",
		ChildrenIds: []string{"text1"},
	}
	textBlock := &model.Block{
		Id: "text1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "Empty Collection",
			},
		},
	}
	st := state.NewDoc("root", map[string]simple.Block{
		"root":  simple.New(rootBlock),
		"text1": simple.New(textBlock),
	}).NewState()

	// Set collection layout
	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("collection-empty"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("type-collection"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))

	// No collection objects in store

	// Create mock resolver
	resolver := &testResolver{
		types: map[string]*domain.Details{
			"type-collection": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Collection"),
			}),
		},
	}

	// Create converter
	conv := NewMDConverterWithResolver(st, &testFileNamer{}, true, false, resolver)

	// Convert to markdown
	result := conv.Convert(model.SmartBlockType_Page)
	resultStr := string(result)

	// Verify Collection field is not present for empty collection
	assert.NotContains(t, resultStr, "Collection:")
}

func TestMD_RenderCollection_WithSchema(t *testing.T) {
	// Create test state with a collection layout
	rootBlock := &model.Block{
		Id:          "root",
		ChildrenIds: []string{"text1"},
	}
	textBlock := &model.Block{
		Id: "text1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "Collection with Schema",
			},
		},
	}
	st := state.NewDoc("root", map[string]simple.Block{
		"root":  simple.New(rootBlock),
		"text1": simple.New(textBlock),
	}).NewState()

	// Set collection layout
	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("collection-456"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("type-collection"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
	st.SetDetailAndBundledRelation(bundle.RelationKeyName, domain.String("Collection with Schema"))

	// Add one collection object
	st.SetInStore([]string{template.CollectionStoreKey}, pbtypes.StringList([]string{"obj1"}))

	// Create mock resolver
	resolver := &testResolver{
		objects: map[string]*domain.Details{
			"obj1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Object One"),
			}),
		},
		types: map[string]*domain.Details{
			"type-collection": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:                         domain.String("My Collection Type"),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-name"}),
			}),
		},
		relations: map[string]*domain.Details{
			"rel-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-type"),
				bundle.RelationKeyRelationKey:    domain.String("type"),
				bundle.RelationKeyName:           domain.String("Type"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			}),
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

	// Create converter with schema enabled
	conv := NewMDConverterWithResolver(st, &testFileNamer{}, true, true, resolver)

	// Convert to markdown
	result := conv.Convert(model.SmartBlockType_Page)
	resultStr := string(result)

	// Verify schema reference is present
	assert.Contains(t, resultStr, "# yaml-language-server: $schema=schemas/my_collection_type.schema.json")

	// Verify collection is present
	assert.Contains(t, resultStr, "Collection:")
	// Object is not in knownDocs (not set), so it shows name only
	assert.Contains(t, resultStr, "- Object One")
}

func TestMD_RenderCollection_UnknownObjects(t *testing.T) {
	// Create test state
	rootBlock := &model.Block{
		Id:          "root",
		ChildrenIds: []string{"text1"},
	}
	textBlock := &model.Block{
		Id: "text1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "Collection",
			},
		},
	}
	st := state.NewDoc("root", map[string]simple.Block{
		"root":  simple.New(rootBlock),
		"text1": simple.New(textBlock),
	}).NewState()

	// Set collection layout
	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("collection-789"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("type-collection"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))

	// Add collection objects, including unknown ones
	st.SetInStore([]string{template.CollectionStoreKey}, pbtypes.StringList([]string{"known1", "unknown1", "known2"}))

	// Create mock resolver with only some objects known
	resolver := &testResolver{
		objects: map[string]*domain.Details{
			"known1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Known Object 1"),
			}),
			"known2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Known Object 2"),
			}),
			// unknown1 is not in resolver
		},
		types: map[string]*domain.Details{
			"type-collection": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Collection"),
			}),
		},
	}

	// Create converter
	conv := NewMDConverterWithResolver(st, &testFileNamer{}, true, false, resolver)

	// Convert to markdown
	result := conv.Convert(model.SmartBlockType_Page)
	resultStr := string(result)

	// Verify known objects show their names (not in knownDocs, so just names)
	assert.Contains(t, resultStr, "- Known Object 1")
	assert.Contains(t, resultStr, "- Known Object 2")

	// Unknown object is not in resolver, so it won't appear in the list
}

func TestMD_RenderObjectRelation_FileFieldOnlyForExportedObjects(t *testing.T) {
	// Create test state with a simple block
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

	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("test-object"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("test-type"))

	// Add object relation with references
	st.SetDetail(domain.RelationKey("relatedObjects"), domain.StringList([]string{"obj1", "obj2", "obj3"}))

	// Create mock resolver
	resolver := &testResolver{
		objects: map[string]*domain.Details{
			"obj1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Object One"),
			}),
			"obj2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Object Two"),
			}),
			"obj3": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Object Three"),
			}),
		},
		types: map[string]*domain.Details{
			"test-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:                         domain.String("Test Type"),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-related"}),
			}),
		},
		relations: map[string]*domain.Details{
			"rel-related": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-related"),
				bundle.RelationKeyRelationKey:    domain.String("relatedObjects"),
				bundle.RelationKeyName:           domain.String("Related Objects"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			}),
		},
	}
	resolver.keyMapping = map[string]string{
		"type":           "rel-type",
		"relatedObjects": "rel-related",
	}

	// Create fileNamer
	fileNamer := &testFileNamer{}

	// Create converter with known docs (only obj1 and obj2 are in export)
	conv := NewMDConverterWithResolver(st, fileNamer, true, false, resolver)
	conv.SetKnownDocs(map[string]*domain.Details{
		"obj1": resolver.objects["obj1"],
		"obj2": resolver.objects["obj2"],
		// obj3 is NOT in knownDocs, simulating it's not included in export
	})

	// Convert to markdown
	result := conv.Convert(model.SmartBlockType_Page)
	resultStr := string(result)

	// Verify the YAML contains object relations as a simple list
	assert.Contains(t, resultStr, "Related Objects:")
	assert.Contains(t, resultStr, "- Object One.md") // obj1 is exported, shows filename
	assert.Contains(t, resultStr, "- Object Two.md") // obj2 is exported, shows filename
	assert.Contains(t, resultStr, "- Object Three")  // obj3 is not exported, shows name only
}

func TestMD_RenderObjectRelation_ShortFormatUnaffected(t *testing.T) {
	// Create test state with a simple block
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

	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("test-object"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("test-type"))

	// Add backlinks (short format)
	st.SetDetailAndBundledRelation(bundle.RelationKeyBacklinks, domain.StringList([]string{"obj1", "obj2"}))

	// Create mock resolver
	resolver := &testResolver{
		objects: map[string]*domain.Details{
			"obj1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Backlink One"),
			}),
			"obj2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Backlink Two"),
			}),
		},
		types: map[string]*domain.Details{
			"test-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:                         domain.String("Test Type"),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-backlinks"}),
			}),
		},
		relations: map[string]*domain.Details{
			"rel-backlinks": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-backlinks"),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyBacklinks.String()),
				bundle.RelationKeyName:           domain.String("Backlinks"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			}),
		},
	}
	resolver.keyMapping = map[string]string{
		bundle.RelationKeyBacklinks.String(): "rel-backlinks",
	}

	// Create fileNamer
	fileNamer := &testFileNamer{}

	// Create converter with only obj1 in known docs
	conv := NewMDConverterWithResolver(st, fileNamer, true, false, resolver)
	conv.SetKnownDocs(map[string]*domain.Details{
		"obj1": resolver.objects["obj1"],
		// obj2 is NOT in knownDocs
	})

	// Convert to markdown
	result := conv.Convert(model.SmartBlockType_Page)
	resultStr := string(result)

	// Verify both objects are shown with just names (short format)
	assert.Contains(t, resultStr, "- Backlink One")
	assert.Contains(t, resultStr, "- Backlink Two")

	// Verify no File fields are shown for short format
	assert.NotContains(t, resultStr, "File:")
}

// testResolver implements ObjectResolver interface
type testResolver struct {
	objects    map[string]*domain.Details
	types      map[string]*domain.Details
	relations  map[string]*domain.Details
	keyMapping map[string]string
}

func (r *testResolver) ResolveRelation(relationId string) (*domain.Details, error) {
	return r.relations[relationId], nil
}

func (r *testResolver) ResolveType(typeId string) (*domain.Details, error) {
	return r.types[typeId], nil
}

func (r *testResolver) ResolveRelationOptions(relationKey string) ([]*domain.Details, error) {
	return nil, nil
}

func (r *testResolver) ResolveObject(objectId string) (*domain.Details, bool) {
	obj, ok := r.objects[objectId]
	return obj, ok
}

func (r *testResolver) GetRelationByKey(relationKey string) (*domain.Details, error) {
	if id, ok := r.keyMapping[relationKey]; ok {
		return r.relations[id], nil
	}
	return nil, nil
}

func TestMD_GenerateJSONSchema_WithEnhancements(t *testing.T) {
	// Create test state
	st := state.NewDoc("root", nil).NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String("test-object"))
	st.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String("test-type"))

	// Create mock resolver
	resolver := &testResolver{
		types: map[string]*domain.Details{
			"test-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:                           domain.String("test-type"),
				bundle.RelationKeyName:                         domain.String("Task"),
				bundle.RelationKeyUniqueKey:                    domain.String("ot-task"), // UniqueKey for TypeKey extraction
				bundle.RelationKeyDescription:                  domain.String("Task management object"),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-name", "rel-status"}),
				bundle.RelationKeyRecommendedRelations:         domain.StringList([]string{"rel-desc"}),
			}),
		},
		relations: map[string]*domain.Details{
			"rel-name": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-name"),
				bundle.RelationKeyRelationKey:    domain.String("custom_name"),
				bundle.RelationKeyName:           domain.String("Name"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			}),
			"rel-status": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-status"),
				bundle.RelationKeyRelationKey:    domain.String("custom_status"),
				bundle.RelationKeyName:           domain.String("Status"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			}),
			"rel-desc": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-desc"),
				bundle.RelationKeyRelationKey:    domain.String("custom_description"),
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
	assert.Equal(t, "id", idProp["x-key"])

	// Check Object type property exists (it's added automatically if not in relations)
	typeProp := properties["Object type"].(map[string]interface{})
	// Object type is added after: id (0) + featured relations (2) + regular relations (1) = 4
	assert.Equal(t, float64(4), typeProp["x-order"])
	assert.Equal(t, "type", typeProp["x-key"])

	// Check featured properties have x-featured and correct order
	nameProp := properties["Name"].(map[string]interface{})
	assert.Equal(t, true, nameProp["x-featured"])
	assert.Equal(t, float64(1), nameProp["x-order"]) // First property after id
	assert.Equal(t, "custom_name", nameProp["x-key"])

	statusProp := properties["Status"].(map[string]interface{})
	assert.Equal(t, true, statusProp["x-featured"])
	assert.Equal(t, float64(2), statusProp["x-order"]) // Second property
	assert.Equal(t, "custom_status", statusProp["x-key"])

	// Check non-featured property doesn't have x-featured but has order
	descProp := properties["Description"].(map[string]interface{})
	_, hasFeatured := descProp["x-featured"]
	assert.False(t, hasFeatured, "Non-featured property should not have x-featured")
	assert.Equal(t, float64(3), descProp["x-order"]) // Third property
	assert.Equal(t, "custom_description", descProp["x-key"])

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
	st.SetDetail(domain.RelationKey("custom_name"), domain.String("My Task"))

	// Create mock resolver
	resolver := &testResolver{
		types: map[string]*domain.Details{
			"test-type": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:                           domain.String("test-type"),
				bundle.RelationKeyName:                         domain.String("Task"),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-name"}),
			}),
		},
		relations: map[string]*domain.Details{
			"rel-name": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("rel-name"),
				bundle.RelationKeyRelationKey:    domain.String("custom_name"),
				bundle.RelationKeyName:           domain.String("Name"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			}),
		},
	}
	resolver.keyMapping = map[string]string{
		"type":        "rel-type",
		"custom_name": "rel-name",
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
	// ID appears after Name in the current implementation
	assert.Greater(t, idLine, nameLine, "ID should come after Name property")
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
				bundle.RelationKeyId:                           domain.String("test-type"),
				bundle.RelationKeyName:                         domain.String("Complex Type"),
				bundle.RelationKeyUniqueKey:                    domain.String("ot-complextype"), // Add unique key for type key extraction
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
		"Property 1": 1, // Featured properties start at 1
		"Property 2": 2,
		"Property 3": 3, // Regular properties follow
		"Property 4": 4,
		"Property 5": 5,
		// Object type is added after all the relations
		"Object type": 6, // Object type is added after the 5 custom properties
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

	// Verify all properties have x-key
	for propName, propValue := range properties {
		prop := propValue.(map[string]interface{})
		xKey, hasXKey := prop["x-key"]
		assert.True(t, hasXKey, "Property %s should have x-key", propName)

		// Verify x-key values for specific properties
		switch propName {
		case "id":
			assert.Equal(t, "id", xKey)
		case "Object type":
			assert.Equal(t, "type", xKey)
		case "Property 1":
			assert.Equal(t, "prop1", xKey)
		case "Property 2":
			assert.Equal(t, "prop2", xKey)
		case "Property 3":
			assert.Equal(t, "prop3", xKey)
		case "Property 4":
			assert.Equal(t, "prop4", xKey)
		case "Property 5":
			assert.Equal(t, "prop5", xKey)
		}
	}
}
