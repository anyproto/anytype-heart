package md

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestMD_RenderObjectRelation_FileFieldOnlyForExportedObjects(t *testing.T) {
	// Create test state with a simple block
	rootBlock := &model.Block{
		Id: "root",
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
		"root": simple.New(rootBlock),
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
				bundle.RelationKeyName: domain.String("Test Type"),
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
		"type": "rel-type",
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

	// Verify obj1 and obj2 have File field
	assert.Contains(t, resultStr, "- Name: Object One")
	assert.Contains(t, resultStr, "  File: Object One.md")
	assert.Contains(t, resultStr, "- Name: Object Two")
	assert.Contains(t, resultStr, "  File: Object Two.md")

	// Verify obj3 has Name but NO File field
	assert.Contains(t, resultStr, "- Name: Object Three")
	assert.NotContains(t, resultStr, "  File: Object Three.md")
}

func TestMD_RenderObjectRelation_ShortFormatUnaffected(t *testing.T) {
	// Create test state with a simple block
	rootBlock := &model.Block{
		Id: "root",
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
		"root": simple.New(rootBlock),
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
				bundle.RelationKeyName: domain.String("Test Type"),
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

// testFileNamer implements FileNamer interface
type testFileNamer struct{}

func (f *testFileNamer) Get(path, hash, title, ext string) string {
	if path != "" {
		return path + "/" + title + ext
	}
	return title + ext
}