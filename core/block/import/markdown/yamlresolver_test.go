package markdown

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestMarkdown_YAMLConsistentProperties(t *testing.T) {
	t.Run("multiple files with same YAML properties get same relation keys", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()

		// Create multiple markdown files with the same properties
		file1Content := `---
title: Document 1
status: In Progress
priority: high
tags: [work, project]
---

# Document 1
Content of document 1.`

		file2Content := `---
title: Document 2
status: Done
priority: low
tags: [personal]
---

# Document 2
Content of document 2.`

		file3Content := `---
title: Document 3
status: In Progress
priority: high
tags: [work, urgent]
custom_field: Some value
---

# Document 3
Content of document 3.`

		file1Path := filepath.Join(testDirectory, "doc1.md")
		file2Path := filepath.Join(testDirectory, "doc2.md")
		file3Path := filepath.Join(testDirectory, "doc3.md")

		err := os.WriteFile(file1Path, []byte(file1Content), os.ModePerm)
		require.NoError(t, err)
		err = os.WriteFile(file2Path, []byte(file2Content), os.ModePerm)
		require.NoError(t, err)
		err = os.WriteFile(file3Path, []byte(file3Content), os.ModePerm)
		require.NoError(t, err)

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
			schemaImporter: NewSchemaImporter(), // No schemas loaded
		}
		p := process.NewNoOp()

		// when
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{testDirectory}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)

		// Collect all relation snapshots and their keys
		relationKeys := make(map[string]string) // name -> key
		fileSnapshots := make(map[string]*common.Snapshot)

		for _, snapshot := range sn.Snapshots {
			if snapshot.Snapshot.SbType == coresb.SmartBlockTypeRelation {
				name := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName)
				key := snapshot.Snapshot.Data.Key

				// Check if we've seen this relation before
				if existingKey, exists := relationKeys[name]; exists {
					// Same property name should have the same key
					assert.Equal(t, existingKey, key,
						"Property '%s' has different keys: %s vs %s", name, existingKey, key)
				} else {
					relationKeys[name] = key
				}
			} else if snapshot.FileName != "" {
				fileSnapshots[snapshot.FileName] = snapshot
			}
		}

		// Verify we have the expected relations
		// Note: properties are title-cased during import
		expectedRelations := []string{"title", "Status", "priority", "Tag", "custom_field"}
		for _, expected := range expectedRelations {
			_, exists := relationKeys[expected]
			assert.True(t, exists, "Missing relation: %s", expected)
		}

		// Verify that all documents use the same keys for the same properties
		for fileName, snapshot := range fileSnapshots {
			details := snapshot.Snapshot.Data.Details

			// For each property in the details, check if it uses the correct key
			for propName, propKey := range relationKeys {
				value := details.Get(domain.RelationKey(propKey))
				if !value.IsNull() {
					t.Logf("File %s has property '%s' with key '%s'",
						filepath.Base(fileName), propName, propKey)
				}
			}
		}

		// Specific checks: all files should use the same key for 'Status'
		statusKey := relationKeys["Status"]
		assert.NotEmpty(t, statusKey)

		doc1Details := fileSnapshots[file1Path].Snapshot.Data.Details
		doc2Details := fileSnapshots[file2Path].Snapshot.Data.Details
		doc3Details := fileSnapshots[file3Path].Snapshot.Data.Details

		// All documents should have status values under the same key
		assert.False(t, doc1Details.Get(domain.RelationKey(statusKey)).IsNull())
		assert.False(t, doc2Details.Get(domain.RelationKey(statusKey)).IsNull())
		assert.False(t, doc3Details.Get(domain.RelationKey(statusKey)).IsNull())
	})
}
