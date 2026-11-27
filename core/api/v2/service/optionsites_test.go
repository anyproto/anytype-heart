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

// A presence leaf carries no value the view keeps (§11): the applier strips
// it with presenceCondition, so the preflight must skip it with the same
// predicate. It used to know only notEmpty, so the wire spelling not_empty
// was refused without consent, and with consent created an option the
// applier then discarded.
func TestViewOpPresenceLeafCreatesNoOption(t *testing.T) {
	const leaf = `{"property":"severity","condition":"not_empty","value":"Ghost"}`
	for _, tc := range []struct {
		name    string
		op      string
		consent bool
	}{
		{"update_view without consent", `{"op":"update_view","set":{"filters":[` + leaf + `]}}`, false},
		{"update_view with consent", `{"op":"update_view","set":{"filters":[` + leaf + `]}}`, true},
		{"insert_view without consent", `{"op":"insert_view","name":"Open","set":{"filters":[` + leaf + `]}}`, false},
		{"insert_view with consent", `{"op":"insert_view","name":"Open","set":{"filters":[` + leaf + `]}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given: no ObjectCreateRelationOption expectation — creating
			// "Ghost" fails the test
			fx := newV2Fixture(t)
			fx.addSelectProperty(t)
			fx.expectMutate(editRead(t, editSetDoc), "headB")

			// when
			result, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", patchBody(tc.op), "", false, tc.consent)

			// then
			require.NoError(t, err)
			assert.Nil(t, result.Created, "nothing was created")
		})
	}
}

// A create body imports whole, so no single entry is being resolved when an
// option refusal fires: it is addressed at the member the caller wrote the
// name in, in the spelling written there, instead of /properties/<slug>.
func TestCreateOptionRefusalsNameTheMemberSent(t *testing.T) {
	options := v2model.RefListPropertyOptions(testSpaceId, "stage")
	resend := v2model.Resend("create_missing_options", "true")
	consentIssue := func(path, spelling string) v2model.Issue {
		return v2model.Issue{
			Path:    path,
			Message: `property "` + spelling + `" has no option named "Missing", and this request did not ask to create one`,
			Hint:    "check the spelling against " + options.String() + ", or resend with " + resend.String() + " to create it",
			SeeAlso: []v2model.Ref{options, resend},
		}
	}

	t.Run("an object property by display name under keys=name", func(t *testing.T) {
		// given
		fx, _ := spellingFixture(t)

		// when: no create consent
		_, err := fx.CreateObject(CtxWithNameKeys(context.Background()), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"Stage":["Missing"]}}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, consentIssue("/properties/Stage", "Stage"), apiErr.Issues[0])
	})

	queryFixture := func(t *testing.T) *v2Fixture {
		fx, _ := spellingFixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-errand"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-errand"),
			bundle.RelationKeyName:                 domain.String("Errand"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-stage"}),
		})
		return fx
	}

	t.Run("a query filter value by display name under keys=name", func(t *testing.T) {
		// given
		fx := queryFixture(t)

		// when — no creator expectation: reaching the create path fails the test
		_, err := fx.CreateQuery(CtxWithNameKeys(context.Background()), testSpaceId, v2model.CreateQueryRequest{
			Name: "Missing ones", Type: "errand",
			Filters: json.RawMessage(`[{"property":"Stage","condition":"in","value":["Missing"]}]`),
		}, false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, consentIssue("/filters/0/value", "Stage"), apiErr.Issues[0])
	})

	t.Run("a query compact filter value", func(t *testing.T) {
		// given
		fx := queryFixture(t)

		// when
		_, err := fx.CreateQuery(context.Background(), testSpaceId, v2model.CreateQueryRequest{
			Name: "Missing ones", Type: "errand", Filter: `stage IN ("Missing")`,
		}, false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, consentIssue("/filter", "stage"), apiErr.Issues[0])
	})
}
