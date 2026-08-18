package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	importv2adapter "github.com/anyproto/anytype-heart/core/block/importv2/adapter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestImportV2Markdown drives a real markdown import through the v2 engine
// end-to-end: real app, real space, real file upload, async run with the
// EventImportFinish completion signal.
func TestImportV2Markdown(t *testing.T) {
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)

	fileSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId}, []database.FilterRequest{
		filterEqualsToInteger(bundle.RelationKeyFileIndexingStatus, model.FileIndexingStatus_Indexed),
		filterEqualsToString(bundle.RelationKeyName, "saturn"),
		filterEqualsToString(bundle.RelationKeyFileMimeType, "image/jpeg"),
		filterNotEmpty(bundle.RelationKeyFileId),
	})

	getService[*config.Config](app).ImportV2Markdown = true
	v2 := getService[importv2adapter.Importer](app)
	require.True(t, v2.Handles(model.Import_Markdown), "flag must route markdown to v2")
	require.True(t, v2.Handles(model.Import_Obsidian))
	require.False(t, v2.Handles(model.Import_Pb), "other formats stay on v1")

	// when — async import, exactly as the gRPC handler invokes it
	v2.Import(&pb.RpcObjectImportRequest{
		SpaceId: app.personalSpaceId(),
		Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
		Type:    model.Import_Markdown,
		Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
			MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
				Path: []string{"./testdata/import/markdown with files/"},
			},
		},
	})

	// then — completion event carries the root collection and object count
	var rootCollectionId string
	app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
		if v := msg.GetImportFinish(); v != nil {
			rootCollectionId = v.RootCollectionID
			assert.Positive(t, v.ObjectsCount)
			assert.Equal(t, model.Import_Markdown, v.ImportType)
			return true
		}
		return false
	})
	assert.NotEmpty(t, rootCollectionId)

	// and the referenced image went through a real upload + indexing
	fileSub.waitOneObjectDetailsSet(t, app, func(t *testing.T, msg *pb.EventObjectDetailsSet) {
		fileObjectId := msg.Details.GetFields()[bundle.RelationKeyId.String()].GetStringValue()
		assertImageAvailableInGateway(t, app, fileObjectId)
	})
}

const dataviewTaskSchema = `{
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
			"x-featured": true,
			"enum": ["High", "Low"]
		},
		"Assignee": {
			"type": "string",
			"x-key": "task_assignee",
			"x-format": "shorttext"
		}
	}
}`

// TestImportV2TypeDataviewColumns inspects what a user sees when they open an
// imported type: the type's own dataview block, and which of its properties
// are columns there.
func TestImportV2TypeDataviewColumns(t *testing.T) {
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task.schema.json"), []byte(dataviewTaskSchema), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "work.md"),
		[]byte("---\nPriority: High\nAssignee: Roman\ntype: Task\n---\n# Work\n\nBody.\n"), 0o644))

	getService[*config.Config](app).ImportV2Markdown = true
	v2 := getService[importv2adapter.Importer](app)
	v2.Import(markdownRequest(app.personalSpaceId(), dir))
	app.waitEventMessage(t, func(msg *pb.EventMessage) bool { return msg.GetImportFinish() != nil })

	typeId := objectIdByName(t, app, "Task", model.ObjectType_objectType)
	sb, err := getService[*block.Service](app).GetObject(context.Background(), typeId)
	require.NoError(t, err)
	st := sb.NewState()

	// The plan's properties resolved to real relation object ids — the input
	// the dataview template reads.
	details := st.Details()
	require.Len(t, details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations), 1)
	require.Len(t, details.GetStringList(bundle.RelationKeyRecommendedRelations), 1)

	blk := st.Pick(state.DataviewBlockID)
	require.NotNil(t, blk, "an imported type must carry its own dataview block")
	dv := blk.Model().GetDataview()
	require.NotNil(t, dv)
	require.Len(t, dv.Views, 1)

	var visible, hidden []string
	for _, rel := range dv.Views[0].Relations {
		if rel.IsVisible {
			visible = append(visible, rel.Key)
		} else {
			hidden = append(hidden, rel.Key)
		}
	}
	t.Logf("VISIBLE columns: %v", visible)
	t.Logf("HIDDEN columns:  %v", hidden)

	// The type's own properties are the columns. Before the fix every one of
	// them was listed but switched off, so an imported type opened as a bare
	// Name column and the whole schema looked lost.
	assert.ElementsMatch(t, []string{"name", "task_priority", "task_assignee"}, visible)
	assert.NotContains(t, hidden, "task_priority")
	assert.NotContains(t, hidden, "task_assignee")
}

// objectIdByName finds one object of the given layout by display name.
func objectIdByName(t *testing.T, app *testApplication, name string, layout model.ObjectTypeLayout) string {
	t.Helper()
	store := getService[objectstore.ObjectStore](app)
	records, err := store.SpaceIndex(app.personalSpaceId()).Query(database.Query{
		Filters: []database.FilterRequest{
			filterEqualsToString(bundle.RelationKeyName, name),
			filterEqualsToInteger(bundle.RelationKeyResolvedLayout, int64(layout)),
		},
	})
	require.NoError(t, err)
	require.Len(t, records, 1, "exactly one %q object expected", name)
	return records[0].Details.GetString(bundle.RelationKeyId)
}
