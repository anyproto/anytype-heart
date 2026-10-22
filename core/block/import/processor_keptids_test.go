package importer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

// A converter's KeptIDs (a bundle's declared-deleted targets, SPEC §2c)
// enter the id map as themselves before any object is created, so every
// creation-time rewrite keeps them instead of writing the sentinel.
func TestInitConversionFieldsSeedsKeptIDs(t *testing.T) {
	p := importProcessor{
		deps:    &Dependencies{},
		request: &ImportRequest{RpcObjectImportRequest: &pb.RpcObjectImportRequest{SpaceId: "spaceId"}},
	}

	require.NoError(t, p.initConversionFields(&common.Response{KeptIDs: []string{"gone"}}, nil))

	assert.Equal(t, "gone", p.oldIDToNew["gone"])
}

// A kept id with no row in the destination gets a tombstone, so the client
// renders "deleted" rather than "not found" and a later export classifies it
// as deleted again. A live row is left alone: restoring into the space the
// backup came from re-links the reference instead.
func TestTombstoneKeptIDs(t *testing.T) {
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, "spaceId", []spaceindex.TestObject{{
		bundle.RelationKeyId:      domain.String("live"),
		bundle.RelationKeySpaceId: domain.String("spaceId"),
		bundle.RelationKeyName:    domain.String("Still here"),
	}})
	p := importProcessor{
		deps:    &Dependencies{objectStore: store},
		request: &ImportRequest{RpcObjectImportRequest: &pb.RpcObjectImportRequest{SpaceId: "spaceId"}},
	}
	require.NoError(t, p.initConversionFields(&common.Response{KeptIDs: []string{"gone", "live"}}, nil))

	p.tombstoneKeptIDs(context.Background())

	gone, err := store.SpaceIndex("spaceId").GetDetails("gone")
	require.NoError(t, err)
	assert.True(t, gone.GetBool(bundle.RelationKeyIsDeleted), "no row becomes a tombstone")
	live, err := store.SpaceIndex("spaceId").GetDetails("live")
	require.NoError(t, err)
	assert.False(t, live.GetBool(bundle.RelationKeyIsDeleted), "a live row is not touched")
	assert.Equal(t, "Still here", live.GetString(bundle.RelationKeyName))
}
