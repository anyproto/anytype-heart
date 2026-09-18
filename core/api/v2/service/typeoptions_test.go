package v2service

import (
	"context"
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

// The bug: a type document declaring a select property WITH its vocabulary
// returned 200, created the property, and silently created no options.
func TestV2TypeDeclaredOptionsAreNotSilentlyDropped(t *testing.T) {
	const body = `{"formatVersion":"2.0","kind":"object_type",` +
		`"properties":{"name":"Plant"},` +
		`"type_settings":{"api_key":"plant","property_definitions":[` +
		`{"property":"Harvest Season","name":"Harvest Season","format":"select",` +
		`"options":[{"name":"Summer"},{"name":"Autumn"}]}]}}`

	t.Run("a dry run reports the options a real run would create", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		result, err := fx.CreateType(context.Background(), testSpaceId, []byte(body), true, true)

		// then: whatever the verdict, it is never a silent success
		if err != nil {
			apiErr := v2Err(t, err)
			require.NotEmpty(t, apiErr.Issues)
			assert.Contains(t, apiErr.Issues[0].Path, "options",
				"a refusal must name the options it could not create, not stay quiet")
			return
		}
		require.NotNil(t, result.Created, "the options were accepted and nothing was reported")
		names := make([]string, 0, len(result.Created.Options))
		for _, o := range result.Created.Options {
			names = append(names, o.Name)
		}
		assert.ElementsMatch(t, []string{"Summer", "Autumn"}, names,
			"the declared vocabulary must be reported as created, not dropped")
	})

	t.Run("without consent the request is refused, never silently accepted", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId, []byte(body), true, false)

		// then
		require.Error(t, err, "declared options with no create consent must refuse")
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
	})
}

// The shape every agent reaches for on POST /types: the body a REST API would
// have. Reproduced from a real MCP session — it failed with the format layer's
// verdict ("this is a property dictionary ... read it with
// UnmarshalPropertyDictionary"), which named no field to move and a Go
// function the caller does not have.
//
// The flat body now takes name, plural_name, icon and layout at its root, so
// there only the field list is still misplaced. A document body takes none of
// them, so it keeps the full naming: one response, every field.
func TestV2CreateTypeNamesEveryMisplacedField(t *testing.T) {
	issuesByPath := func(t *testing.T, err error) map[string]v2model.Issue {
		t.Helper()
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		paths := make(map[string]v2model.Issue, len(apiErr.Issues))
		for _, iss := range apiErr.Issues {
			paths[iss.Path] = iss
			assert.NotContains(t, iss.Message, "Unmarshal", "a Go symbol reached the caller")
			assert.NotContains(t, iss.Hint, "Unmarshal", "a Go symbol reached the caller")
		}
		return paths
	}

	t.Run("the flat body names the one member that moved", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		body := `{"name":"Plant","plural_name":"Plants","icon":{"emoji":"x"},` +
			`"layout":"basic","properties":[{"name":"Location","format":"select"}]}`

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId, []byte(body), true, true)

		// then
		paths := issuesByPath(t, err)
		require.Contains(t, paths, "/properties", "the field list itself is unnamed")
		assert.Contains(t, paths["/properties"].Hint, "property_definitions")

		// the four the flat body accepts must NOT be reported: a caller told to
		// move a field that is already in the right place moves it out of it
		for _, accepted := range []string{"/name", "/plural_name", "/icon", "/layout"} {
			assert.NotContains(t, paths, accepted, "the flat body takes this member at its root")
		}
	})

	t.Run("a document body names every misplaced field in one response", func(t *testing.T) {
		// given: type_settings makes this the interchange document, whose root
		// takes none of the four
		fx := newV2Fixture(t)
		body := `{"name":"Plant","plural_name":"Plants","layout":"basic",` +
			`"type_settings":{"api_key":"plant"},` +
			`"properties":[{"name":"Location","format":"select"}]}`

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId, []byte(body), true, true)

		// then: one round trip per misplaced field is the failure this replaces
		paths := issuesByPath(t, err)
		require.Contains(t, paths, "/properties")
		assert.Contains(t, paths["/properties"].Hint, "type_settings.property_definitions")
		assert.Contains(t, paths, "/layout")
		assert.Contains(t, paths, "/plural_name")
		assert.Contains(t, paths["/layout"].Hint, "type_settings.layout")
	})
}

// TestV2CreateTypeRefusesBeforeMintingAnything is the orphan-property guard.
//
// The consent gate used to fire AFTER anyblockjson.Unmarshal had already
// minted the type's missing properties, so a refused create left them behind
// as relations nothing points at — the caller sees a 400 and the space keeps
// the debris. The assertion is on the absence of the mint, not on the refusal:
// a refusal that still writes is the bug.
func TestV2CreateTypeRefusesBeforeMintingAnything(t *testing.T) {
	// given: a type declaring a select vocabulary, with no consent to create it
	fx := newV2Fixture(t)
	minted := 0
	fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
			minted++
			return &pb.RpcObjectCreateRelationResponse{
				Error:    &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
				ObjectId: "rel-minted",
			}
		}).Maybe()

	body := []byte(`{"name":"Plant","property_definitions":[` +
		`{"name":"Location","format":"select","options":[{"name":"Balcony"}]}]}`)

	// when: a real run, consent withheld (the documented default)
	_, err := fx.CreateType(context.Background(), testSpaceId, body, false, false)

	// then
	require.Error(t, err)
	apiErr := v2Err(t, err)
	assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
	assert.Zero(t, minted, "the request was refused, so it must not have created a property")

	// and the refusal points at what the caller wrote, not at a `properties`
	// member this body does not have
	require.NotEmpty(t, apiErr.Issues)
	assert.Contains(t, apiErr.Issues[0].Path, "property_definitions")
	assert.NotContains(t, apiErr.Issues[0].Path, "/properties/")
}

// TestV2TypeDeclaredOptionColorIsApplied closes the last silent drop on this
// channel. The schema publishes `options[].color`, the format decodes it, and
// applyDeclaredOptions used to read only opt.Name — so a declared colour was
// accepted and thrown away. POST /v2/spaces/{id}/properties has honoured it
// all along, which is what made the two endpoints disagree about the same
// member.
func TestV2TypeDeclaredOptionColorIsApplied(t *testing.T) {
	// given: a type declaring one coloured option on an existing property
	fx := newV2Fixture(t)
	fx.addSelectProperty(t)

	var optionDetails []*types.Struct
	fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationOptionRequest) *pb.RpcObjectCreateRelationOptionResponse {
			optionDetails = append(optionDetails, req.Details)
			return &pb.RpcObjectCreateRelationOptionResponse{
				Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
				ObjectId: "opt-new",
			}
		}).Maybe()

	fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
			return &pb.RpcObjectCreateObjectTypeResponse{
				ObjectId: "type-plant",
				Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
			}
		}).Maybe()
	fx.expectEtagRead("type-plant")

	body := []byte(`{"name":"Plant","property_definitions":[` +
		`{"name":"Severity","format":"select","options":[{"name":"Critical","color":"red"}]}]}`)

	// when: consent given, real run
	_, err := fx.CreateType(context.Background(), testSpaceId, body, false, true)

	// then
	require.NoError(t, err)
	require.Len(t, optionDetails, 1, "the declared option must be created")
	fields := optionDetails[0].Fields
	assert.Equal(t, "Critical", fields[bundle.RelationKeyName.String()].GetStringValue())
	assert.Equal(t, "red", fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue(),
		"the declared colour must reach the store, as it does on POST /properties")
}

// TestV2TypeOptionsNeedASelectFormatOnBothVerbs closes the asymmetry between
// the two type verbs. CreateType gets the rule from the format layer, which
// refuses options on a non-select definition; UpdateType validates no document
// at all, so the same body went through and hung a select vocabulary on a text
// property. One schema describes both verbs, so one rule has to hold for both.
func TestV2TypeOptionsNeedASelectFormatOnBothVerbs(t *testing.T) {
	const defs = `{"property_definitions":[{"name":"Notes","format":"text","options":[{"name":"x"}]}]}`

	t.Run("create refuses", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when: consent granted, so only the format rule can refuse this
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"name":"Plant",`+defs[1:]), true, true)

		// then
		require.Error(t, err)
	})

	t.Run("update refuses too", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-plant"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-plant"),
			bundle.RelationKeyApiObjectKey: domain.String("plant"),
			bundle.RelationKeyName:         domain.String("Plant"),
		})

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", []byte(`{"type_settings":`+defs+`}`), true, true)

		// then
		require.Error(t, err, "a text property must not take a select vocabulary")
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		assert.Contains(t, apiErr.Message, "select format")
	})
}

// TestV2CreateObjectConsentReachesBothBodyShapes pins the bug an MCP agent hit
// on a real run: POST /objects honoured ?create_missing_options=true for the
// DOCUMENT body and dropped it for the SHORTCUT body, so the same flag worked
// or not depending on which form the caller picked — and the shortcut is the
// form an agent sends. createFromShortcut took the parameter and passed only
// dryRun to createFromDocument.
//
// The refusal was doubly misleading: its hint said "resend with
// ?create_missing_options=true", which the caller had already done.
func TestV2CreateObjectConsentReachesBothBodyShapes(t *testing.T) {
	bodies := map[string]string{
		"shortcut": `{"type":"task","name":"Probe","properties":{"severity":"Brand New Option"}}`,
		"document": `{"formatVersion":"2.0","type":"task","properties":{"name":"Probe","severity":"Brand New Option"}}`,
	}

	for shape, body := range bodies {
		t.Run(shape+" honours consent", func(t *testing.T) {
			// given
			fx := newV2Fixture(t)
			fx.addSelectProperty(t)

			// when: consent granted, dry run
			result, err := fx.CreateObject(context.Background(), testSpaceId, []byte(body), true, true)

			// then
			require.NoError(t, err, "consent was given, so the unmatched option must be allowed")
			require.NotNil(t, result.Created, "the option it would create must be reported")
			require.NotEmpty(t, result.Created.Options)
			assert.Equal(t, "Brand New Option", result.Created.Options[0].Name)
		})

		t.Run(shape+" refuses without consent", func(t *testing.T) {
			// given
			fx := newV2Fixture(t)
			fx.addSelectProperty(t)

			// when: no consent — and a DRY RUN, which must agree with the real
			// run rather than green-light a body the real run would refuse
			_, err := fx.CreateObject(context.Background(), testSpaceId, []byte(body), true, false)

			// then
			require.Error(t, err, "a dry run that passes here promises a create that fails")
			apiErr := v2Err(t, err)
			assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		})
	}
}

// TestV2UpdateTypeReportsDetachedProperties reproduces the exact call all three
// benchmark agents made: to ADD one field they sent property_definitions
// holding only that field. The list REPLACES, so the type's other four fields
// were detached — and the 200 response mentioned only what it created, so two
// of the three ran on a gutted type for a dozen more calls. One of them called
// get-type specifically to check and was reassured, because the type's dataview
// block still listed the old fields.
//
// The write still replaces; that is the documented contract. What changes is
// that it now says what it took away.
func TestV2UpdateTypeReportsDetachedProperties(t *testing.T) {
	newTypeFixture := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		for _, p := range []struct{ id, key, name string }{
			{"rel-location", "location", "Location"},
			{"rel-sun", "sun_needs", "Sun Needs"},
			{"rel-harvest", "harvest_season", "Harvest Season"},
		} {
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:             domain.String(p.id),
				bundle.RelationKeyRelationKey:    domain.String(p.key),
				bundle.RelationKeyApiObjectKey:   domain.String(p.key),
				bundle.RelationKeyName:           domain.String(p.name),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			})
		}
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-plant"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-plant"),
			bundle.RelationKeyApiObjectKey: domain.String("plant"),
			bundle.RelationKeyName:         domain.String("Plant"),
			bundle.RelationKeyRecommendedRelations: domain.StringList(
				[]string{"rel-location", "rel-sun"}),
		})
		return fx
	}

	t.Run("a one-entry list names what it detached", func(t *testing.T) {
		// given: a type that already recommends location and sun_needs
		fx := newTypeFixture(t)

		// when: the benchmark's call — add one field by sending only that field
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", []byte(`{"type_settings":{"property_definitions":[{"property":"harvest_season","format":"select"}]}}`),
			true, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Removed, "the response must state what the replace took away")
		keys := make([]string, 0, len(result.Removed.Properties))
		for _, row := range result.Removed.Properties {
			keys = append(keys, row.Key)
		}
		assert.ElementsMatch(t, []string{"location", "sun_needs"}, keys)

		// and loudly, for a reader who does not know to look for a new field
		require.NotEmpty(t, result.Warnings, "a removal must also warn")
		assert.Contains(t, result.Warnings[0].Message, "replaces the type's whole field list")
		assert.Contains(t, result.Warnings[0].Message, "location")
		assert.Equal(t, "/type_settings/property_definitions", result.Warnings[0].Path)
	})

	t.Run("re-sending the complete list detaches nothing", func(t *testing.T) {
		// given
		fx := newTypeFixture(t)

		// when: the repair call every agent had to make
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", []byte(`{"type_settings":{"property_definitions":[`+
				`{"property":"location","format":"select"},`+
				`{"property":"sun_needs","format":"select"},`+
				`{"property":"harvest_season","format":"select"}]}}`),
			true, true)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Removed, "nothing was dropped, so nothing is reported")
		assert.Empty(t, result.Warnings)
	})

	t.Run("omitting property_definitions leaves the list alone", func(t *testing.T) {
		// given: the other way to avoid the trap
		fx := newTypeFixture(t)

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", []byte(`{"plural_name":"Plants"}`), true, true)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Removed, "an untouched list detaches nothing")
	})
}
