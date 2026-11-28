package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// Every issue path addresses the member the caller wrote, in the request's
// own path syntax: a JSON Pointer for the type and create channels, the op
// path for an object PATCH, an index for an array entry.
func TestV2IssuePathsAddressTheMemberSent(t *testing.T) {
	t.Run("an unknown move anchor is the anchor field's fault", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(`{"op":"move_property","property":"sun_needs","after":"no_such"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops/0/after", apiErr.Issues[0].Path)
		assert.Equal(t, `no property in this space answers to "no_such"`, apiErr.Issues[0].Message)
	})

	t.Run("unset entries are addressed by index, spelled in the message", func(t *testing.T) {
		// given
		fx, read := spellingFixture(t)
		fx.expectMutate(read)
		want := []v2model.Issue{
			{Path: "ops[0].unset[0]", Message: `"created_date" is output-only and cannot be unset`},
			{Path: "ops[0].unset[1]", Message: `"Creation date" is output-only and cannot be unset`},
		}

		// when
		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","unset":["created_date","Creation date"]}`), "", false, true)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, want, apiErr.Issues)
	})

	t.Run("a format conflict in a flat type body is rebased with the rest", func(t *testing.T) {
		// given: severity exists as a select
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)

		// when: the flat body, whose definitions sit at the root
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"name":"Gadget","property_definitions":[{"property":"severity","format":"number"}]}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/property_definitions/0/format", apiErr.Issues[0].Path)
	})

	queryFixture := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t) // type "chore" recommending "severity"
		return fx
	}

	t.Run("a compact type filter on query create points at the filter string", func(t *testing.T) {
		// given
		fx := queryFixture(t)

		// when — no creator expectation: reaching the create path fails the test
		_, err := fx.CreateQuery(context.Background(), testSpaceId, v2model.CreateQueryRequest{
			Name: "Chores", Type: "chore", Filter: `type = "chore"`,
		}, false, true)

		// then: the caller wrote a string; /filters/0/property is the server's
		// own parse of it
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/filter", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "drop the type filter")
	})

	t.Run("an unknown group_by on query create names the wire member", func(t *testing.T) {
		// given
		fx := queryFixture(t)

		// when
		_, err := fx.CreateQuery(context.Background(), testSpaceId, v2model.CreateQueryRequest{
			Name: "Chores", Type: "chore",
			Views: json.RawMessage(`[{"name":"Board","type":"kanban","group_by":"no_such"}]`),
		}, false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/views/0/group_by", apiErr.Issues[0].Path)
	})
}

// A member name the caller chose is one JSON Pointer token wherever an
// unknown-member refusal names it.
func TestV2UnknownMemberPathsAreEscaped(t *testing.T) {
	const want = "/a~1b~0c"

	t.Run("the PATCH body", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil).Maybe()

		// when
		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", []byte(`{"ops":[],"a/b~c":1}`), "", false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0].Path)
	})

	t.Run("the create shortcut", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId, []byte(`{"type":"page","name":"x","a/b~c":1}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0].Path)
	})

	t.Run("the flat type body", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId, []byte(`{"name":"Gadget","a/b~c":1}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0].Path)
	})
}

// A read's format warning about a property is addressed at the member the
// read serves, escaped: the format names it by its stored key in some
// warnings and by its served spelling, raw, in others.
func TestV2ReadWarningPathsNameTheServedMember(t *testing.T) {
	t.Run("a display name holding a slash under keys=name", func(t *testing.T) {
		// given: a date stored in milliseconds, which the format warns about
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-span"),
			bundle.RelationKeyRelationKey:    domain.String("6a7663db61fab21cd4b9e4a1"),
			bundle.RelationKeyApiObjectKey:   domain.String("start_end"),
			bundle.RelationKeyName:           domain.String("Start/End"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
		})
		read := editRead(t, editBaseDoc)
		read.Snapshot.Details.Fields["6a7663db61fab21cd4b9e4a1"] = pbtypes.Float64(1e13)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)

		// when
		body, _, err := fx.GetObject(CtxWithNameKeys(context.Background()), testSpaceId, "obj1", ObjectQuery{})

		// then
		require.NoError(t, err)
		var doc struct {
			Warnings []v2model.Issue `json:"warnings"`
		}
		require.NoError(t, json.Unmarshal(body, &doc))
		require.Len(t, doc.Warnings, 1)
		assert.Equal(t, "/properties/Start~1End", doc.Warnings[0].Path)
	})

	t.Run("a stored key is re-spelled as the served one", func(t *testing.T) {
		spell := func(stored string) string {
			return map[string]string{"6a7663db61fab21cd4b9e4b1": "linked"}[stored]
		}
		assert.Equal(t, "/properties/linked/0",
			readWarningPath("/properties/6a7663db61fab21cd4b9e4b1/0", []string{"6a7663db61fab21cd4b9e4b1"}, spell))
		assert.Equal(t, "/blocks/3", readWarningPath("/blocks/3", []string{"6a7663db61fab21cd4b9e4b1"}, spell),
			"only property paths are re-addressed")
	})
}
