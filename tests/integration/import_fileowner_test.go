package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/objectgc"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// sharedFileFixture: two pages embedding the same image. One file object
// serves both, so exactly one of them can own it.
func sharedFileFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{
		"apage.md": "# APage\n\nFirst ![pic](pic.png)\n\nLink to [BPage](bpage.md)\n",
		"bpage.md": "# BPage\n\nSecond ![pic](pic.png)\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o644))
	}
	copyFile(t, "./testdata/test_image.png", filepath.Join(root, "pic.png"))
	return root
}

// activeObjects returns the space's live pages, collections and files.
func activeObjects(t *testing.T, app *testApplication) []database.Record {
	t.Helper()
	store := getService[objectstore.ObjectStore](app)
	records, err := store.SpaceIndex(app.personalSpaceId()).Query(database.Query{
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_In,
			Value: domain.Int64List([]int64{
				int64(model.ObjectType_basic), int64(model.ObjectType_note), int64(model.ObjectType_todo),
				int64(model.ObjectType_collection), int64(model.ObjectType_image), int64(model.ObjectType_file),
			}),
		}},
	})
	require.NoError(t, err)
	return records
}

func objectIdByNameAndLayout(t *testing.T, app *testApplication, name string, layout model.ObjectTypeLayout) string {
	t.Helper()
	for _, r := range activeObjects(t, app) {
		if r.Details.GetString(bundle.RelationKeyName) == name &&
			model.ObjectTypeLayout(r.Details.GetInt64(bundle.RelationKeyResolvedLayout)) == layout {
			return r.Details.GetString(bundle.RelationKeyId)
		}
	}
	t.Fatalf("no %v named %q in the space", layout, name)
	return ""
}

func detailsOf(t *testing.T, app *testApplication, objectId string) *domain.Details {
	t.Helper()
	store := getService[objectstore.ObjectStore](app)
	details, err := store.SpaceIndex(app.personalSpaceId()).GetDetails(objectId)
	require.NoError(t, err)
	return details
}

// fileBlockId returns the id of the block on pageId whose file block targets
// fileObjectId.
func fileBlockId(t *testing.T, app *testApplication, pageId, fileObjectId string) string {
	t.Helper()
	sb, err := getService[*block.Service](app).GetObject(context.Background(), pageId)
	require.NoError(t, err)
	for _, b := range sb.NewState().Blocks() {
		if f := b.GetFile(); f != nil && f.TargetObjectId == fileObjectId {
			return b.Id
		}
	}
	return ""
}

// waitOutOfActiveSet polls until objectId stops showing up in an
// active-objects query — archiving propagates through the indexer.
func waitOutOfActiveSet(t *testing.T, app *testApplication, objectId string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		found := false
		for _, r := range activeObjects(t, app) {
			if r.Details.GetString(bundle.RelationKeyId) == objectId {
				found = true
			}
		}
		if !found {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("object %s never left the active set", objectId)
}

// TestImportedFileOwnership pins the attachment ownership an import must
// produce: an imported file names the page it was created in and the block
// holding the reference. Object GC gates every cleanup path on that pair, so
// a file without it is invisible to the cleanup the user is offered.
func TestImportedFileOwnership(t *testing.T) {
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)
	indexed := imageIndexedSub(t, app)
	importWithV2(t, app, sharedFileFixture(t))
	indexed.waitOneObjectDetailsSet(t, app, func(t *testing.T, msg *pb.EventObjectDetailsSet) {})

	fileId := objectIdByNameAndLayout(t, app, "pic", model.ObjectType_image)
	aPageId := objectIdByNameAndLayout(t, app, "APage", model.ObjectType_basic)
	bPageId := objectIdByNameAndLayout(t, app, "BPage", model.ObjectType_basic)

	// both pages reference the one file object
	aBlockId := fileBlockId(t, app, aPageId, fileId)
	require.NotEmpty(t, aBlockId, "APage must hold a file block for the image")
	require.NotEmpty(t, fileBlockId(t, app, bPageId, fileId), "BPage references the same file object")

	details := detailsOf(t, app, fileId)
	assert.Equal(t, aPageId, details.GetString(bundle.RelationKeyCreatedInContext),
		"the page converted first owns the shared file")
	assert.Equal(t, aBlockId, details.GetString(bundle.RelationKeyCreatedInContextRef),
		"the ref is the block holding the reference")

	// ownership is not exclusive: the second page keeps the file alive
	gc := getService[objectgc.ObjectGC](app)
	res, err := gc.CheckObjectsOnObjectArchived(app.personalSpaceId(), aPageId, true)
	require.NoError(t, err)
	assert.Empty(t, res.Candidates,
		"a file still shown on another live page must not be offered for cleanup")

	// once nothing references it, it is exactly the orphan the user is offered
	ds := getService[detailservice.Service](app)
	require.NoError(t, ds.SetIsArchived(nil, context.Background(), bPageId, true, true))
	waitOutOfActiveSet(t, app, bPageId)
	res, err = gc.CheckObjectsOnObjectArchived(app.personalSpaceId(), aPageId, true)
	require.NoError(t, err)
	assert.Equal(t, []string{fileId}, res.Candidates,
		"with every referencing page gone the file is a cleanup candidate")
}
