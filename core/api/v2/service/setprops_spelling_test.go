package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// A set_properties refusal names the property the way the caller addressed
// it — the slug by default, the display name under ?keys=name — exactly as
// its issue path does. A space-minted property's stored key is a 24-hex id
// the caller was never served and cannot act on; a built-in's stored key is
// the camelCase spelling the slug vocabulary hides. Neither belongs in the
// message (GO-7615, the class of suite APIV2-007).
func TestSetPropertiesRefusalsSpellTheServedKey(t *testing.T) {
	const (
		doneKey  = "6a1f0c2e9b3d4e5f6a7b8c9d" // checkbox, slug done_flag
		stageKey = "6a1f0c2e9b3d4e5f6a7b8c9e" // select, slug stage, holds a value
	)
	newFixture := func(t *testing.T) *v2Fixture {
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
			want: v2model.Issue{Path: "ops[0].unset.created_date",
				Message: `"created_date" is output-only and cannot be unset`},
		},
		{
			name:   "unset of an output-only built-in, by name under keys=name",
			names:  true,
			op:     `{"op":"set_properties","unset":["Creation date"]}`,
			stored: "createdDate",
			want: v2model.Issue{Path: "ops[0].unset.Creation date",
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
