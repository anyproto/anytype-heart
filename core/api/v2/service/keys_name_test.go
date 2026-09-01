package v2service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Chain step 5 (VOCABULARY §4.3): display names resolve on every
// key-accepting channel — an agent reading "Due date" from a ?keys=name body
// must be able to use it in a PATCH op, fields=, a route param or a view-op
// column. Names join the chain LAST (a live stored key or slug always
// outranks a name — the verbatim-first obligation), exact NFC name beats the
// FoldKeyTerm fold, hidden entries never answer by name, and a bundled NAME
// gets the same no-shadowing discipline the bundled SLUG has at step 2: a
// live visible twin refuses loudly, never silently wins or loses.

// joinedCandidates flattens an ambiguity candidate list for Contains asserts.
func joinedCandidates(candidates []string) string {
	return strings.Join(candidates, " and ")
}

func TestResolvePropertyInputDisplayNames(t *testing.T) {
	fx := newV2Fixture(t)
	// the baseline namespace: a BSON-keyed property whose NAME is the only
	// readable multi-word handle, a hidden entry, and a legacy stored key
	// that a twin's NAME tries (and must fail) to claim
	entries := []propertyEntry{
		{Id: "rel-goal", Key: "6a7663db61fab21cd4b9e201", Slug: "goal_field", Name: "Sprint goal", Format: model.RelationFormat_longtext},
		{Id: "rel-hidden", Key: "6a7663db61fab21cd4b9e202", Name: "Ghost name", Hidden: true},
		{Id: "rel-legacy", Key: "deadline", Name: "Legacy deadline"},
		{Id: "rel-named-key", Key: "6a7663db61fab21cd4b9e203", Name: "deadline"},
	}

	t.Run("resolution table", func(t *testing.T) {
		cases := []struct {
			name    string
			input   string
			wantKey string
			wantOk  bool
		}{
			{"exact display name resolves the visible holder", "Sprint goal", "6a7663db61fab21cd4b9e201", true},
			{"FoldKeyTerm variant with a dash resolves the name", "sprint-goal", "6a7663db61fab21cd4b9e201", true},
			{"FoldKeyTerm variant with underscores and case resolves the name", "SPRINT_GOAL", "6a7663db61fab21cd4b9e201", true},
			{"FoldKeyTerm variant with a lowercased space resolves the name", "sprint goal", "6a7663db61fab21cd4b9e201", true},
			{"a hidden entry's name does not resolve", "Ghost name", "", false},
			{"a hidden entry's name does not resolve through the fold either", "ghost-name", "", false},
			{"a name equal to another live entry's stored key loses to that key (step 1)", "deadline", "deadline", true},
			{"a full miss stays a miss", "no such property", "", false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				// when
				entry, ok, ambiguous := fx.resolvePropertyInput(tc.input, entries)

				// then
				assert.Empty(t, ambiguous)
				assert.Equal(t, tc.wantOk, ok)
				assert.Equal(t, tc.wantKey, entry.Key)
			})
		}
	})

	t.Run("NFC-decomposed spelling of a name resolves the precomposed holder", func(t *testing.T) {
		// given: the store holds the precomposed é; the caller pastes the
		// decomposed form (e + combining acute) — same name to any reader
		withCafe := append([]propertyEntry{}, entries...)
		withCafe = append(withCafe, propertyEntry{Id: "rel-cafe", Key: "6a7663db61fab21cd4b9e204", Name: "Café notes"})

		// when
		entry, ok, ambiguous := fx.resolvePropertyInput("Cafe\u0301 notes" /* decomposed: e + combining acute */, withCafe)

		// then
		assert.Empty(t, ambiguous)
		require.True(t, ok)
		assert.Equal(t, "rel-cafe", entry.Id)
	})

	t.Run("two visible entries sharing a name are a loud ambiguity", func(t *testing.T) {
		// given
		twins := append([]propertyEntry{}, entries...)
		twins = append(twins, propertyEntry{Id: "rel-goal-twin", Key: "6a7663db61fab21cd4b9e205", Name: "Sprint goal"})

		// when
		_, ok, ambiguous := fx.resolvePropertyInput("Sprint goal", twins)

		// then: both holders named, by the one address that always resolves
		assert.False(t, ok)
		require.Len(t, ambiguous, 2)
		assert.Contains(t, joinedCandidates(ambiguous), "6a7663db61fab21cd4b9e201")
		assert.Contains(t, joinedCandidates(ambiguous), "6a7663db61fab21cd4b9e205")
	})

	t.Run("two visible names in one fold class refuse with both candidates", func(t *testing.T) {
		// given: exact spellings differ, fold classes collide
		folded := append([]propertyEntry{}, entries...)
		folded = append(folded, propertyEntry{Id: "rel-goal-dashed", Key: "6a7663db61fab21cd4b9e206", Name: "Sprint-Goal"})

		// when
		_, ok, ambiguous := fx.resolvePropertyInput("sprintgoal", folded)

		// then
		assert.False(t, ok)
		require.Len(t, ambiguous, 2)
	})

	t.Run("the bundled name resolves the bundled key when nothing live claims it", func(t *testing.T) {
		// when: "Due date" is bundled dueDate's display name; the entries hold
		// no claimant and no installed dueDate
		entry, ok, ambiguous := fx.resolvePropertyInput("Due date", entries)

		// then: resolved but not installed — Id == "" is the step-3 contract
		assert.Empty(t, ambiguous)
		require.True(t, ok)
		assert.Equal(t, "dueDate", entry.Key)
		assert.Empty(t, entry.Id)
	})

	t.Run("the bundled name's fold class resolves too", func(t *testing.T) {
		// "due date" never reaches the bundled tables at step 4 (FoldApiKey
		// keeps the space) — the FoldKeyTerm arm owns it
		entry, ok, ambiguous := fx.resolvePropertyInput("due date", entries)

		assert.Empty(t, ambiguous)
		require.True(t, ok)
		assert.Equal(t, "dueDate", entry.Key)
	})

	t.Run("a live visible property NAMED like a bundled name refuses with both holders, the slug-step shadow discipline", func(t *testing.T) {
		// given: a custom property named "Due date" beside the (uninstalled)
		// bundled dueDate whose table binds that exact name — resolving either
		// silently would repeat the §7.5a-6 squatter defect on the name axis
		squatted := append([]propertyEntry{}, entries...)
		squatted = append(squatted, propertyEntry{Id: "rel-due-claim", Key: "6a7663db61fab21cd4b9e207", Name: "Due date"})

		// when
		_, ok, ambiguous := fx.resolvePropertyInput("Due date", squatted)

		// then
		assert.False(t, ok)
		require.Len(t, ambiguous, 2)
		assert.Contains(t, joinedCandidates(ambiguous), "rel-due-claim")
		assert.Contains(t, joinedCandidates(ambiguous), "the bundled")
		assert.Contains(t, joinedCandidates(ambiguous), "dueDate")
	})

	t.Run("an installed bundled property renamed live still answers its bundled name with its Id", func(t *testing.T) {
		// given: dueDate installed and renamed — the bundled table still binds
		// "Due date" to it, and the address routes need the live row's Id
		renamed := append([]propertyEntry{}, entries...)
		renamed = append(renamed, propertyEntry{Id: "rel-due-installed", Key: "dueDate", Name: "Deadline v2", Format: model.RelationFormat_date})

		// when
		entry, ok, ambiguous := fx.resolvePropertyInput("Due date", renamed)

		// then
		assert.Empty(t, ambiguous)
		require.True(t, ok)
		assert.Equal(t, "rel-due-installed", entry.Id)
	})
}

func TestResolveTypeInputDisplayNames(t *testing.T) {
	fx := newV2Fixture(t)
	entries := []typeEntry{
		{Id: "type-review", Key: "6a7663db61fab21cd4b9e301", Slug: "review_meeting", Name: "Sprint review"},
		{Id: "type-hidden", Key: "6a7663db61fab21cd4b9e302", Name: "Hidden kind", Hidden: true},
		{Id: "type-legacy", Key: "reviewkind", Name: "Legacy review"},
		{Id: "type-named-key", Key: "6a7663db61fab21cd4b9e303", Name: "reviewkind"},
	}

	t.Run("resolution table", func(t *testing.T) {
		cases := []struct {
			name    string
			input   string
			wantKey string
			wantOk  bool
		}{
			{"exact display name resolves the visible holder", "Sprint review", "6a7663db61fab21cd4b9e301", true},
			{"FoldKeyTerm variant with a dash resolves the name", "sprint-review", "6a7663db61fab21cd4b9e301", true},
			{"FoldKeyTerm variant with underscores and case resolves the name", "SPRINT_REVIEW", "6a7663db61fab21cd4b9e301", true},
			{"a hidden entry's name does not resolve", "Hidden kind", "", false},
			{"a name equal to another live entry's stored key loses to that key (step 1)", "reviewkind", "reviewkind", true},
			{"a full miss stays a miss", "no such type", "", false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				// when
				entry, ok, ambiguous := fx.resolveTypeInput(tc.input, entries)

				// then
				assert.Empty(t, ambiguous)
				assert.Equal(t, tc.wantOk, ok)
				assert.Equal(t, tc.wantKey, entry.Key)
			})
		}
	})

	t.Run("two visible types sharing a name are a loud ambiguity", func(t *testing.T) {
		twins := append([]typeEntry{}, entries...)
		twins = append(twins, typeEntry{Id: "type-review-twin", Key: "6a7663db61fab21cd4b9e304", Name: "Sprint review"})

		_, ok, ambiguous := fx.resolveTypeInput("Sprint review", twins)

		assert.False(t, ok)
		require.Len(t, ambiguous, 2)
	})

	t.Run("the bundled type name resolves the bundled key when nothing live claims it", func(t *testing.T) {
		// "Space member" is bundled participant's display name — a spelling no
		// slug step ever answered ("space_member" is not participant's slug)
		entry, ok, ambiguous := fx.resolveTypeInput("Space member", entries)

		assert.Empty(t, ambiguous)
		require.True(t, ok)
		assert.Equal(t, "participant", entry.Key)
		assert.Empty(t, entry.Id)
	})

	t.Run("the bundled type name's fold class resolves too", func(t *testing.T) {
		entry, ok, ambiguous := fx.resolveTypeInput("space member", entries)

		assert.Empty(t, ambiguous)
		require.True(t, ok)
		assert.Equal(t, "participant", entry.Key)
	})

	t.Run("a live visible type NAMED like a bundled name refuses with both holders, the slug-step shadow discipline", func(t *testing.T) {
		squatted := append([]typeEntry{}, entries...)
		squatted = append(squatted, typeEntry{Id: "type-member-claim", Key: "6a7663db61fab21cd4b9e305", Name: "Space member"})

		_, ok, ambiguous := fx.resolveTypeInput("Space member", squatted)

		assert.False(t, ok)
		require.Len(t, ambiguous, 2)
		assert.Contains(t, joinedCandidates(ambiguous), "type-member-claim")
		assert.Contains(t, joinedCandidates(ambiguous), "the bundled")
		assert.Contains(t, joinedCandidates(ambiguous), "participant")
	})
}

// One end-to-end per channel family: every channel rides the same two
// functions, so one PATCH op and one fields=/keycanon path pin the wiring.
func TestV2NameAddressedChannels(t *testing.T) {
	ctx := context.Background()

	t.Run("PATCH set_properties by display name lands the value on the stored key", func(t *testing.T) {
		// given: the round-trip §4.3 exists for — read "Manual property" from
		// a ?keys=name body, write it back through a PATCH op key
		fx := slugSpaceFixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"Manual property":"from-name"}}`), "", false, true)

		// then
		require.NoError(t, err)
		st := *captured
		assert.Equal(t, "from-name", st.CombinedDetails().GetString(slugPropKey),
			"the detail lands under the stored BSON key the name resolves to")
		assert.False(t, st.CombinedDetails().Has("Manual property"),
			"the display name must not become a detail key")
	})

	t.Run("fields= takes the display name and emits under it", func(t *testing.T) {
		// given: the keycanon channel (search fields, list fields, filters and
		// sorts all ride keyCanon.canon → the same chain)
		fx := slugQueryFixture(t)

		// when
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
			Fields: []string{"Manual property"},
		}, 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.NotNil(t, rows[0].Properties)
		assert.Equal(t, "hello", rows[0].Properties["Manual property"],
			"the value reads from the stored key and emits under the requested spelling")
	})

	t.Run("list fields validate a display name", func(t *testing.T) {
		fx := slugQueryFixture(t)
		assert.NoError(t, fx.validateListFields(testSpaceId, []string{"Manual property"}))
	})
}
