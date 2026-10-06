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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// addDiscussionParents registers two objects carrying the discussion-parent
// counters a discussion writes onto its parent as local details. The space
// holds no relation object for either counter, as a real space does not.
func (fx *v2Fixture) addDiscussionParents(t *testing.T) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:                 domain.String("mentioned"),
			bundle.RelationKeyName:               domain.String("Mentioned page"),
			bundle.RelationKeyType:               domain.String("type-page"),
			bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastModifiedDate:   domain.Int64(4000),
			bundle.RelationKeyDiscussionId:       domain.String("disc-mentioned"),
			bundle.RelationKeyUnreadMentionCount: domain.Int64(2),
			bundle.RelationKeyUnreadMessageCount: domain.Int64(3),
		},
		{
			bundle.RelationKeyId:                 domain.String("quiet"),
			bundle.RelationKeyName:               domain.String("Quiet page"),
			bundle.RelationKeyType:               domain.String("type-page"),
			bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastModifiedDate:   domain.Int64(5000),
			bundle.RelationKeyDiscussionId:       domain.String("disc-quiet"),
			bundle.RelationKeyUnreadMentionCount: domain.Int64(0),
			bundle.RelationKeyUnreadMessageCount: domain.Int64(7),
		},
	})
}

func bundledKeysSetup(t *testing.T) *v2Fixture {
	fx := searchSetup(t)
	fx.addDiscussionParents(t)
	return fx
}

func TestV2SearchBundledQueryKeys(t *testing.T) {
	ctx := context.Background()

	for _, spelling := range []string{"unreadMentionCount", "unread_mention_count"} {
		t.Run("a structured filter on "+spelling+" finds the mentioned object", func(t *testing.T) {
			// given
			fx := bundledKeysSetup(t)
			req := v2model.SearchRequest{
				Filters: json.RawMessage(`[{"property":"` + spelling + `","condition":"greater","value":0}]`),
			}
			want := []string{"mentioned"}

			// when
			rows, total, _, warnings, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

			// then
			require.NoError(t, err)
			assert.Empty(t, warnings)
			assert.Equal(t, 1, total)
			assert.Equal(t, want, rowIds(rows))
		})

		t.Run("the filter string on "+spelling+" finds the mentioned object", func(t *testing.T) {
			// given
			fx := bundledKeysSetup(t)
			req := v2model.SearchRequest{Filter: spelling + " > 0"}
			want := []string{"mentioned"}

			// when
			rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

			// then
			require.NoError(t, err)
			assert.Equal(t, want, rowIds(rows))
		})
	}

	t.Run("a sort on unread_message_count orders by the counter", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		req := v2model.SearchRequest{
			Filters: json.RawMessage(`[{"property":"unread_message_count","condition":"greater","value":0}]`),
			Sorts:   json.RawMessage(`[{"property":"unread_message_count","direction":"desc"}]`),
		}
		want := []string{"quiet", "mentioned"}

		// when
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rowIds(rows))
	})

	t.Run("a listed field serves the counter as a number, under the requested spelling", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		req := v2model.SearchRequest{
			Filter: "unread_mention_count > 0",
			Fields: []string{"unread_mention_count", "unreadMessageCount"},
		}
		want := map[string]any{"unread_mention_count": float64(2), "unreadMessageCount": float64(3)}

		// when
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, want, jsonRoundTrip(t, rows[0].Properties))
	})

	t.Run("global search takes the counter too", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		req := v2model.SearchRequest{Filter: "unread_mention_count > 0", Fields: []string{"unread_mention_count"}}
		want := []string{"mentioned"}

		// when
		rows, _, _, warnings, err := fx.GlobalSearchObjects(ctx, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.Equal(t, want, rowIds(rows))
		assert.Equal(t, map[string]any{"unread_mention_count": float64(2)}, jsonRoundTrip(t, rows[0].Properties))
	})

	t.Run("a default row carries no counter", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		req := v2model.SearchRequest{Filter: "unread_mention_count > 0"}

		// when
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Empty(t, rows[0].Properties)
	})

	t.Run("a refusal neither lists nor suggests the hidden counters", func(t *testing.T) {
		for name, req := range map[string]v2model.SearchRequest{
			"structured filter": {Filters: json.RawMessage(`[{"property":"unread_mention","condition":"greater","value":0}]`)},
			"filter string":     {Filter: "unread_mention > 0"},
			"field":             {Fields: []string{"unread_mention"}},
			"sort":              {Sorts: json.RawMessage(`[{"property":"unread_mention","direction":"desc"}]`)},
		} {
			t.Run(name, func(t *testing.T) {
				// given
				fx := bundledKeysSetup(t)

				// when
				_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

				// then
				apiErr := v2Err(t, err)
				require.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
				body, marshalErr := json.Marshal(apiErr)
				require.NoError(t, marshalErr)
				assert.Contains(t, string(body), "known property keys")
				for _, hidden := range []string{"unread_mention_count", "unreadMentionCount", "unread_message_count", "unreadMessageCount"} {
					assert.NotContains(t, string(body), hidden)
				}
			})
		}
	})
}

func TestV2SetViewBundledQueryKeys(t *testing.T) {
	ctx := context.Background()

	t.Run("a list read takes the counter in fields", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)
		want := map[string]any{"mentioned": float64(2), "quiet": float64(0)}

		// when
		rows, _, _, err := fx.ListObjects(ctx, testSpaceId, []string{"unread_mention_count"}, 0, 25)

		// then
		require.NoError(t, err)
		got := map[string]any{}
		for _, row := range rows {
			if value, ok := row.Properties["unread_mention_count"]; ok {
				got[row.Id] = jsonRoundTrip(t, value)
			}
		}
		assert.Equal(t, want, got)
	})

	t.Run("a list read refusal does not list the counters", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)

		// when
		_, _, _, err := fx.ListObjects(ctx, testSpaceId, []string{"unread_mention"}, 0, 25)

		// then
		apiErr := v2Err(t, err)
		body, marshalErr := json.Marshal(apiErr)
		require.NoError(t, marshalErr)
		assert.NotContains(t, string(body), "unread_mention_count")
		assert.NotContains(t, string(body), "unreadMentionCount")
	})

	for name, req := range map[string]v2model.CreateQueryRequest{
		"structured filter, served spelling": {Filters: json.RawMessage(`[{"property":"unread_mention_count","condition":"greater","value":0}]`)},
		"structured filter, stored spelling": {Filters: json.RawMessage(`[{"property":"unreadMentionCount","condition":"greater","value":0}]`)},
		"filter string":                      {Filter: "unread_mention_count > 0"},
	} {
		t.Run("a query stores a "+name+" on the counter under its stored key", func(t *testing.T) {
			// given
			fx := bundledKeysSetup(t)
			captured := fx.expectCreate("newSet")
			fx.expectEtagRead("newSet")
			req.Name, req.Type = "Mentioned chores", "chore"

			// when
			_, err := fx.CreateQuery(ctx, testSpaceId, req, false, true)

			// then
			require.NoError(t, err)
			dv := (*captured).Blocks[1].GetDataview()
			require.Len(t, dv.Views, 1)
			require.Len(t, dv.Views[0].Filters, 1)
			assert.Equal(t, "unreadMentionCount", dv.Views[0].Filters[0].RelationKey)
			assert.Equal(t, model.BlockContentDataviewFilter_Greater, dv.Views[0].Filters[0].Condition)
		})
	}

	t.Run("a query sorts on the counter", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)

		// when
		result, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{
			Name: "Busiest chores", Type: "chore",
			Sorts: json.RawMessage(`[{"property":"unread_message_count","direction":"desc"}]`),
		}, true, true)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
	})

	t.Run("a query's filter-string refusal does not list the counters", func(t *testing.T) {
		// given
		fx := bundledKeysSetup(t)

		// when
		_, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{
			Name: "X", Type: "chore", Filter: "unread_mention > 0",
		}, true, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, "known property keys")
		assert.NotContains(t, apiErr.Issues[0].Message, "unread_mention_count")
		assert.NotContains(t, apiErr.Issues[0].Message, "unreadMentionCount")
		assert.NotContains(t, apiErr.Issues[0].Hint, "unread_mention_count")
	})
}

// jsonRoundTrip serves a value the way the wire does, so number types compare
// as a client decodes them.
func jsonRoundTrip(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	var out any
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}
