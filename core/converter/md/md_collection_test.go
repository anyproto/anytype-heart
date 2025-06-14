package md

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

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
				bundle.RelationKeyName: domain.String("Collection"),
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

	// Verify task1 and task2 have File field (they are in knownDocs)
	assert.Contains(t, resultStr, "- Name: First Task")
	assert.Contains(t, resultStr, "  File: First Task.md")
	assert.Contains(t, resultStr, "- Name: Second Task")
	assert.Contains(t, resultStr, "  File: Second Task.md")
	
	// Verify task3 has Id field instead (not in knownDocs)
	assert.Contains(t, resultStr, "- Name: Third Task")
	assert.Contains(t, resultStr, "  Id: task3")
	assert.NotContains(t, resultStr, "  File: Third Task.md")

	// Verify the structure is correct
	lines := strings.Split(resultStr, "\n")
	var inCollection bool
	var collectionIndent int
	for _, line := range lines {
		if strings.Contains(line, "Collection:") {
			inCollection = true
			collectionIndent = len(line) - len(strings.TrimLeft(line, " "))
		}
		if inCollection && strings.TrimSpace(line) != "" && !strings.Contains(line, "Collection:") && !strings.Contains(line, "---") {
			// Check that collection items are properly indented
			itemIndent := len(line) - len(strings.TrimLeft(line, " "))
			assert.Greater(t, itemIndent, collectionIndent, "Collection items should be indented")
		}
		// Stop checking after YAML frontmatter ends
		if inCollection && line == "---" {
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
				bundle.RelationKeyName: domain.String("My Collection Type"),
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
	assert.Contains(t, resultStr, "# yaml-language-server: $schema=./schemas/my_collection_type.schema.json")

	// Verify collection is present
	assert.Contains(t, resultStr, "Collection:")
	assert.Contains(t, resultStr, "- Name: Object One")
	// Object is not in knownDocs (not set), so it shows Id
	assert.Contains(t, resultStr, "  Id: obj1")
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

	// Verify known objects show their names
	assert.Contains(t, resultStr, "- Name: Known Object 1")
	assert.Contains(t, resultStr, "- Name: Known Object 2")

	// Verify unknown object shows its ID
	assert.Contains(t, resultStr, "- Name: unknown1")
}