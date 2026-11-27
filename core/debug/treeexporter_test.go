package debug

import (
	"archive/zip"
	"context"
	"io"
	stdlog "log"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	anystorev1 "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

// exportedLocalStore runs the real tree export for an object whose index row
// names an app, and returns localstore.json as it sits in the finished ZIP.
func exportedLocalStore(t *testing.T, anonymized bool, appName string) string {
	t.Helper()
	ctx := context.Background()
	const (
		spaceId  = "space1"
		objectId = "0" // the testable tree's own id
	)
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, spaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:              domain.String(objectId),
		bundle.RelationKeyName:            domain.String("Some page"),
		bundle.RelationKeyLastModifiedVia: domain.String(appName),
	}})

	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	aclList, err := list.NewInMemoryDerivedAcl(spaceId, keys)
	require.NoError(t, err)
	changeCreator := objecttree.NewMockChangeCreator(func() anystorev1.DB {
		path := filepath.Join(t.TempDir(), "changes.db")
		db, err := anystorev1.Open(ctx, path, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		return objecttree.TestStore{DB: db, Path: path}
	})
	treeStorage := changeCreator.CreateNewTreeStorage(t, objectId, aclList.Head().Id, false)
	if setter, ok := treeStorage.(interface{ SetAddSeq(seq *atomic.Uint64) }); ok {
		setter.SetAddSeq(&atomic.Uint64{})
	}
	tree, err := objecttree.BuildTestableTree(treeStorage, aclList)
	require.NoError(t, err)

	exporter := &treeExporter{
		log:        stdlog.New(io.Discard, "", 0),
		s:          store,
		anonymized: anonymized,
		id:         domain.FullID{SpaceID: spaceId, ObjectID: objectId},
	}
	zipPath, err := exporter.Export(ctx, t.TempDir(), tree)
	require.NoError(t, err)

	archive, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer archive.Close()
	for _, file := range archive.File {
		if strings.HasSuffix(file.Name, "localstore.json") {
			reader, err := file.Open()
			require.NoError(t, err)
			defer reader.Close()
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			return string(data)
		}
	}
	t.Fatalf("the export holds no localstore.json")
	return ""
}

// The app behind an object's latest change is in its index row, and the row
// is exported with the tree. An anonymized export must not carry the name —
// whatever it is made of.
func TestTreeExport_AnonymizedLocalStoreHoldsNoAppName(t *testing.T) {
	// a name of punctuation only: the generic text scrambler leaves it whole
	const appName = "!!!"

	t.Run("anonymized: the detail is gone from the finished ZIP", func(t *testing.T) {
		// when
		got := exportedLocalStore(t, true, appName)

		// then: red if the row is only passed through the generic
		// anonymizer — "!!!" survives it
		assert.NotContains(t, got, appName)
		assert.NotContains(t, got, bundle.RelationKeyLastModifiedVia.String())
	})

	t.Run("plain: the detail is exported as it is", func(t *testing.T) {
		// when
		got := exportedLocalStore(t, false, appName)

		// then: the control — red if the key is dropped unconditionally, in
		// which case the assertion above would pass for no reason
		assert.Contains(t, got, bundle.RelationKeyLastModifiedVia.String())
		assert.Contains(t, got, appName)
	})
}
