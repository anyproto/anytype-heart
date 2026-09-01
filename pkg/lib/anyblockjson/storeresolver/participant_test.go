package storeresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The participant ids are the real shape: `_participant_<space>_<account>`,
// 135 characters, which is what makes naming the member worth doing at all.
const (
	namedParticipant   = "_participant_bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq_30afw2fe3tvff_AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"
	unnamedParticipant = "_participant_bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq_30afw2fe3tvff_AAJmVGvhCctWjPUeQxpZjbQCbLcVeCTHM7LmJRCzEnHwqXBt"
	absentParticipant  = "_participant_bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq_30afw2fe3tvff_AAHTtt8gtEhk9vPBFdrpxNXFrYaZQBS4rjhBLTGRRfDrDwLA"
)

// participantFixture builds a resolver over a space holding two members: one
// with a profile name, one who never set one.
func participantFixture(t *testing.T) *Resolvers {
	index := spaceindex.NewStoreFixture(t)
	index.AddObjects(t, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:             domain.String(namedParticipant),
			bundle.RelationKeyName:           domain.String("Roman"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		},
		{
			bundle.RelationKeyId:             domain.String(unnamedParticipant),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		},
	})
	return New(index)
}

// ParticipantName is what turns a 135-character id into the one thing a
// reader of the document wants from it. Three answers, and the two negative
// ones matter as much as the positive: export writes the property only when
// this says yes, so a "yes, empty string" would put a blank `creator` on
// every object whose author never named themselves.
//
// How these can fail: a lookup keyed on anything but the object id (a unique
// key, a relation key) misses the named row and the first case fails; reading
// any field but `name` — `identity`, `globalName`, the id itself — fails it
// too; and returning true for an empty name fails the second and third.
func TestParticipantName(t *testing.T) {
	t.Run("a named member resolves to the name", func(t *testing.T) {
		// given
		r := participantFixture(t)

		// when
		name, ok := r.ParticipantName(namedParticipant)

		// then
		require.True(t, ok)
		assert.Equal(t, "Roman", name)
	})

	t.Run("a member with no profile name answers no", func(t *testing.T) {
		// given
		r := participantFixture(t)

		// when
		name, ok := r.ParticipantName(unnamedParticipant)

		// then
		assert.False(t, ok, "an unnamed member must not export as a blank name")
		assert.Empty(t, name)
	})

	t.Run("an id this space has no participant for answers no", func(t *testing.T) {
		// given
		r := participantFixture(t)

		// when
		name, ok := r.ParticipantName(absentParticipant)

		// then
		assert.False(t, ok)
		assert.Empty(t, name, "the id must never be its own answer (§3)")
	})

	t.Run("the answer is cached, misses included", func(t *testing.T) {
		// given
		r := participantFixture(t)

		// when
		first, _ := r.ParticipantName(namedParticipant)
		_, missOk := r.ParticipantName(absentParticipant)
		second, secondOk := r.ParticipantName(namedParticipant)

		// then
		assert.Equal(t, first, second)
		assert.True(t, secondOk)
		assert.False(t, missOk)
		assert.Len(t, r.participantNames, 2, "one entry per id asked about, hit or miss")
	})
}

// The wiring: a node-backed export gets the participant resolver without the
// caller naming it, exactly as it gets the option and property resolvers.
// Without this the seam exists and nothing in the product reaches it.
func TestOptions_CarriesParticipantResolver(t *testing.T) {
	// given
	r := participantFixture(t)

	// when
	opts := r.Options()

	// then
	require.NotNil(t, opts.ResolveParticipants)
	name, ok := opts.ResolveParticipants.ParticipantName(namedParticipant)
	require.True(t, ok)
	assert.Equal(t, "Roman", name)
}
