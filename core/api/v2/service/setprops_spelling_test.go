package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	spellingDoneKey  = "6a1f0c2e9b3d4e5f6a7b8c9d" // checkbox, slug done_flag, name "Done flag"
	spellingStageKey = "6a1f0c2e9b3d4e5f6a7b8c9e" // select, slug stage, name "Stage"; obj1 holds "Draft"
)

// spellingFixture is a space with two space-minted (24-hex keyed) properties
// and obj1, which holds a value of the select one. The mutator is not wired:
// a test whose PATCH reaches the lock adds expectMutate itself.
func spellingFixture(t *testing.T) (*v2Fixture, apicore.ObjectRead) {
	const (
		doneKey  = spellingDoneKey
		stageKey = spellingStageKey
	)
	fx := newV2Fixture(t)
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("rel-done-flag"),
			bundle.RelationKeyRelationKey:    domain.String(doneKey),
			bundle.RelationKeyApiObjectKey:   domain.String("done_flag"),
			bundle.RelationKeyName:           domain.String("Done flag"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_checkbox)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		},
		{
			bundle.RelationKeyId:             domain.String("rel-stage"),
			bundle.RelationKeyRelationKey:    domain.String(stageKey),
			bundle.RelationKeyApiObjectKey:   domain.String("stage"),
			bundle.RelationKeyName:           domain.String("Stage"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		},
		{
			bundle.RelationKeyId:             domain.String("opt-draft"),
			bundle.RelationKeyRelationKey:    domain.String(stageKey),
			bundle.RelationKeyName:           domain.String("Draft"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		},
	})
	read := editRead(t, editBaseDoc)
	read.Snapshot.Details.Fields[stageKey] = pbtypes.StringList([]string{"opt-draft"})
	return fx, read
}

// A set_properties refusal names the property the way the caller addressed
// it — the slug by default, the display name under ?keys=name — exactly as
// its issue path does. A space-minted property's stored key is a 24-hex id
// the caller was never served and cannot act on; a built-in's stored key is
// the camelCase spelling the slug vocabulary hides. Neither belongs in the
// message (GO-7615, the class of suite APIV2-007).
func TestSetPropertiesRefusalsSpellTheServedKey(t *testing.T) {
	const (
		doneKey  = spellingDoneKey
		stageKey = spellingStageKey
	)
	newFixture := func(t *testing.T) *v2Fixture {
		fx, read := spellingFixture(t)
		fx.expectMutate(read)
		return fx
	}
	const listShapeTail = ` only applies to list-shaped formats (select, multi_select, objects, files); use set`

	for _, tc := range []struct {
		name   string
		names  bool // the request asked for ?keys=name
		op     string
		stored string // the stored key the message must not name
		want   v2model.Issue
	}{
		{
			name:   "add on a scalar custom property, by slug",
			op:     `{"op":"set_properties","add":{"done_flag":[true]}}`,
			stored: doneKey,
			want: v2model.Issue{Path: "ops[0].add.done_flag",
				Message: `"done_flag" has format "checkbox" — add` + listShapeTail},
		},
		{
			name:   "remove on a scalar custom property, by name under keys=name",
			names:  true,
			op:     `{"op":"set_properties","remove":{"Done flag":[true]}}`,
			stored: doneKey,
			want: v2model.Issue{Path: "ops[0].remove.Done flag",
				Message: `"Done flag" has format "checkbox" — remove` + listShapeTail},
		},
		{
			name:   "add on a custom select that holds a value, by slug",
			op:     `{"op":"set_properties","add":{"stage":["Draft"]}}`,
			stored: stageKey,
			want: v2model.Issue{Path: "ops[0].add.stage",
				Message: `"stage" has format "select" and holds a single value`,
				Hint:    "use set to replace it, or unset to clear it first"},
		},
		{
			name:   "add on a custom select that holds a value, by name under keys=name",
			names:  true,
			op:     `{"op":"set_properties","add":{"Stage":["Draft"]}}`,
			stored: stageKey,
			want: v2model.Issue{Path: "ops[0].add.Stage",
				Message: `"Stage" has format "select" and holds a single value`,
				Hint:    "use set to replace it, or unset to clear it first"},
		},
		{
			name:   "two spellings of one custom property",
			op:     `{"op":"set_properties","set":{"done_flag":true,"Done flag":false}}`,
			stored: doneKey,
			want: v2model.Issue{Path: "ops[0].set.done_flag",
				Message: `"Done flag" and "done_flag" both address one property — keep one`},
		},
		{
			name:   "set on an output-only built-in, by slug",
			op:     `{"op":"set_properties","set":{"created_date":1}}`,
			stored: "createdDate",
			want: v2model.Issue{Path: "ops[0].set.created_date",
				Message: `"created_date" is output-only — export writes it, writes must not`},
		},
		{
			name:   "set on an output-only built-in, by name under keys=name",
			names:  true,
			op:     `{"op":"set_properties","set":{"Creation date":1}}`,
			stored: "createdDate",
			want: v2model.Issue{Path: "ops[0].set.Creation date",
				Message: `"Creation date" is output-only — export writes it, writes must not`},
		},
		{
			name:   "unset of an output-only built-in, by slug",
			op:     `{"op":"set_properties","unset":["created_date"]}`,
			stored: "createdDate",
			want: v2model.Issue{Path: "ops[0].unset[0]",
				Message: `"created_date" is output-only and cannot be unset`},
		},
		{
			name:   "unset of an output-only built-in, by name under keys=name",
			names:  true,
			op:     `{"op":"set_properties","unset":["Creation date"]}`,
			stored: "createdDate",
			want: v2model.Issue{Path: "ops[0].unset[0]",
				Message: `"Creation date" is output-only and cannot be unset`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			fx := newFixture(t)
			ctx := context.Background()
			if tc.names {
				ctx = CtxWithNameKeys(ctx)
			}

			// when
			_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(tc.op), "", false, true)

			// then
			apiErr := v2Err(t, err)
			require.Len(t, apiErr.Issues, 1)
			got := apiErr.Issues[0]
			assert.Equal(t, tc.want, got)
			for _, text := range []string{apiErr.Message, got.Message, got.Hint} {
				assert.NotRegexp(t, `[0-9a-f]{24}`, text, "a stored bson key reached the caller")
				assert.NotContains(t, text, tc.stored)
			}
		})
	}
}

// One op may spell one property two ways in two fields. Each refusal must
// address the member it is about, in the spelling written THERE: a single
// canonical → spelling map let the field canonicalized last overwrite the
// others, so a refusal about `add` named the spelling `remove` used and
// pointed at a member the request does not contain.
func TestSetPropertiesRefusalsKeepEachFieldsSpelling(t *testing.T) {
	const listShapeTail = ` only applies to list-shaped formats (select, multi_select, objects, files); use set`

	for _, tc := range []struct {
		name string
		op   string
		want []v2model.Issue
	}{
		{
			name: "scalar add by slug beside a remove by name",
			op:   `{"op":"set_properties","add":{"done_flag":[true]},"remove":{"Done flag":[true]}}`,
			want: []v2model.Issue{
				{Path: "ops[0].add.done_flag", Message: `"done_flag" has format "checkbox" — add` + listShapeTail},
				{Path: "ops[0].remove.Done flag", Message: `"done_flag" in add and "Done flag" in remove both address one property — pick one`},
			},
		},
		{
			name: "output-only set by slug beside an unset by name",
			op:   `{"op":"set_properties","set":{"created_date":1},"unset":["Creation date"]}`,
			want: []v2model.Issue{
				{Path: "ops[0].set.created_date", Message: `"created_date" is output-only — export writes it, writes must not`},
				{Path: "ops[0].unset[0]", Message: `"Creation date" is output-only and cannot be unset`},
			},
		},
		{
			name: "unset by name beside an add by slug",
			op:   `{"op":"set_properties","unset":["Stage"],"add":{"stage":["Draft"]}}`,
			want: []v2model.Issue{
				{Path: "ops[0].add.stage", Message: `"Stage" in unset and "stage" in add both address one property — pick one`},
			},
		},
		{
			name: "an empty add by slug beside an unset by name",
			op:   `{"op":"set_properties","add":{"stage":[]},"unset":["Stage"]}`,
			want: []v2model.Issue{
				{Path: "ops[0].add.stage", Message: `"Stage" in unset and "stage" in add both address one property — pick one`},
			},
		},
		{
			name: "the envelope type by its display name, set",
			op:   `{"op":"set_properties","set":{"Object type":"task"}}`,
			want: []v2model.Issue{
				{Path: "ops[0].set.Object type",
					Message: `"Object type" is not a property — it is the document envelope's type, and changes through the set_type op`,
					Hint:    `send {"op":"set_type","type":…} instead; ` + v2model.RefGetOpSchema("set_type").String() + ` describes it`,
					SeeAlso: []v2model.Ref{v2model.RefGetOpSchema("set_type")}},
			},
		},
		{
			name: "the envelope type by its display name, unset",
			op:   `{"op":"set_properties","unset":["Object type"]}`,
			want: []v2model.Issue{
				{Path: "ops[0].unset[0]", Message: `"Object type" is not a property and cannot be unset — an object always has one`},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			fx, read := spellingFixture(t)
			fx.expectMutate(read)

			// when
			_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", patchBody(tc.op), "", false, true)

			// then
			apiErr := v2Err(t, err)
			assert.Equal(t, tc.want, apiErr.Issues)
			for _, iss := range apiErr.Issues {
				assert.NotRegexp(t, `[0-9a-f]{24}`, iss.Message, "a stored bson key reached the caller")
				assert.NotContains(t, iss.Message, "createdDate")
			}
		})
	}

	t.Run("the favorite flag by its display name", func(t *testing.T) {
		// given
		fx, read := spellingFixture(t)
		fx.expectMutate(read)
		want := v2model.Issue{Path: "ops[0].set.Favorited", Message: "Favorited takes true or false"}

		// when
		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"Favorited":"yes"}}`), "", false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
	})
}

// The option-consent preflight runs before the lock, over the raw op
// payloads. Its refusal used to be the create channel's, addressed at
// /properties/<served key> — a member a PATCH does not have, and under
// ?keys=name a spelling the caller did not use.
func TestSetPropertiesOptionConsentNamesTheEntry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		names bool
		op    string
		want  v2model.Issue
	}{
		{
			name: "by slug",
			op:   `{"op":"set_properties","set":{"stage":["Typo"]}}`,
			want: v2model.Issue{Path: "ops[0].set.stage",
				Message: `property "stage" has no option named "Typo", and this request did not ask to create one`},
		},
		{
			name:  "by display name under keys=name",
			names: true,
			op:    `{"op":"set_properties","set":{"Stage":["Typo"]}}`,
			want: v2model.Issue{Path: "ops[0].set.Stage",
				Message: `property "Stage" has no option named "Typo", and this request did not ask to create one`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given: the read alone — a pre-lock refusal never reaches the mutator
			fx, read := spellingFixture(t)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
			ctx := context.Background()
			if tc.names {
				ctx = CtxWithNameKeys(ctx)
			}
			options := v2model.RefListPropertyOptions(testSpaceId, "stage")
			resend := v2model.Resend("create_missing_options", "true")
			tc.want.Hint = "check the spelling against " + options.String() + ", or resend with " + resend.String() + " to create it"
			tc.want.SeeAlso = []v2model.Ref{options, resend}

			// when: no create consent
			_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(tc.op), "", false, false)

			// then
			apiErr := v2Err(t, err)
			assert.Equal(t, "option does not exist", apiErr.Message)
			require.Len(t, apiErr.Issues, 1)
			assert.Equal(t, tc.want, apiErr.Issues[0])
			assert.NotRegexp(t, `[0-9a-f]{24}`, apiErr.Issues[0].Message)
		})
	}
}

// The update_view prewarm walks the set channel's filter leaves and custom
// sort orders before the lock, through the same consent gate. Its refusal
// used the create channel's /properties/<served key>, a member a view op does
// not have, in a spelling a ?keys=name caller did not use.
func TestViewOpOptionConsentNamesTheEntry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		names    bool
		op       string
		path     string
		spelling string
	}{
		{
			name:     "a structured filter leaf, by slug",
			op:       `{"op":"update_view","set":{"filters":[{"property":"stage","condition":"equal","value":"Typo"}]}}`,
			path:     "ops[0].set.filters[0].value",
			spelling: "stage",
		},
		{
			name:     "a structured filter leaf, by display name under keys=name",
			names:    true,
			op:       `{"op":"update_view","set":{"filters":[{"property":"Stage","condition":"equal","value":"Typo"}]}}`,
			path:     "ops[0].set.filters[0].value",
			spelling: "Stage",
		},
		{
			name: "a leaf inside a filter group",
			op: `{"op":"update_view","set":{"filters":[{"property":"name","condition":"not_empty"},` +
				`{"operator":"or","filters":[{"property":"stage","condition":"equal","value":"Typo"}]}]}}`,
			path:     "ops[0].set.filters[1].filters[0].value",
			spelling: "stage",
		},
		{
			name:     "a compact filter string",
			op:       `{"op":"update_view","set":{"filter":"stage = \"Typo\""}}`,
			path:     "ops[0].set.filter",
			spelling: "stage",
		},
		{
			name:     "a custom sort order",
			op:       `{"op":"update_view","set":{"sorts":[{"property":"stage","direction":"custom","custom_order":["Typo"]}]}}`,
			path:     "ops[0].set.sorts[0].custom_order",
			spelling: "stage",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given: the read alone — a pre-lock refusal never reaches the mutator
			fx, read := spellingFixture(t)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
			ctx := context.Background()
			if tc.names {
				ctx = CtxWithNameKeys(ctx)
			}
			options := v2model.RefListPropertyOptions(testSpaceId, "stage")
			resend := v2model.Resend("create_missing_options", "true")
			want := v2model.Issue{
				Path:    tc.path,
				Message: `property "` + tc.spelling + `" has no option named "Typo", and this request did not ask to create one`,
				Hint:    "check the spelling against " + options.String() + ", or resend with " + resend.String() + " to create it",
				SeeAlso: []v2model.Ref{options, resend},
			}

			// when: no create consent
			_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(tc.op), "", false, false)

			// then
			apiErr := v2Err(t, err)
			assert.Equal(t, "option does not exist", apiErr.Message)
			require.Len(t, apiErr.Issues, 1)
			assert.Equal(t, want, apiErr.Issues[0])
			assert.NotRegexp(t, `[0-9a-f]{24}`, apiErr.Issues[0].Message)
		})
	}
}
