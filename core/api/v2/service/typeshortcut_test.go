package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gogo/protobuf/types"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The flat body is the shape a caller sends before reading anything. It is
// reproduced here from a real MCP session that failed on it.
func TestV2TypeFlatBody(t *testing.T) {
	t.Run("the shape a caller guesses is accepted", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		body := `{"name":"Plant","plural_name":"Plants","icon":{"format":"emoji","emoji":"x"},` +
			`"layout":"basic","property_definitions":[{"property":"Location","name":"Location","format":"select"}]}`

		// when
		result, err := fx.CreateType(context.Background(), testSpaceId, []byte(body), true, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "plant", result.Key, "the api key is derived from the name")
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Properties, 1)
		assert.Equal(t, "location", result.Created.Properties[0].Key)
	})

	t.Run("the AnyBlock document still works", func(t *testing.T) {
		// the flat form is an addition, not a replacement: a caller echoing a
		// GetType read back into POST must keep working
		// given
		fx := newV2Fixture(t)
		body := `{"formatVersion":"2.0","kind":"object_type","properties":{"name":"Plant"},` +
			`"type_settings":{"api_key":"plant"}}`

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId, []byte(body), true, false)

		// then
		require.NoError(t, err)
	})

	t.Run("properties as an array names the rename, not an unknown key", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		body := `{"name":"Plant","layout":"basic","properties":[{"name":"Location"}]}`

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId, []byte(body), true, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/properties", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "property_definitions")
	})

	t.Run("an unknown field says how to send a document instead", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"name":"Plant","colour":"green"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/colour", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "formatVersion")
	})

	t.Run("the same flat body patches an existing type", func(t *testing.T) {
		// create and update taking different shapes for one resource is what
		// cost a real caller three round trips
		// given
		fields, err := parseEnvelope([]byte(`{"plural_name":"Plants","layout":"todo"}`))
		require.NoError(t, err)

		// when
		patch, err := typeShortcutPatch(fields)

		// then
		require.NoError(t, err)
		assert.Contains(t, patch, "type_settings")
		assert.NotContains(t, patch, "properties", "a patch that names no name must not rewrite one")
	})

	t.Run("a patch that changes nothing is refused", func(t *testing.T) {
		// given
		fields, err := parseEnvelope([]byte(`{}`))
		require.NoError(t, err)

		// when
		_, err = typeShortcutPatch(fields)

		// then
		require.Error(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, v2Err(t, err).Code)
	})
}

// TestV2TypeSchemaIsTheBodyTheEndpointTakes closes the loop the discovery
// endpoint opens: GET /v2/schemas/type hands a caller a schema and a worked
// example, and the only thing that makes either worth serving is that POST
// /types accepts what they describe. Validating the example against its own
// schema cannot catch the drift that matters — a schema both halves agree on
// and the runtime refuses.
func TestV2TypeSchemaIsTheBodyTheEndpointTakes(t *testing.T) {
	t.Run("the served example is accepted by the endpoint it names", func(t *testing.T) {
		// given: exactly what a caller reads from GET /v2/schemas/type
		fx := newV2Fixture(t)
		entry, err := fx.SchemaKind("type")
		require.NoError(t, err)
		require.Equal(t, "POST /v2/spaces/{space_id}/types", entry.Endpoint)

		// when: sent to that endpoint as a dry run, with consent for the
		// select options the example declares
		_, err = fx.CreateType(context.Background(), testSpaceId, entry.Example, true, true)

		// then
		require.NoError(t, err, "the example served beside the schema is refused by the endpoint")
	})

	t.Run("the document example still reaches the same endpoint", func(t *testing.T) {
		// given: the interchange form is a fallback, not a casualty
		fx := newV2Fixture(t)
		entry, err := fx.SchemaKind("type_document")
		require.NoError(t, err)
		require.Contains(t, entry.Endpoint, "POST /v2/spaces/{space_id}/types")
		assert.Contains(t, entry.Endpoint, "kind `type`",
			"the two kinds share an endpoint, so this row has to say which one authors a type")

		// when
		_, err = fx.CreateType(context.Background(), testSpaceId, entry.Example, true, true)

		// then
		require.NoError(t, err, "the document body must keep working")
	})

	// The schema is hand-written and the flat decoder has its own key list;
	// nothing but this makes them agree. A member in one and not the other is
	// either a field the schema hides or a field the endpoint refuses.
	t.Run("the schema and the decoder declare the same members", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		entry, err := fx.SchemaKind("type")
		require.NoError(t, err)

		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(entry.Schema, &schema))

		// then
		for member := range schema.Properties {
			assert.True(t, typeShortcutKeys[member],
				"the schema advertises %q, which the flat body refuses", member)
		}
		for member := range typeShortcutKeys {
			assert.Contains(t, schema.Properties, member,
				"the flat body takes %q, and the schema does not mention it", member)
		}
	})
}

// TestV2TypeViewsAreShapedThroughTheObjectSurface pins the route the
// create-time refusal now names. POST /types rejects `blocks` outright, and
// its hint sends the caller to the object surface instead of to the app; a
// hint naming a route that does not work is worse than no hint at all.
func TestV2TypeViewsAreShapedThroughTheObjectSurface(t *testing.T) {
	const typeWithDataview = `{"formatVersion":"2.0","id":"type1","kind":"object_type",` +
		`"properties":{"name":"Plant"},"type_settings":{"api_key":"plant"},"blocks":[` +
		`{"id":"dataview","type":"dataview",` +
		`"properties":[{"property":"name","format":"text"}],` +
		`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"}]}]}]}`

	// given: a type read through the object surface, carrying its dataview
	fx := newV2Fixture(t)
	read := editRead(t, typeWithDataview)
	read.SbType = model.SmartBlockType_STType
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type1").Return(read, nil).Maybe()

	// then: the system-managed gate must not catch a type. Properties,
	// options, files and members are excluded there; types are not, which is
	// what makes this route exist at all.
	t.Run("a type is not refused as system-managed", func(t *testing.T) {
		// when
		_, err := fx.PatchObject(context.Background(), testSpaceId, "type1",
			patchBody(`{"op":"update_view","view":"viewAll1","set":{"name":"Everything"}}`), "", true, true)

		// then
		require.NoError(t, err)
	})

	t.Run("insert_view reaches the type's dataview", func(t *testing.T) {
		// when: no block ref, because a type has exactly one dataview
		_, err := fx.PatchObject(context.Background(), testSpaceId, "type1",
			patchBody(`{"op":"insert_view","name":"By status","set":{"type":"kanban"}}`), "", true, true)

		// then
		require.NoError(t, err)
	})
}

// TestV2ViewNotFoundSaysToSendAnId covers a retry loop rather than a crash.
// View references resolve by id only — full value or unique suffix — but the
// not-found list prints each view as `viewAll1 ("All")`, and a caller who
// reads the name back out of it and sends that name arrives here again. The
// message has to say which half of that pair is the address.
func TestV2ViewNotFoundSaysToSendAnId(t *testing.T) {
	// given: one view, id viewAll1, named All
	const doc = `{"formatVersion":"2.0","id":"type1","kind":"object_type",` +
		`"properties":{"name":"Plant"},"type_settings":{"api_key":"plant"},"blocks":[` +
		`{"id":"dataview","type":"dataview",` +
		`"properties":[{"property":"name","format":"text"}],` +
		`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"}]},` +
		`{"id":"viewBoard2","name":"Board","columns":[{"property":"name"}]}]}]}`

	fx := newV2Fixture(t)
	read := editRead(t, doc)
	read.SbType = model.SmartBlockType_STType
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type1").Return(read, nil).Maybe()

	// when: addressed by the display name, which is not an address
	_, err := fx.PatchObject(context.Background(), testSpaceId, "type1",
		patchBody(`{"op":"update_view","view":"All","set":{"name":"Everything"}}`), "", true, true)

	// then
	require.Error(t, err)
	apiErr := v2Err(t, err)
	assert.Contains(t, apiErr.Message, "view id",
		"the message must say an id is what resolves, or the caller retries with the name")
	assert.Contains(t, apiErr.Message, "viewAll1", "and it must list the ids to choose from")

	// the suffix rule still works, which is what makes ids usable by hand
	_, err = fx.PatchObject(context.Background(), testSpaceId, "type1",
		patchBody(`{"op":"update_view","view":"Board2","set":{"name":"Everything"}}`), "", true, true)
	require.NoError(t, err, "a unique id suffix must still resolve")
}

// TestV2TypePatchAcceptsDefaultViewAndTemplate covers the two members the flat
// body declared and PATCH silently refused: both landed in type_settings, but
// v2TypeSettingsPatch never declared them and UpdateType decodes with
// DisallowUnknownFields, so a caller following the schema got
// `json: unknown field "default_view"` naming a wrapper they never wrote.
//
// The stored shapes are the part worth pinning. default_view is an ENUM INT,
// not the name the caller sends; default_template is stored as a LIST even
// though the member is one id — the client reads entry 0, so writing a bare
// string makes the type unreadable to it.
func TestV2TypePatchAcceptsDefaultViewAndTemplate(t *testing.T) {
	setup := func(t *testing.T) (*v2Fixture, *[]*model.Detail) {
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-plant"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-plant"),
			bundle.RelationKeyApiObjectKey: domain.String("plant"),
			bundle.RelationKeyName:         domain.String("Plant"),
		})
		captured := &[]*model.Detail{}
		fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
				*captured = append(*captured, req.Details...)
				return &pb.RpcObjectSetDetailsResponse{
					Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL},
				}
			}).Maybe()
		return fx, captured
	}

	find := func(details []*model.Detail, key string) *types.Value {
		for _, d := range details {
			if d.Key == key {
				return d.Value
			}
		}
		return nil
	}

	t.Run("default_view stores the enum int for the name sent", func(t *testing.T) {
		// given
		fx, captured := setup(t)
		fx.expectEtagRead("type-plant")

		// when: the caller sends the name the schema advertises
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", []byte(`{"default_view":"kanban"}`), false, false)

		// then
		require.NoError(t, err, "the schema advertises this member on PATCH")
		v := find(*captured, "defaultViewType")
		require.NotNil(t, v, "default_view must reach the store")
		assert.Equal(t, float64(model.BlockContentDataviewView_Kanban), v.GetNumberValue())
	})

	t.Run("default_template stores a LIST, not a bare string", func(t *testing.T) {
		// given
		fx, captured := setup(t)
		fx.expectEtagRead("type-plant")

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", []byte(`{"default_template":"tpl-123"}`), false, false)

		// then
		require.NoError(t, err)
		v := find(*captured, "defaultTemplateId")
		require.NotNil(t, v)
		require.NotNil(t, v.GetListValue(), "the client reads entry 0 of a list")
		require.Len(t, v.GetListValue().Values, 1)
		assert.Equal(t, "tpl-123", v.GetListValue().Values[0].GetStringValue())
	})

	t.Run("an unknown view type is refused by name", func(t *testing.T) {
		// given
		fx, _ := setup(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", []byte(`{"default_view":"spreadsheet"}`), false, false)

		// then
		require.Error(t, err)
		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "unknown view type")
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Hint, "kanban", "the hint lists the vocabulary")
	})
}
