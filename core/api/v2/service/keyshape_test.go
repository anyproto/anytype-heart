package v2service

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestParseKeysShape(t *testing.T) {
	for _, raw := range []string{"", V2KeysSlug} {
		names, err := ParseKeysShape(raw)
		require.NoError(t, err)
		assert.False(t, names, "%q is the slug default", raw)
	}
	names, err := ParseKeysShape(V2KeysName)
	require.NoError(t, err)
	assert.True(t, names)

	_, err = ParseKeysShape("display")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid keys value")
}

// TestKeysShapeOnRead pins §4.2's whole point: one read, two spellings, and
// the choice is the request's — never a re-spelling of a marshaled body.
func TestKeysShapeOnRead(t *testing.T) {
	fx := newV2Fixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-awareness"),
		bundle.RelationKeyRelationKey:  domain.String("bsonAwareKey"),
		bundle.RelationKeyApiObjectKey: domain.String("discovery"),
		bundle.RelationKeyName:         domain.String("Awareness"),
	})
	read := apicore.ObjectRead{
		SbType: model.SmartBlockType_Page,
		Snapshot: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"name":         pbtypes.String("Doc"),
				"bsonAwareKey": pbtypes.String("high"),
			}},
		},
		Heads: []string{"h1"},
	}
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)

	t.Run("the default spells the slug", func(t *testing.T) {
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", ObjectQuery{})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"discovery"`)
		assert.NotContains(t, string(body), `"Awareness":`,
			"the name is data (the name property), never a key spelling here")
	})

	t.Run("?keys=name spells the display name", func(t *testing.T) {
		body, _, err := fx.GetObject(CtxWithNameKeys(context.Background()), testSpaceId, "obj1", ObjectQuery{})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"Awareness"`,
			"the format's own vocabulary — the resolver unwrapped")
		assert.NotContains(t, string(body), `"discovery"`)
	})
}
