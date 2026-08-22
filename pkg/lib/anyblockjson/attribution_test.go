package anyblockjson

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The real shape, 135 characters of it, on the account that produced the
// corpus these numbers come from.
const testParticipantId = "_participant_bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq_30afw2fe3tvff_AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"

// `creator` and `lastModifiedBy` are DERIVED: recovered from the object
// tree root's own signature on every rebuild, and discarded by every write
// path. A document naming one is telling a reader who wrote the object, not
// setting anything — so import drops the key instead of refusing it, in every
// spelling, and whatever shape the value arrives in.
//
// Before this, the two behaved differently for no reason anyone could state:
// `creator` was accepted (it sat in propertiesKeptOnExport, so the deny rule
// never saw it) and landed a detail that the next rebuild overwrote, while
// `lastModifiedBy` — the same relation definition, one word apart in the
// bundle — was refused outright, making 71 documents in a 36,966-object
// corpus unimportable for a line neither side could act on.
//
// How these can fail: drop the keys from derivedAttributionProperties and
// every case flips to an error (the deny rule reaches them the moment they
// are not exempt, since export strips both); wire the exemption so that it
// only skips the ERROR and not the write, and the NotContains assertions
// fail with the value on details.
func TestAttributionProperties_DroppedNotRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		doc string
		key string
	}{
		"creator, as a member name (what export writes)": {
			doc: `{"version": 1, "properties": {"creator": "Roman"}}`,
			key: "creator",
		},
		"creator, as the id array older exports wrote": {
			doc: fmt.Sprintf(`{"version": 1, "properties": {"creator": [%q]}}`, testParticipantId),
			key: "creator",
		},
		"last_modified_by, the spelling that used to be refused": {
			doc: `{"version": 1, "properties": {"last_modified_by": "Roman"}}`,
			key: "lastModifiedBy",
		},
		"last_modified_by, as an id array": {
			doc: fmt.Sprintf(`{"version": 1, "properties": {"last_modified_by": [%q]}}`, testParticipantId),
			key: "lastModifiedBy",
		},
		"the stored spelling drops too": {
			doc: `{"version": 1, "properties": {"lastModifiedBy": "Roman"}}`,
			key: "lastModifiedBy",
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given the document above

			// when
			validateErr := Validate([]byte(tc.doc))
			_, snap, err := Unmarshal([]byte(tc.doc), Options{})

			// then
			require.NoError(t, validateErr, "an attribution line makes a document stale, not wrong")
			require.NoError(t, err, "Validate and Unmarshal agree (§11 I2)")
			assert.NotContains(t, snap.GetDetails().GetFields(), tc.key,
				"and it must not reach the snapshot: the value is derived, not input")
		})
	}
}

// The control that keeps this from spreading. `assignee` points at a
// participant too, and looks identical in a document — but it is
// `source: details, maxCount: 0`, chosen by a person, and the id is the whole
// of its meaning. Dropping it, or spelling it as a name, would be data loss.
//
// How this can fail: add assignee (or author, or any `source: details`
// property) to derivedAttributionProperties and the value stops arriving.
func TestAttributionProperties_UserChosenParticipantsAreUntouched(t *testing.T) {
	for _, key := range []string{"assignee", "author", "stakeholders"} {
		t.Run(key, func(t *testing.T) {
			// given
			doc := fmt.Sprintf(`{"version": 1, "properties": {%q: [%q]}}`, key, testParticipantId)

			// when
			_, snap, err := Unmarshal([]byte(doc), Options{})

			// then
			require.NoError(t, err)
			require.Contains(t, snap.GetDetails().GetFields(), key)
			assert.Equal(t, []string{testParticipantId},
				valueStringList(snap.GetDetails().GetFields()[key]),
				"a user-chosen participant reference keeps its full id")
		})
	}
}

// The legend resolves a spelling onto a stored key before admission runs
// (§3), so it is the way a document could smuggle a key past a rule keyed on
// the spelling. It must reach the same verdict here: dropped, not landed and
// not refused.
func TestAttributionProperties_LegendCannotLandThem(t *testing.T) {
	for name, tc := range map[string]struct{ doc, key string }{
		"creator":        {`{"version": 1, "property_keys": {"who": "creator"}, "properties": {"who": "Roman"}}`, "creator"},
		"lastModifiedBy": {`{"version": 1, "property_keys": {"who": "lastModifiedBy"}, "properties": {"who": "Roman"}}`, "lastModifiedBy"},
	} {
		t.Run(name, func(t *testing.T) {
			// when
			validateErr := Validate([]byte(tc.doc))
			_, snap, err := Unmarshal([]byte(tc.doc), Options{})

			// then
			require.NoError(t, validateErr)
			require.NoError(t, err)
			assert.NotContains(t, snap.GetDetails().GetFields(), tc.key)
			assert.NotContains(t, snap.GetDetails().GetFields(), "who")
		})
	}
}
