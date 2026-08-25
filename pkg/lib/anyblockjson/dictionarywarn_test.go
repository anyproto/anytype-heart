package anyblockjson

// dictionarywarn_test.go — the property dictionary is the one file whose keys
// are STORED keys, and what happens when an author writes the other spelling
// (§2f).

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dictHead = `"$schema":"https://schemas.anytype.io/anyblock/1/properties.schema.json","version":1,`

func readDict(t *testing.T, doc string) (*PropertyDictionary, []Issue) {
	t.Helper()
	var warns []Issue
	d, err := UnmarshalPropertyDictionaryWarn([]byte(doc), func(i Issue) { warns = append(warns, i) })
	require.NoError(t, err)
	return d, warns
}

// Every other slot in the format spells a property the way a document does —
// `due_date` — and the dictionary alone spells it `dueDate`. Within ONE real
// exported bundle an object document says `created_date` while the dictionary
// beside it says `createdDate`, so writing the document's spelling here is
// not an exotic mistake; it is the obvious one.
//
// It used to read clean with no warning and fail only when the dictionary was
// written back out, which is the wrong end: the bundle had already shipped.
//
// How this can fail: drop the fold recovery and a bundle that reads fine
// cannot be re-rendered; drop the warning and the author is never told which
// spelling this file wanted.
func TestDictionary_TheLabelSpellingIsRecoveredAndReported(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+`"installed":["due_date","created_date"]}`)

	assert.Equal(t, []string{"dueDate", "createdDate"}, d.Installed,
		"read as the stored keys they name")
	require.Len(t, warns, 2)
	assert.Equal(t, "/installed/0", warns[0].Path)
	assert.Contains(t, warns[0].Message, `"dueDate"`, "the warning names the spelling to write")

	// and the recovered dictionary is writable, where the original was not
	_, err := MarshalPropertyDictionary(d)
	require.NoError(t, err, "recovery is what closes the read/write disagreement")
}

// An entry carries a full definition, so its key is NOT recovered: a custom
// property deliberately named close to a bundled one is legitimate, and
// rewriting the key would change what the entry defines. But an entry keyed
// `due_date` beside the bundled `dueDate` defines a SECOND property that only
// looks like the first, and every document referring to the bundled one goes
// on referring to it — so the shadow collects nothing.
func TestDictionary_AnEntryThatShadowsABundledProperty(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+
		`"properties":[{"key":"due_date","name":"Due date","format":"date"}]}`)

	require.Len(t, d.Properties, 1)
	assert.EqualValues(t, "due_date", d.Properties[0].Key, "the entry’s own key is never rewritten")
	require.Len(t, warns, 1)
	assert.Equal(t, "/properties/0/key", warns[0].Path)
	assert.Contains(t, warns[0].Message, `"dueDate"`)

	t.Run("a genuinely custom key is not a shadow", func(t *testing.T) {
		_, warns := readDict(t, `{`+dictHead+
			`"properties":[{"key":"estimateHours","name":"Estimate","format":"number"}]}`)
		assert.Empty(t, warns, "only a key that folds onto a BUNDLED one is a shadow")
	})
}

// The tolerance is deliberate and must survive: the bundled table grows
// independently of the format version, so a backup written by a newer app
// lists keys this reader has never heard of. Refusing them would make every
// backup unreadable one app version back. What was missing is that nothing
// said so.
//
// How this can fail: turn the warning into an error and a newer app's backup
// stops reading; recover such a key by guess and it silently becomes a
// different property.
func TestDictionary_AKeyFromANewerAppIsToleratedAndReported(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+`"installed":["someKeyThisBuildHasNeverHeardOf"]}`)

	assert.Equal(t, []string{"someKeyThisBuildHasNeverHeardOf"}, d.Installed,
		"kept verbatim — it may be the newer app's")
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0].Message, "installs NOTHING for it")
	assert.Contains(t, warns[0].Message, "newer app", "the tolerance is explained, not just the fault")
}

// The stored spelling is the canonical one and must stay silent, or the
// warnings that matter get ignored.
func TestDictionary_TheStoredSpellingWarnsAboutNothing(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+
		`"installed":["dueDate","createdDate"],`+
		`"properties":[{"key":"estimate","name":"Estimate","format":"number"}]}`)

	assert.Equal(t, []string{"dueDate", "createdDate"}, d.Installed)
	assert.Empty(t, warns)
}

// UnmarshalPropertyDictionary is UnmarshalPropertyDictionaryWarn with no
// sink: the same verdicts, the warnings discarded — the relationship Validate
// and ValidateWarn have.
func TestDictionary_TheSinklessDoorAgrees(t *testing.T) {
	doc := `{` + dictHead + `"installed":["due_date"]}`

	quiet, err := UnmarshalPropertyDictionary([]byte(doc))
	require.NoError(t, err)
	loud, _ := readDict(t, doc)
	assert.Equal(t, loud, quiet)
}
