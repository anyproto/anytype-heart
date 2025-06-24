package md

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// mockWriter for testing
type mockWriter struct {
	files map[string][]byte
}

func newMockWriter() *mockWriter {
	return &mockWriter{
		files: make(map[string][]byte),
	}
}

func (w *mockWriter) WriteFile(filename string, r io.Reader, lastModifiedDate int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	w.files[filename] = data
	return nil
}

// mockResolver for testing
type mockResolver struct {
	types      map[string]*domain.Details
	relations  map[string]*domain.Details
	objects    map[string]*domain.Details
	options    map[string][]*domain.Details
	keyMapping map[string]string // relationKey -> relationId
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		types:      make(map[string]*domain.Details),
		relations:  make(map[string]*domain.Details),
		objects:    make(map[string]*domain.Details),
		options:    make(map[string][]*domain.Details),
		keyMapping: make(map[string]string),
	}
}

func (r *mockResolver) ResolveRelation(relationId string) (*domain.Details, error) {
	return r.relations[relationId], nil
}

func (r *mockResolver) ResolveType(typeId string) (*domain.Details, error) {
	return r.types[typeId], nil
}

func (r *mockResolver) ResolveRelationOptions(relationKey string) ([]*domain.Details, error) {
	return r.options[relationKey], nil
}

func (r *mockResolver) ResolveObject(objectId string) (*domain.Details, bool) {
	obj, ok := r.objects[objectId]
	return obj, ok
}

func (r *mockResolver) GetRelationByKey(relationKey string) (*domain.Details, error) {
	if id, ok := r.keyMapping[relationKey]; ok {
		return r.relations[id], nil
	}
	return nil, nil
}

// postProcessorFileNamer for testing
type postProcessorFileNamer struct{}

func (f *postProcessorFileNamer) Get(path, hash, title, ext string) string {
	if path != "" {
		return filepath.Join(path, title+ext)
	}
	return title + ext
}

func TestPostProcessor_GenerateAllSchemas(t *testing.T) {
	// Setup mock resolver
	resolver := newMockResolver()

	// Add Type relation
	typeRelationId := "rel-type"
	resolver.relations[typeRelationId] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:             domain.String(typeRelationId),
		bundle.RelationKeyRelationKey:    domain.String("type"),
		bundle.RelationKeyName:           domain.String("Type"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
	})
	resolver.keyMapping["type"] = typeRelationId

	// Add Name relation
	nameRelationId := "rel-name"
	resolver.relations[nameRelationId] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:             domain.String(nameRelationId),
		bundle.RelationKeyRelationKey:    domain.String("custom_name"),
		bundle.RelationKeyName:           domain.String("Name"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
	})

	// Add Description relation
	descRelationId := "rel-desc"
	resolver.relations[descRelationId] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:             domain.String(descRelationId),
		bundle.RelationKeyRelationKey:    domain.String("custom_description"),
		bundle.RelationKeyName:           domain.String("Description"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
	})

	// Add Task type (including Type relation in featured)
	taskTypeId := "type-task"
	resolver.types[taskTypeId] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:                           domain.String(taskTypeId),
		bundle.RelationKeyName:                         domain.String("Task"),
		bundle.RelationKeyUniqueKey:                    domain.String("ot-task"), // Add unique key for type key extraction
		bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{nameRelationId}),
	})

	// Add Page type (including Type relation in featured)
	pageTypeId := "type-page"
	resolver.types[pageTypeId] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:                           domain.String(pageTypeId),
		bundle.RelationKeyName:                         domain.String("Page"),
		bundle.RelationKeyUniqueKey:                    domain.String("ot-page"), // Add unique key for type key extraction
		bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{}),
		bundle.RelationKeyRecommendedRelations:         domain.StringList([]string{descRelationId}),
	})

	// Create test documents with different types
	docs := map[string]*domain.Details{
		"obj1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyType: domain.String(taskTypeId),
			bundle.RelationKeyName: domain.String("My Task"),
		}),
		"obj2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj2"),
			bundle.RelationKeyType: domain.String(pageTypeId),
			bundle.RelationKeyName: domain.String("My Page"),
		}),
		"obj3": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj3"),
			bundle.RelationKeyType: domain.String(taskTypeId), // Another task
			bundle.RelationKeyName: domain.String("Another Task"),
		}),
	}

	// Create post processor
	fileNamer := &postProcessorFileNamer{}
	postProcessor := NewMDPostProcessor(resolver, fileNamer)

	// Create mock writer
	writer := newMockWriter()

	// Run schema generation
	err := postProcessor.Process(docs, writer)
	require.NoError(t, err)

	// Verify schemas were generated
	assert.Len(t, writer.files, 2, "Should generate 2 schemas (one for each unique type)")

	// Check Task schema
	taskSchema, ok := writer.files["schemas/task.schema.json"]
	assert.True(t, ok, "Task schema should be generated")
	assert.Contains(t, string(taskSchema), `"title": "Task"`)
	assert.Contains(t, string(taskSchema), `"Name"`)

	// Check Page schema
	pageSchema, ok := writer.files["schemas/page.schema.json"]
	assert.True(t, ok, "Page schema should be generated")
	assert.Contains(t, string(pageSchema), `"title": "Page"`)
	assert.Contains(t, string(pageSchema), `"Description"`)

	// Verify no duplicate schemas were generated
	assert.Len(t, postProcessor.writtenSchemas, 2, "Should track 2 written schemas")
}

func TestPostProcessor_GenerateAllSchemas_SkipsInvalidTypes(t *testing.T) {
	resolver := newMockResolver()
	fileNamer := &postProcessorFileNamer{}
	postProcessor := NewMDPostProcessor(resolver, fileNamer)
	writer := newMockWriter()

	// Create documents with missing or invalid types
	docs := map[string]*domain.Details{
		"obj1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("obj1"),
			// No type specified
		}),
		"obj2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj2"),
			bundle.RelationKeyType: domain.String("invalid-type"), // Type not in resolver
		}),
		"obj3": nil, // nil document
	}

	// Run schema generation
	err := postProcessor.Process(docs, writer)
	require.NoError(t, err)

	// Verify no schemas were generated
	assert.Len(t, writer.files, 0, "Should not generate schemas for invalid types")
}

func TestPostProcessor_GenerateAllSchemas_HandlesEmptyDocs(t *testing.T) {
	resolver := newMockResolver()
	fileNamer := &postProcessorFileNamer{}
	postProcessor := NewMDPostProcessor(resolver, fileNamer)
	writer := newMockWriter()

	// Empty docs map
	docs := map[string]*domain.Details{}

	// Run schema generation
	err := postProcessor.Process(docs, writer)
	require.NoError(t, err)

	// Verify no schemas were generated
	assert.Len(t, writer.files, 0, "Should not generate schemas for empty docs")
}

func TestPostProcessor_IgnoresSystemTypes(t *testing.T) {
	resolver := newMockResolver()
	fileNamer := &postProcessorFileNamer{}
	postProcessor := NewMDPostProcessor(resolver, fileNamer)
	writer := newMockWriter()

	// Add Image type (system type that should be ignored)
	imageTypeId := "type-image"
	resolver.types[imageTypeId] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:        domain.String(imageTypeId),
		bundle.RelationKeyName:      domain.String("Image"),
		bundle.RelationKeyUniqueKey: domain.String("ot-image"), // UniqueKey format: ot-<typekey>
	})

	// Add File type (system type that should be ignored)
	fileTypeId := "type-file"
	resolver.types[fileTypeId] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:        domain.String(fileTypeId),
		bundle.RelationKeyName:      domain.String("File"),
		bundle.RelationKeyUniqueKey: domain.String("ot-file"),
	})

	// Add custom type (should NOT be ignored)
	customTypeId := "type-custom"
	resolver.types[customTypeId] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:        domain.String(customTypeId),
		bundle.RelationKeyName:      domain.String("Custom Type"),
		bundle.RelationKeyUniqueKey: domain.String("ot-customtype"),
	})

	// Create documents with these types
	docs := map[string]*domain.Details{
		"obj1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyType: domain.String(imageTypeId),
		}),
		"obj2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj2"),
			bundle.RelationKeyType: domain.String(fileTypeId),
		}),
		"obj3": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj3"),
			bundle.RelationKeyType: domain.String(customTypeId),
		}),
	}

	// Add Type relation for schema generation
	resolver.keyMapping["type"] = "rel-type"
	resolver.relations["rel-type"] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:   domain.String("rel-type"),
		bundle.RelationKeyName: domain.String("Type"),
	})

	// Run schema generation
	err := postProcessor.Process(docs, writer)
	require.NoError(t, err)

	// Verify only custom type got a schema
	assert.Len(t, writer.files, 1, "Should generate only 1 schema (custom type)")

	// Check that custom type schema was generated
	customSchema, ok := writer.files["schemas/custom_type.schema.json"]
	assert.True(t, ok, "Custom type schema should be generated")
	assert.Contains(t, string(customSchema), `"title": "Custom Type"`)

	// Verify system types were ignored
	_, hasImageSchema := writer.files["schemas/image.schema.json"]
	assert.False(t, hasImageSchema, "Image schema should NOT be generated")

	_, hasFileSchema := writer.files["schemas/file.schema.json"]
	assert.False(t, hasFileSchema, "File schema should NOT be generated")
}
