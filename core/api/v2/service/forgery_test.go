package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// The document body is an input channel like any other (review cause 1):
// identity-bearing details must never ride a properties map into the create
// RPC. A forged uniqueKey reaches getUniqueKeyOrGenerate verbatim and
// DERIVES the bundled type's object id — strategy (b)'s silent merge,
// reachable under (a) through a channel the union check never inspects.

func TestV2TypeDocumentForgery(t *testing.T) {
	t.Run("a forged uniqueKey is rejected, path-addressed", func(t *testing.T) {
		// given: no create expectation — reaching the RPC fails the test
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","key":"forged","properties":{"name":"Forged","uniqueKey":"ot-page"}}`), false)

		// then
		apiErr := v2ErrWithIssue(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/properties/uniqueKey", apiErr.Issues[0].Path)
	})

	t.Run("relationKey, isReadonly and restrictions are rejected too", func(t *testing.T) {
		fx := newV2Fixture(t)
		for _, forged := range []string{"relationKey", "isReadonly", "restrictions"} {
			_, err := fx.CreateType(context.Background(), testSpaceId,
				[]byte(`{"kind":"object_type","key":"forged2","properties":{"name":"Forged","`+forged+`":"x"}}`), false)
			apiErr := v2ErrWithIssue(t, err)
			require.NotEmpty(t, apiErr.Issues, forged)
			assert.Equal(t, "/properties/"+forged, apiErr.Issues[0].Path)
		}
	})

	t.Run("a document-supplied apiObjectKey is dropped, never trusted", func(t *testing.T) {
		// the slug is derived from the key/name and union-checked; a
		// document-supplied value would bypass the check ("object_type"
		// would shadow the bundled type slug)
		fx := newV2Fixture(t)
		var captured *pb.RpcObjectCreateObjectTypeRequest
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
				captured = req
				return &pb.RpcObjectCreateObjectTypeResponse{
					ObjectId: "type-clean",
					Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
				}
			})
		fx.expectEtagRead("type-clean")

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","key":"cleantype","properties":{"name":"Clean","apiObjectKey":"object_type"}}`), false)

		// then: the checked slug wins; the forged one is gone
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Equal(t, "cleantype", pbtypes.GetString(captured.Details, bundle.RelationKeyApiObjectKey.String()))

		// the sharp case: no key, a name that slugs to nothing — before the
		// drop, the forged value was the ONLY apiObjectKey and it landed
		// unchecked on the bundled slug
		captured = nil
		fx2 := newV2Fixture(t)
		fx2.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
				captured = req
				return &pb.RpcObjectCreateObjectTypeResponse{
					ObjectId: "type-clean2",
					Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
				}
			})
		fx2.expectEtagRead("type-clean2")
		_, err = fx2.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"☕","apiObjectKey":"object_type"}}`), false)
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Empty(t, pbtypes.GetString(captured.Details, bundle.RelationKeyApiObjectKey.String()))
	})

	t.Run("an object document must not carry an envelope key", func(t *testing.T) {
		// the same forgery through the second channel: doc.Key becomes
		// snapshot.Key becomes uniqueKeyInternal becomes DeriveTreeObject
		fx := newV2Fixture(t)

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"key":"page","blocks":[{"type":"paragraph","text":"x"}]}`), false)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/key", apiErr.Issues[0].Path)
	})
}
