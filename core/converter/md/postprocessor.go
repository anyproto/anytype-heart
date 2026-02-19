package md

import (
	"bytes"
	"io"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// PostProcessor handles post-processing tasks for markdown export
type PostProcessor struct {
	resolver       ObjectResolver
	fileNamer      FileNamer
	writtenSchemas map[string]bool
}

// ignoredSystemTypes contains types that should not have schemas generated.
// These are system types that have special handling and are not meant to be
// created or modified by users through normal means.
var ignoredSystemTypes = map[domain.TypeKey]bool{
	// File types - handled specially with file content
	bundle.TypeKeyFile:  true,
	bundle.TypeKeyImage: true,
	bundle.TypeKeyVideo: true,
	bundle.TypeKeyAudio: true,

	// System infrastructure types
	bundle.TypeKeySpace:          true, // Workspace/space objects
	bundle.TypeKeySpaceView:      true, // Space view configuration
	bundle.TypeKeyParticipant:    true, // User/participant objects
	bundle.TypeKeyDashboard:      true, // Dashboard configuration
	bundle.TypeKeyObjectType:     true, // Type definitions themselves
	bundle.TypeKeyRelation:       true, // Relation/property definitions
	bundle.TypeKeyRelationOption: true, // Options for select/status relations
	bundle.TypeKeyDate:           true, // Date objects (special handling)
	bundle.TypeKeyTemplate:       true, // Template objects
}

// NewMDPostProcessor creates a new markdown post-processor
func NewMDPostProcessor(resolver ObjectResolver, fileNamer FileNamer) *PostProcessor {
	return &PostProcessor{
		resolver:       resolver,
		fileNamer:      fileNamer,
		writtenSchemas: make(map[string]bool),
	}
}

// Writer interface for writing files during post-processing
type Writer interface {
	WriteFile(filename string, r io.Reader, lastModifiedDate int64) error
}

// Process generates JSON schemas for all object types found in the given documents
func (p *PostProcessor) Process(docs map[string]*domain.Details, writer Writer) error {
	// Track all unique object types
	processedTypes := make(map[string]bool)

	// Iterate through all docs to find their types
	for _, doc := range docs {
		if doc == nil {
			continue
		}

		objectTypeId := doc.GetString(bundle.RelationKeyType)
		if objectTypeId == "" || processedTypes[objectTypeId] {
			continue
		}

		// Mark as processed
		processedTypes[objectTypeId] = true

		// Get type details
		typeDetails, err := p.resolver.ResolveType(objectTypeId)
		if err != nil || typeDetails == nil {
			continue
		}

		typeName := typeDetails.GetString(bundle.RelationKeyName)
		if typeName == "" {
			continue
		}

		// Check if this is a system type that should be ignored
		if rawUniqueKey := typeDetails.GetString(bundle.RelationKeyUniqueKey); rawUniqueKey != "" {
			typeKey, err := domain.GetTypeKeyFromRawUniqueKey(rawUniqueKey)
			if err == nil && ignoredSystemTypes[typeKey] {
				continue
			}
		}

		schemaFileName := GenerateSchemaFileName(typeName)

		// Skip if already written
		if p.writtenSchemas[schemaFileName] {
			continue
		}

		// Create a temporary state and converter to generate schema
		tempState := state.NewDoc("temp", nil).(*state.State)
		tempState.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String(objectTypeId))
		mdConv := NewMDConverterWithResolver(tempState, p.fileNamer, true, true, p.resolver)

		// Generate and write schema
		if schemaBytes, err := mdConv.(*MD).GenerateJSONSchema(); err == nil && schemaBytes != nil {
			if err = writer.WriteFile(schemaFileName, bytes.NewReader(schemaBytes), 0); err != nil {
				log.Warnf("failed to write JSON schema: %v", err)
			} else {
				p.writtenSchemas[schemaFileName] = true
			}
		}
	}

	return nil
}
