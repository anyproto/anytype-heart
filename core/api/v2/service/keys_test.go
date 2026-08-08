package v2service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The §7.5-requirement-2 corpse policy: an archived (v2 delete) or
// uninstalled (UI delete) type/property must neither list, nor resolve as an
// address, nor be suggested as a remedy. Before this landed, isUninstalled
// was filtered NOWHERE (ADDRESSING §2.3-6): a UI-deleted property still
// appeared in GET /properties and PATCH steered callers into editing a
// corpse.

// addRelation registers one relation object in the store fixture.
func (fx *v2Fixture) addRelation(t *testing.T, spaceId string, obj objectstore.TestObject) {
	base := objectstore.TestObject{
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
	}
	for k, v := range obj {
		base[k] = v
	}
	fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{base})
}

// addType registers one type object in the store fixture.
func (fx *v2Fixture) addType(t *testing.T, spaceId string, obj objectstore.TestObject) {
	base := objectstore.TestObject{
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}
	for k, v := range obj {
		base[k] = v
	}
	fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{base})
}

func corpsePolicyFixture(t *testing.T) *v2Fixture {
	fx := newV2Fixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:          domain.String("rel-live"),
		bundle.RelationKeyRelationKey: domain.String("liveKey"),
		bundle.RelationKeyName:        domain.String("Live property"),
	})
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:            domain.String("rel-uninstalled"),
		bundle.RelationKeyRelationKey:   domain.String("corpseKey"),
		bundle.RelationKeyName:          domain.String("UI-deleted property"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	})
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:          domain.String("rel-archived"),
		bundle.RelationKeyRelationKey: domain.String("archivedKey"),
		bundle.RelationKeyName:        domain.String("v2-deleted property"),
		bundle.RelationKeyIsArchived:  domain.Bool(true),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:        domain.String("type-live"),
		bundle.RelationKeyUniqueKey: domain.String("ot-livetype"),
		bundle.RelationKeyName:      domain.String("Live type"),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:            domain.String("type-uninstalled"),
		bundle.RelationKeyUniqueKey:     domain.String("ot-corpsetype"),
		bundle.RelationKeyName:          domain.String("UI-deleted type"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	})
	return fx
}

func requireNotFoundError(t *testing.T, err error) *v2model.Error {
	t.Helper()
	var apiErr *v2model.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.Status)
	return apiErr
}

func TestV2CorpsePolicyProperties(t *testing.T) {
	t.Run("uninstalled and archived properties do not list", func(t *testing.T) {
		// given
		fx := corpsePolicyFixture(t)

		// when
		rows, total, _, err := fx.ListProperties(context.Background(), testSpaceId, 0, 25)

		// then: only the live property — before the fix the uninstalled one
		// passed every filter (nothing anywhere excluded isUninstalled)
		require.NoError(t, err)
		keys := make([]string, 0, len(rows))
		for _, row := range rows {
			keys = append(keys, row.Key)
		}
		assert.Equal(t, []string{"liveKey"}, keys)
		assert.Equal(t, 1, total)
	})

	t.Run("known property keys never suggest a corpse", func(t *testing.T) {
		fx := corpsePolicyFixture(t)
		assert.Equal(t, []string{"liveKey"}, fx.knownPropertyKeys(testSpaceId))
	})

	t.Run("PATCH of a UI-deleted property is 404, not a corpse edit", func(t *testing.T) {
		// given
		fx := corpsePolicyFixture(t)
		name := "renamed"

		// when: before the fix GetRelationByKey saw the uninstalled relation
		// and the PATCH proceeded against the object the user deleted
		_, err := fx.UpdateProperty(context.Background(), testSpaceId, "corpseKey", v2model.UpdatePropertyRequest{Name: &name}, false)

		// then
		requireNotFoundError(t, err)
	})

	t.Run("DELETE of a UI-deleted property is 404, not a re-archive", func(t *testing.T) {
		// the UNINSTALLED corpse is the revert-sensitive case: the old
		// GetRelationByKey lookup saw it (nothing filtered isUninstalled)
		// and the DELETE archived the corpse. (The archived variant cannot
		// fail on revert — the store's injected defaults hid archived
		// objects from the old lookup too — so it is asserted only as a
		// bonus line, not as this subtest's claim.)
		fx := corpsePolicyFixture(t)
		_, err := fx.DeleteProperty(context.Background(), testSpaceId, "corpseKey", false)
		requireNotFoundError(t, err)
		_, err = fx.DeleteProperty(context.Background(), testSpaceId, "archivedKey", false)
		requireNotFoundError(t, err)
	})

	t.Run("options of a corpse property are 404", func(t *testing.T) {
		fx := corpsePolicyFixture(t)
		_, _, _, err := fx.ListPropertyOptions(context.Background(), testSpaceId, "corpseKey", "", 0, 25)
		requireNotFoundError(t, err)
	})

	t.Run("a live property still resolves on every touched route", func(t *testing.T) {
		fx := corpsePolicyFixture(t)
		_, _, _, err := fx.ListPropertyOptions(context.Background(), testSpaceId, "liveKey", "", 0, 25)
		assert.NoError(t, err)
	})
}

func TestV2CorpsePolicyTypes(t *testing.T) {
	t.Run("uninstalled types do not list", func(t *testing.T) {
		// given
		fx := corpsePolicyFixture(t)

		// when
		rows, total, _, err := fx.ListTypes(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		keys := make([]string, 0, len(rows))
		for _, row := range rows {
			keys = append(keys, row.Key)
		}
		assert.Equal(t, []string{"livetype"}, keys)
		assert.Equal(t, 1, total)
	})

	t.Run("GET of a UI-deleted type is 404 with live candidates", func(t *testing.T) {
		fx := corpsePolicyFixture(t)
		_, _, err := fx.GetType(context.Background(), testSpaceId, "corpsetype")
		apiErr := requireNotFoundError(t, err)
		assert.Contains(t, apiErr.Message, "livetype")
		assert.NotContains(t, apiErr.Message, "corpsetype: ")
	})

	t.Run("PATCH and DELETE of a UI-deleted type are 404", func(t *testing.T) {
		fx := corpsePolicyFixture(t)
		_, err := fx.UpdateType(context.Background(), testSpaceId, "corpsetype", []byte(`{"properties":{"name":"x"}}`), false)
		requireNotFoundError(t, err)
		_, err = fx.DeleteType(context.Background(), testSpaceId, "corpsetype", false)
		requireNotFoundError(t, err)
	})

	t.Run("search scoped to a corpse type is a loud 400", func(t *testing.T) {
		// given
		fx := corpsePolicyFixture(t)

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, v2model.SearchRequest{Type: "corpsetype"}, 0, 25)

		// then: unknown-type steering, live keys only
		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	})
}

func TestSanitizeApiSlug(t *testing.T) {
	// derived slugs (from names and document keys — inputs no pattern ever
	// checked) must land inside the advertised key grammar or not exist:
	// before this, "50% done" and "☕" (unidecode: "?") became
	// identity-bearing apiObjectKey values no /properties/{key} route could
	// accept
	cases := map[string]string{
		"50%_done":  "50_done",
		"c++":       "c",
		"?":         "",
		"foo/bar":   "foo_bar",
		"a__b":      "a_b",
		"_x_":       "x",
		"clean_key": "clean_key",
		"":          "",
	}
	for in, want := range cases {
		assert.Equal(t, want, sanitizeApiSlug(in), "input %q", in)
	}
}
