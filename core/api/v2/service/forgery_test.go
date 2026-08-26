package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
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
			[]byte(`{"kind":"object_type","properties":{"name":"Forged","uniqueKey":"ot-page"},"type_settings":{"api_key":"forged"}}`), false, true)

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
				[]byte(`{"kind":"object_type","type_settings":{"api_key":"forged2"},"properties":{"name":"Forged","`+forged+`":"x"}}`), false, true)
			apiErr := v2ErrWithIssue(t, err)
			require.NotEmpty(t, apiErr.Issues, forged)
			assert.Equal(t, "/properties/"+forged, apiErr.Issues[0].Path)
		}
	})

	t.Run("a document-supplied apiObjectKey is refused, never trusted", func(t *testing.T) {
		// the slug is derived from the api_key/name and union-checked; a
		// document-supplied value would bypass that check ("object_type"
		// would shadow the bundled type slug).
		//
		// §2a closed this one layer earlier and harder than the API's own
		// drop did: apiObjectKey is a type_settings member now, so the
		// format REFUSES it in `properties` instead of the create silently
		// dropping it. The invariant is unchanged — a forged slug never
		// reaches the mint — so this asserts the refusal rather than the
		// value that used to survive it. No create expectation: reaching the
		// RPC at all fails the test.
		for _, body := range []string{
			`{"kind":"object_type","properties":{"name":"Clean","apiObjectKey":"object_type"},"type_settings":{"api_key":"cleantype"}}`,
			// the sharp case: no api_key, and a name that slugs to nothing,
			// so the forged value would have been the ONLY apiObjectKey
			`{"kind":"object_type","properties":{"name":"☕","apiObjectKey":"object_type"}}`,
		} {
			fx := newV2Fixture(t)
			_, err := fx.CreateType(context.Background(), testSpaceId, []byte(body), false, true)

			apiErr := v2ErrWithIssue(t, err)
			require.NotEmpty(t, apiErr.Issues)
			assert.Equal(t, "/properties/apiObjectKey", apiErr.Issues[0].Path)
			assert.Contains(t, apiErr.Issues[0].Message, "type_settings")
		}
	})
	t.Run("an object document must not carry an envelope key", func(t *testing.T) {
		// the same forgery through the second channel: doc.Key becomes
		// snapshot.Key becomes uniqueKeyInternal becomes DeriveTreeObject
		fx := newV2Fixture(t)

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"key":"page","blocks":[{"type":"paragraph","text":"x"}]}`), false, true)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/key", apiErr.Issues[0].Path)
	})
}
