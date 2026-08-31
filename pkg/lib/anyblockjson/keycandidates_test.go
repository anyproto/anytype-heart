package anyblockjson

// keycandidates_test.go — what the importer does with the two LISTS a
// space-backed vocabulary hands it: the candidates for a spelling, and the
// declared type's own property keys. Both are read as counts — "how many live
// entities answer to this spelling", "does the type single one of them out" —
// and a count is exactly the thing a bookkeeping slip in the producer can
// falsify without corrupting anything else. The vocabulary here repeats every
// entry on purpose, because the shipped vocabulary's own guard against that
// (storeresolver's addClaimant) is one append away from being lost again and
// ScopedKeyVocabulary is a public interface Options.Keys accepts from anyone.
//
// The second half pins WHERE an ambiguity is reported. The refusal used to
// name `/property_internal_keys`, a member that is absent in precisely the
// documents that trigger it — the missing legend is the fix being asked for,
// so pointing at it tells a reader to look at nothing.

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

// dupVocab is nameVocab with every list answer doubled — one entity reported
// twice, in both namespaces and in the type's property scope.
type dupVocab struct{ nameVocab }

func doubled(keys []string) []string {
	out := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		out = append(out, key, key)
	}
	return out
}

func (v dupVocab) PropertyKeyCandidates(spelling string) []string {
	return doubled(v.nameVocab.PropertyKeyCandidates(spelling))
}

func (v dupVocab) TypeKeyCandidates(spelling string) []string {
	return doubled(v.nameVocab.TypeKeyCandidates(spelling))
}

func (v dupVocab) TypePropertyKeys(typeKey string) []string {
	return doubled(v.nameVocab.TypePropertyKeys(typeKey))
}

// A repeated candidate is one entity, not two. Every arm here would round-trip
// under a vocabulary that counts correctly; the point is that the importer
// must not turn a producer's slip into a refused document, because a refusal
// is the one outcome the reader cannot recover from — it has no row count to
// compare the list against.
func TestKeyCandidates_ARepeatedCandidateIsNotAnAmbiguity(t *testing.T) {
	const (
		keyA     = "6a7663db61fab21cd4b9c001"
		keyB     = "6a7663db61fab21cd4b9c002"
		typeKey  = "6a7663db61fab21cd4b9c003"
		typeName = "Sprint"
	)

	t.Run("a property listed twice still binds its value", func(t *testing.T) {
		// given
		vocab := dupVocab{nameVocab{names: map[string]string{keyA: "Projects"}}}
		doc := `{"version":2,"id":"o1","properties":{"Projects":"kept"}}`

		// when
		_, back, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})

		// then
		require.NoError(t, err, "one entity named twice is still one entity")
		assert.Equal(t, "kept", back.Details.Fields[keyA].GetStringValue())
	})

	t.Run("a type listed twice still binds the envelope", func(t *testing.T) {
		// given
		vocab := dupVocab{nameVocab{typeNames: map[string]string{typeKey: typeName}}}
		doc := `{"version":2,"id":"o1","type":"` + typeName + `"}`
		want := []string{domain.TypeKey(typeKey).URL()}

		// when
		_, back, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})

		// then
		require.NoError(t, err, "the type namespace has no wider scope to recover in")
		assert.Equal(t, want, back.ObjectTypes)
	})

	t.Run("a genuinely shared name is still resolved inside a doubled scope", func(t *testing.T) {
		// given — two live properties really do bear the name, and the type
		// names its own one TWICE: the intersection has to be counted by
		// distinct key, or the type stops being able to single out the
		// property it declares
		vocab := dupVocab{nameVocab{
			names:     map[string]string{keyA: "Projects", keyB: "Projects"},
			typeNames: map[string]string{typeKey: typeName},
			typeProps: map[string][]string{typeKey: {keyA}},
		}}
		doc := `{"version":2,"id":"o1","type":"` + typeName + `","properties":{"Projects":"resolved"}}`

		// when
		_, back, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})

		// then
		require.NoError(t, err)
		assert.Equal(t, "resolved", back.Details.Fields[keyA].GetStringValue())
		assert.Nil(t, back.Details.Fields[keyB], "the scope picked one, and only one")
	})
}

// Where the loud refusal points. The message asks for a legend entry, which
// is right — the legend is what settles a shared name — but the POINTER has
// to name the slot that spelled the term, because that is the only place in
// the document a reader can go and look.
func TestKeyCandidates_TheAmbiguityNamesTheOffendingSlot(t *testing.T) {
	const (
		keyA = "6a7663db61fab21cd4b9c011"
		keyB = "6a7663db61fab21cd4b9c022"
	)
	shared := nameVocab{names: map[string]string{keyA: "Projects", keyB: "Projects"}}

	refusal := func(t *testing.T, doc string, vocab nameVocab) Issue {
		t.Helper()
		_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})
		require.Error(t, err)
		var invalid *ValidationError
		require.True(t, errors.As(err, &invalid), "a refusal, not a decode failure: %v", err)
		require.Len(t, invalid.Issues, 1)
		return invalid.Issues[0]
	}

	t.Run("a properties member names its own slot", func(t *testing.T) {
		// given — no legend, which is what makes the term ambiguous in the
		// first place, so pointing at the legend points at nothing
		doc := `{"version":2,"id":"o1","type":"Page","properties":{"Projects":"?"}}`
		want := "/properties/Projects"

		// when
		got := refusal(t, doc, shared)

		// then
		assert.Equal(t, want, got.Path)
		assert.Contains(t, got.Message, `"Projects"`)
		assert.Contains(t, got.Message, memberPropertyInternalKeys,
			"the message still asks for the legend — that is the repair")
	})

	t.Run("a block slot names the block section", func(t *testing.T) {
		// given — these slots are built without a pointer of their own, so
		// the coarse-but-true section is the honest answer, exactly as the
		// empty-key refusal beside it already reports
		doc := `{"version":2,"id":"o1","type":"Page","blocks":[{"id":"dv","type":"dataview",` +
			`"properties":[{"property":"Projects","format":"text"}]}]}`
		want := "/blocks"

		// when
		got := refusal(t, doc, shared)

		// then
		assert.Equal(t, want, got.Path)
	})

	t.Run("the type envelope names /type", func(t *testing.T) {
		// given
		sharedTypes := nameVocab{typeNames: map[string]string{
			"6a7663db61fab21cd4b9c077": "Meeting",
			"6a7663db61fab21cd4b9c088": "Meeting",
		}}
		doc := `{"version":2,"id":"o1","type":"Meeting"}`
		want := "/type"

		// when
		got := refusal(t, doc, sharedTypes)

		// then
		assert.Equal(t, want, got.Path, "the type namespace has carried its slot pointer all along")
		assert.Contains(t, got.Message, memberTypeInternalKeys)
	})
}
