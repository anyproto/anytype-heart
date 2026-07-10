package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	importv2adapter "github.com/anyproto/anytype-heart/core/block/importv2/adapter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// parityProjection is the engine-independent view of an imported space:
// what the user sees, with ids/keys/timestamps (which legitimately differ
// between engines) projected away.
type parityProjection struct {
	PageNames       []string
	CollectionCount int
	RelationNames   []string
	OptionNames     []string
	FileNames       []string
}

// TestImportParityMarkdown imports one fixture through both engines into
// separate accounts and compares the semantic projections. Intended
// differences are documented inline — everything else must match.
func TestImportParityMarkdown(t *testing.T) {
	fixture := buildParityFixture(t)

	v1App := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)
	v1Files := imageIndexedSub(t, v1App)
	importWithV1(t, v1App, fixture)
	v1Files.waitOneObjectDetailsSet(t, v1App, func(t *testing.T, msg *pb.EventObjectDetailsSet) {})
	v1 := projectionOf(t, v1App)

	v2App := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)
	v2Files := imageIndexedSub(t, v2App)
	importWithV2(t, v2App, fixture)
	v2Files.waitOneObjectDetailsSet(t, v2App, func(t *testing.T, msg *pb.EventObjectDetailsSet) {})
	v2 := projectionOf(t, v2App)

	// Pages and files must match exactly.
	assert.Equal(t, v1.PageNames, v2.PageNames, "page set")
	assert.Equal(t, v1.FileNames, v2.FileNames, "file set")
	// Collections: the csv sub-collection plus the dated root collection.
	assert.Equal(t, v1.CollectionCount, v2.CollectionCount, "collection count")
	// Custom relations/options by display name (keys differ by design:
	// v1 mints random bson ids, v2 derives stable hashes).
	assert.Equal(t, v1.RelationNames, v2.RelationNames, "custom relation set")
	assert.Equal(t, v1.OptionNames, v2.OptionNames, "option set")
}

func buildParityFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"index.md":    "---\nAuthor: Roman\nMood: [happy, calm]\n---\n# Home\n\nSee [Note](sub/note.md) and ![pic](assets/pic.png)\n",
		"sub/note.md": "# Note\n\nBack to [Home](../index.md)\n",
		"Tasks.csv":   "Name\n",
		"Tasks/a.md":  "# TaskA\n",
	}
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "assets"), 0o755))
	copyFile(t, "./testdata/test_image.png", filepath.Join(root, "assets", "pic.png"))
	return root
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from)
	require.NoError(t, err)
	defer src.Close()
	dst, err := os.Create(to)
	require.NoError(t, err)
	defer dst.Close()
	_, err = io.Copy(dst, src)
	require.NoError(t, err)
}

func markdownRequest(spaceId, path string) *pb.RpcObjectImportRequest {
	return &pb.RpcObjectImportRequest{
		SpaceId: spaceId,
		Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
		Type:    model.Import_Markdown,
		Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
			MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{path}},
		},
	}
}

// imageIndexedSub arms (before the import — uploads are async in v1 and
// in-run in v2) a wait for the fixture image to finish upload + indexing,
// so both projections observe the same final state.
func imageIndexedSub(t *testing.T, app *testApplication) *testSubscription {
	t.Helper()
	return newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId}, []database.FilterRequest{
		filterEqualsToInteger(bundle.RelationKeyFileIndexingStatus, model.FileIndexingStatus_Indexed),
		filterEqualsToString(bundle.RelationKeyName, "pic"),
		filterNotEmpty(bundle.RelationKeyFileId),
	})
}

func importWithV1(t *testing.T, app *testApplication, fixture string) {
	t.Helper()
	importerService := getService[importer.Importer](app)
	res := importerService.Import(context.Background(), &importer.ImportRequest{
		RpcObjectImportRequest: markdownRequest(app.personalSpaceId(), fixture),
		Origin:                 objectorigin.Import(model.Import_Markdown),
		IsSync:                 true,
	})
	require.NoError(t, res.Err)
}

func importWithV2(t *testing.T, app *testApplication, fixture string) {
	t.Helper()
	getService[*config.Config](app).ImportV2Markdown = true
	v2 := getService[importv2adapter.Importer](app)
	require.True(t, v2.Handles(model.Import_Markdown))
	v2.Import(markdownRequest(app.personalSpaceId(), fixture))
	app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
		return msg.GetImportFinish() != nil
	})
}

// projectionOf reads the imported object set back from the store.
func projectionOf(t *testing.T, app *testApplication) parityProjection {
	t.Helper()
	store := getService[objectstore.ObjectStore](app)
	records, err := store.SpaceIndex(app.personalSpaceId()).Query(database.Query{
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyOrigin,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectOrigin_import)),
		}},
	})
	require.NoError(t, err)

	projection := parityProjection{}
	for _, record := range records {
		name := record.Details.GetString(bundle.RelationKeyName)
		if strings.HasPrefix(name, "Import report — ") {
			// v2-only diagnostic page (§16 item 1), deliberately absent in v1.
			continue
		}
		layout := model.ObjectTypeLayout(record.Details.GetInt64(bundle.RelationKeyResolvedLayout))
		switch layout {
		case model.ObjectType_basic, model.ObjectType_todo, model.ObjectType_note:
			projection.PageNames = append(projection.PageNames, name)
		case model.ObjectType_collection:
			projection.CollectionCount++
		case model.ObjectType_relation:
			if !bundle.HasRelation(domain.RelationKey(record.Details.GetString(bundle.RelationKeyRelationKey))) {
				projection.RelationNames = append(projection.RelationNames, name)
			}
		case model.ObjectType_relationOption:
			projection.OptionNames = append(projection.OptionNames, name)
		case model.ObjectType_image, model.ObjectType_file:
			projection.FileNames = append(projection.FileNames, name)
		}
	}
	sort.Strings(projection.PageNames)
	sort.Strings(projection.RelationNames)
	sort.Strings(projection.OptionNames)
	sort.Strings(projection.FileNames)
	t.Logf("projection: %s", formatProjection(projection))
	return projection
}

func formatProjection(p parityProjection) string {
	return fmt.Sprintf("pages=%v collections=%d relations=%v options=%v files=%v",
		p.PageNames, p.CollectionCount, p.RelationNames, p.OptionNames, p.FileNames)
}
