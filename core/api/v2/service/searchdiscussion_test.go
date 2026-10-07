package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

// Search rows carry the object's discussion as their own top-level member,
// as the object read serves it: the parent keeps the id in a hidden relation
// the property vocabulary never serves.
func TestV2SearchRowDiscussion(t *testing.T) {
	ctx := context.Background()

	t.Run("a space search row carries its discussion, a row without one carries none", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		req := v2model.SearchRequest{Type: "page"}
		want := []v2model.ObjectRow{
			{Id: "quiet", Name: "Quiet page", Type: "page", Discussion: "disc-quiet"},
			{Id: "mentioned", Name: "Mentioned page", Type: "page", Discussion: "disc-mentioned"},
			{Id: "page1", Name: "A page", Type: "page"},
		}

		// when
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
	})

	t.Run("the member is top-level on the wire", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		req := v2model.SearchRequest{Filter: "unread_mention_count > 0"}

		// when
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		data, err := json.Marshal(rows)
		require.NoError(t, err)
		assert.JSONEq(t, `[{"id":"mentioned","name":"Mentioned page","type":"page","discussion":"disc-mentioned"}]`, string(data))
	})

	t.Run("fields=discussion is accepted and adds no property", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		req := v2model.SearchRequest{Filter: "unread_mention_count > 0", Fields: []string{"discussion"}}
		want := []v2model.ObjectRow{{Id: "mentioned", Name: "Mentioned page", Type: "page", Discussion: "disc-mentioned"}}

		// when
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
	})

	t.Run("a live property spelled discussion is served under properties beside the member", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-topic"),
			bundle.RelationKeyRelationKey:  domain.String("bsonTopicKey"),
			bundle.RelationKeyApiObjectKey: domain.String("discussion"),
			bundle.RelationKeyName:         domain.String("Discussion"),
		})
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                 domain.String("mentioned"),
			bundle.RelationKeyName:               domain.String("Mentioned page"),
			bundle.RelationKeyType:               domain.String("type-page"),
			bundle.RelationKeyResolvedLayout:     domain.Int64(0),
			bundle.RelationKeyDiscussionId:       domain.String("disc-mentioned"),
			bundle.RelationKeyUnreadMentionCount: domain.Int64(2),
			domain.RelationKey("bsonTopicKey"):   domain.String("roadmap"),
		}})
		req := v2model.SearchRequest{Filter: "unread_mention_count > 0", Fields: []string{"discussion"}}
		want := []v2model.ObjectRow{{Id: "mentioned", Name: "Mentioned page", Type: "page", Discussion: "disc-mentioned",
			Properties: map[string]any{"discussion": "roadmap"}}}

		// when
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
	})

	t.Run("a filter on discussion is still an unknown property", func(t *testing.T) {
		// given: the member is a row member, not a property a query can bind
		fx := bundledKeysSetup(t)
		req := v2model.SearchRequest{Filters: json.RawMessage(`[{"property":"discussion","condition":"not_empty"}]`)}

		// when
		_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

		// then
		requireV2Code(t, err, v2model.CodeValidationFailed)
	})

	t.Run("a global search row carries its discussion", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		req := v2model.SearchRequest{Filter: "unread_mention_count > 0", Fields: []string{"discussion"}}
		want := []v2model.ObjectRow{{Id: "mentioned", Name: "Mentioned page", Type: "page", SpaceId: testSpaceId, Discussion: "disc-mentioned"}}

		// when
		rows, _, _, _, err := fx.GlobalSearchObjects(ctx, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
	})
}
