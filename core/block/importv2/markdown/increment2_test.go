package markdown

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const taskSchema = `{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"title": "Task",
	"x-app": "Anytype",
	"x-type-key": "custom_task",
	"properties": {
		"Priority": {
			"type": "string",
			"x-key": "task_priority",
			"x-format": "status",
			"enum": ["High", "Low"]
		},
		"Assignee": {
			"type": "string",
			"x-key": "task_assignee",
			"x-format": "shorttext"
		}
	}
}`

func runConverterWithParams(t *testing.T, files map[string]string, params Params) (*recordingSink, importv2.RootSpec) {
	t.Helper()
	src, err := source.Open(writeTree(t, files))
	require.NoError(t, err)
	t.Cleanup(func() { src.Close() })
	converter := New(src, params, stubFactory{})
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))
	sink := &recordingSink{}
	rootSpec, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)
	return sink, rootSpec
}

func TestSchemas(t *testing.T) {
	files := map[string]string{
		"task.schema.json": taskSchema,
		"work.md":          "---\nPriority: High\nAssignee: Roman\ntype: Task\n---\n# Work\n\nBody.\n",
	}

	t.Run("schema keys, options and type are adopted", func(t *testing.T) {
		// given / when — dir pages requested but force-disabled by schemas
		sink, rootSpec := runConverterWithParams(t, files, Params{CreateDirectoryPages: true})

		// then — schema definitions come first, with schema keys
		relation := sink.byKey("relation:task_priority")
		require.NotNil(t, relation, "schema relation must be emitted under its x-key")
		assert.Equal(t, coresb.SmartBlockTypeRelation, relation.SbType)
		assert.Equal(t, "task_priority", relation.Payload.Key)

		require.NotNil(t, sink.byKey("option:task_priority:High"), "schema enum option emitted")
		require.NotNil(t, sink.byKey("option:task_priority:Low"))

		typeObject := sink.byKey("type:Task")
		require.NotNil(t, typeObject)
		assert.Equal(t, "custom_task", typeObject.Payload.Key)

		page := sink.byKey("work.md")
		require.NotNil(t, page)
		assert.Equal(t, []string{"custom_task"}, page.Payload.ObjectTypes)
		assert.Equal(t, "option:task_priority:High", page.Payload.Details.GetString("task_priority"),
			"status value resolves to the schema option source key")
		assert.Equal(t, "Roman", page.Payload.Details.GetString("task_assignee"))

		// schemas force-disable directory pages
		assert.Nil(t, sink.byKey("dir:."))
		assert.Equal(t, "Markdown Import", rootSpec.CollectionName)
	})

	t.Run("schema definitions are emitted once, before any page", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, files, Params{})

		// then
		keys := sink.keys()
		indexOf := func(key string) int {
			for i, k := range keys {
				if k == key {
					return i
				}
			}
			return -1
		}
		assert.Less(t, indexOf("relation:task_priority"), indexOf("work.md"))
		assert.Less(t, indexOf("type:Task"), indexOf("work.md"))
	})
}

func TestDirectoryPages(t *testing.T) {
	files := map[string]string{
		"docs/guide/setup.md": "# Setup\n",
		"docs/guide/usage.md": "# Usage\n",
		"docs/intro.md":       "# Intro\n",
	}

	t.Run("one page per directory, tree widget at the collapsed root", func(t *testing.T) {
		// given / when — a single top-level dir collapses into the root
		sink, rootSpec := runConverterWithParams(t, files, Params{CreateDirectoryPages: true})

		// then
		assert.Equal(t, "dir:docs", rootSpec.RootObjectKey)
		assert.Equal(t, model.BlockContentWidget_Tree, rootSpec.WidgetLayout)
		assert.Empty(t, rootSpec.CollectionName)

		root := sink.byKey("dir:docs")
		require.NotNil(t, root)
		assert.True(t, root.IsRootCandidate)
		assert.Equal(t, "docs", root.Payload.Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, directoryIcon, root.Payload.Details.GetString(bundle.RelationKeyIconEmoji))

		var targets []string
		for _, b := range root.Payload.Blocks {
			targets = append(targets, b.GetLink().TargetBlockId)
		}
		assert.Equal(t, []string{"dir:docs/guide", "docs/intro.md"}, targets,
			"subdirectories link first, then documents")

		guide := sink.byKey("dir:docs/guide")
		require.NotNil(t, guide)
		assert.False(t, guide.IsRootCandidate)

		// regular pages are no longer root candidates
		page := sink.byKey("docs/intro.md")
		require.NotNil(t, page)
		assert.False(t, page.IsRootCandidate)
	})

	t.Run("multiple top-level entries keep the synthetic root", func(t *testing.T) {
		// given / when
		sink, rootSpec := runConverterWithParams(t, map[string]string{
			"a.md":      "# A\n",
			"docs/b.md": "# B\n",
		}, Params{CreateDirectoryPages: true})

		// then
		assert.Equal(t, "dir:.", rootSpec.RootObjectKey)
		root := sink.byKey("dir:.")
		require.NotNil(t, root)
		assert.Equal(t, "Markdown Import", root.Payload.Details.GetString(bundle.RelationKeyName))
	})
}

func TestPropertiesAsBlock(t *testing.T) {
	t.Run("front-matter renders as relation blocks, system keys excluded", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{
			"a.md": "---\nAuthor: Roman\ncreated: 2024-01-05\n---\n# A\n\nBody.\n",
		}, Params{IncludePropertiesAsBlock: true})

		// then
		page := sink.byKey("a.md")
		require.NotNil(t, page)
		var relationKeys []string
		for _, b := range page.Payload.Blocks {
			if relation := b.GetRelation(); relation != nil {
				relationKeys = append(relationKeys, relation.Key)
			}
		}
		require.Len(t, relationKeys, 1, "created maps to the system createdDate and is excluded")
		assert.NotEqual(t, bundle.RelationKeyCreatedDate.String(), relationKeys[0])
	})
}

func TestEmojiTitle(t *testing.T) {
	t.Run("leading H1 emoji becomes the page icon", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{
			"a.md": "# 🚀 Launch Plan\n\nBody.\n",
		}, Params{})

		// then
		page := sink.byKey("a.md")
		require.NotNil(t, page)
		assert.Equal(t, "🚀", page.Payload.Details.GetString(bundle.RelationKeyIconEmoji))
		assert.Equal(t, "Launch Plan", page.Payload.Details.GetString(bundle.RelationKeyName))
	})
}
