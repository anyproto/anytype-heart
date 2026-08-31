package anyblockjson

// internalkey_test.go pins the key/spelling split (§2, §2e): `internal_key`
// is the ONLY thing called a key that is a stored id, `property` is the
// document-facing spelling, and a property definition may be identified by
// either (or by a `name` the spelling derives from). The split exists
// because one word carried both meanings — the envelope held stored bson
// ids while the same member name held slugs one level down — which is the
// §15 #14 disease measured over a 77-space corpus.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// An entry identified by `internal_key` alone validates and imports, and the
// key is taken VERBATIM: a stored id is its own address (§3), so it never
// re-enters the slug ladder — not for a bundled key, not for a minted bson,
// and not even for a stored key that happens to look like a bundled slug.
//
// The last case is the one that pins the rule. `due_date` is the bundled
// table's slug for `dueDate`, and a term that reached the §3 ladder would be
// folded onto that bundled twin — which is exactly wrong for a member whose
// whole meaning is "this exact stored key": a space really can hold a shadow
// relation stored as `due_date` beside bundled `dueDate` (§3's identity-entry
// case), and `internal_key` is how a document addresses it with no legend.
func TestPropertyDefinitions_InternalKeyAloneIdentifiesVerbatim(t *testing.T) {
	for name, tc := range map[string]struct {
		internalKey string
		format      string
	}{
		"a bundled stored key":          {"dueDate", "date"},
		"a minted bson id":              {"6a83296f61fab2265263ae34", "number"},
		"a shadow key shaped as a slug": {"due_date", "date"},
	} {
		t.Run(name, func(t *testing.T) {
			// given
			doc := `{"version": 2, "kind": "object_type", "internal_key": "t",
				"type_settings": {"property_definitions": [
					{"internal_key": "` + tc.internalKey + `", "format": "` + tc.format + `", "section": "featured"}]}}`

			// when
			require.NoError(t, Validate([]byte(doc)))
			_, snapshot, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("id")})

			// then: without a resolver the resolved KEY passes through in
			// place of an id, which is what makes the resolution observable
			require.NoError(t, err)
			assert.Equal(t, strList(tc.internalKey),
				snapshot.Details.Fields["recommendedFeaturedRelations"],
				"the stored key travels verbatim — no ladder, no fold, no rebinding")
		})
	}
}

// When an entry states both members, `property` outranks `internal_key`. On
// export's own output the two agree — both are written from one stored key —
// so the order only decides a DISAGREEING authored pair, and there the
// spelling wins because it is the member the document's own legend speaks
// for (§3 chain step 1). authoredKey is the one place the order lives.
func TestPropertyDefinitions_PropertyOutranksInternalKey(t *testing.T) {
	// given a pair that disagrees on purpose
	doc := `{"version": 2, "kind": "object_type", "internal_key": "t",
		"property_internal_keys": {"budget": "6a83296f61fab2265263ae34"},
		"type_settings": {"property_definitions": [
			{"property": "budget", "internal_key": "somethingElse", "format": "number", "section": "featured"}]}}`

	// when
	_, snapshot, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("id")})

	// then: the spelling resolved through the legend, the internal_key lost
	require.NoError(t, err)
	assert.Equal(t, strList("6a83296f61fab2265263ae34"),
		snapshot.Details.Fields["recommendedFeaturedRelations"])
}

// The identity anyOf admits each of the three members alone and refuses an
// entry with none — in BOTH homes of the shape (§2e).
func TestPropertyDefinitions_IdentityIsPropertyOrInternalKeyOrName(t *testing.T) {
	entryDoc := func(entry string) string {
		return `{"version": 2, "kind": "object_type", "internal_key": "t",
			"type_settings": {"property_definitions": [` + entry + `]}}`
	}
	t.Run("each identity member alone is enough", func(t *testing.T) {
		for _, entry := range []string{
			`{"property": "budget", "format": "number"}`,
			`{"internal_key": "6a83296f61fab2265263ae34", "format": "number"}`,
			`{"name": "Budget", "format": "number"}`,
		} {
			assert.NoError(t, Validate([]byte(entryDoc(entry))), entry)
		}
	})
	t.Run("an entry with no identity at all is refused", func(t *testing.T) {
		err := Validate([]byte(entryDoc(`{"format": "number"}`)))
		require.Error(t, err)
	})
	t.Run("the dictionary home says the same", func(t *testing.T) {
		_, err := UnmarshalPropertyDictionary([]byte(
			`{"version":2,"properties":[{"internal_key":"6a83296f61fab2265263ae34","format":"number"}]}`))
		assert.NoError(t, err)
		_, err = UnmarshalPropertyDictionary([]byte(
			`{"version":2,"properties":[{"format":"number"}]}`))
		require.Error(t, err)
	})
}

// A dictionary entry's `internal_key` is verbatim too — same rule, third
// home: the fold ladder that recovers a stored key from a `property`
// spelling must not touch a member that IS the stored key.
func TestPropertyDictionary_InternalKeyIsVerbatim(t *testing.T) {
	d, err := UnmarshalPropertyDictionary([]byte(`{"version":2,"properties":[
		{"internal_key":"due_date","format":"date"},
		{"internal_key":"6a83296f61fab2265263ae34","name":"Budget","format":"number"}]}`))
	require.NoError(t, err)
	require.Len(t, d.Properties, 2)
	assert.Equal(t, "due_date", string(d.Properties[0].Key),
		"a slug-shaped stored key stays itself — the fold onto bundled dueDate is for spellings")
	assert.Equal(t, "6a83296f61fab2265263ae34", string(d.Properties[1].Key))
}

// Export writes BOTH members on a dictionary entry: `property` in the
// spelling every document uses, `internal_key` the stored key verbatim —
// fidelity an author never has to produce (§2f).
func TestPropertyDictionary_ExportWritesBothIdentityMembers(t *testing.T) {
	out, err := MarshalPropertyDictionary(&PropertyDictionary{Properties: []PropertyDefinition{
		{Key: "dueDate", Name: "End", Format: model.RelationFormat_date},
		{Key: "6a83296f61fab2265263ae34", Name: "Budget", Format: model.RelationFormat_number},
	}})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"property": "Due date"`)
	assert.Contains(t, string(out), `"internal_key": "dueDate"`)
	assert.Contains(t, string(out), `"property": "6a83296f61fab2265263ae34"`,
		"a bson id has no slug and must never be given one (§2f)")
	assert.Contains(t, string(out), `"internal_key": "6a83296f61fab2265263ae34"`)
}

// One property, one slot — whichever member names it. Two entries that state
// one identity through the two different members are still two definitions
// of one property, refused with the first occurrence named (§2e, §2f).
func TestPropertyDictionary_DuplicateAcrossTheTwoIdentityMembers(t *testing.T) {
	_, err := UnmarshalPropertyDictionary([]byte(`{"version":2,"properties":[
		{"property":"6a83296f61fab2265263ae34","format":"number"},
		{"internal_key":"6a83296f61fab2265263ae34","format":"text"}]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/properties/1/property")
	assert.Contains(t, err.Error(), "already defined at /properties/0")
}

// property_settings refuses the identity pair the way it refused `key`: a
// relation document's stored key is the envelope `internal_key`, and its
// spelling is derived, never stated — admitting either member would be a
// second spelling of a fact another surface owns (§2d).
func TestRelationSettings_RefusesTheIdentityPair(t *testing.T) {
	for member, wantHome := range map[string]string{
		"property":     "internal_key",
		"internal_key": "envelope",
	} {
		t.Run(member, func(t *testing.T) {
			err := Validate([]byte(`{"version":2,"kind":"property","id":"o1","internal_key":"b",
				"property_settings":{"format":"number","` + member + `":"x"}}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "/property_settings/"+member)
			assert.Contains(t, strings.ToLower(err.Error()), wantHome,
				"the refusal names where the fact lives")
		})
	}
}

// An unwritable `internal_key` member is refused with the member's own path
// and a readable reason — the same writable-key rule every stored-key slot
// carries (§3), stated where the fault is instead of as a bare schema bound.
func TestPropertyDefinitions_UnwritableInternalKeyIsRefusedByName(t *testing.T) {
	err := Validate([]byte(`{"version": 2, "kind": "object_type", "internal_key": "t",
		"type_settings": {"property_definitions": [{"internal_key": "a\nb", "format": "text"}]}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/type_settings/property_definitions/0/internal_key")
}
