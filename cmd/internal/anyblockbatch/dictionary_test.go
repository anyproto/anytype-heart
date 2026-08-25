package anyblockbatch

// dictionary_test.go pins the batch's §2f wiring: the property dictionary is
// a declaration source beside the type documents, and the used-key scan is
// what decides which properties the dictionary must name.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// DictionaryFormats reads properties.json into the same table ScanFormats
// builds — keyed by STORED key, since that is what the dictionary spells
// (§2f), with the declared vocabulary carried along so newBatch pre-mints
// it in declaration order.
//
// How this can fail: shed Options on the way into FormatInfo (the
// vocabulary assertion goes red and a dictionary-declared select mints no
// options), or decode `format` literally so a bundled short-text entry
// mints longtext.
func TestDictionaryFormats_ReadsEntries(t *testing.T) {
	// given
	dir := t.TempDir()
	path := filepath.Join(dir, "properties.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,
		"installed":["tag"],
		"properties":[
			{"key":"6a32d4856761631534b22f85","name":"Stage","format":"select",
			 "options":["Now",{"name":"Later","color":"blue"}]}]}`), 0o644))

	// when
	formats, defs, err := DictionaryFormats(path)

	// then
	require.NoError(t, err)
	require.Len(t, defs, 1)
	fi, ok := formats["6a32d4856761631534b22f85"]
	require.True(t, ok, "the table is keyed by the stored key the file spells")
	assert.Equal(t, model.RelationFormat_status, fi.Format)
	assert.Equal(t, "Stage", fi.Name)
	require.Len(t, fi.Options, 2)
	assert.Equal(t, "Now", fi.Options[0].Name)
	assert.Equal(t, "blue", fi.Options[1].Color)
}

// On a conflict the dictionary wins, and the conflict is SAID: a type entry
// disagreeing with the dictionary means the bundle contradicts itself, and
// silence would let whichever file loaded last decide. A type-declared
// vocabulary survives when the dictionary entry declares none.
//
// How this can fail: keep first-seen on conflict (the format assertion goes
// red), stop warning (the warned assertion), or overwrite the options
// unconditionally (the vocabulary assertion).
func TestMergeDictionaryFormats_DictionaryWins(t *testing.T) {
	// given
	scanned := map[string]FormatInfo{
		"budget": {Format: model.RelationFormat_longtext, FormatName: "text", Name: "Budget"},
		"stage": {Format: model.RelationFormat_status, FormatName: "select", Name: "Stage",
			Options: []anyblockjson.OptionDefinition{{Name: "Now"}, {Name: "Later"}}},
	}
	dict := map[string]FormatInfo{
		"budget": {Format: model.RelationFormat_number, FormatName: "number", Name: "Budget"},
		"stage":  {Format: model.RelationFormat_status, FormatName: "select", Name: "Stage"},
	}
	var warned []string
	warn := func(format string, args ...any) { warned = append(warned, format) }

	// when
	got := MergeDictionaryFormats(scanned, dict, warn)

	// then
	assert.Equal(t, model.RelationFormat_number, got["budget"].Format, "the dictionary wins the conflict")
	require.Len(t, warned, 1, "and the conflict is said")
	assert.Equal(t, []anyblockjson.OptionDefinition{{Name: "Now"}, {Name: "Later"}}, got["stage"].Options,
		"a type-declared vocabulary survives a dictionary entry that declares none")
}

// UsedPropertyKeys resolves through the same chain every scan runs — the
// document's own property_internal_keys legend, the bundled table, verbatim — and
// counts the two slots that reference a property: a `properties` member and
// a property-definition entry. `id`/`type` are envelope facts and never
// count.
//
// How this can fail: key the set by the raw spelling (the legend-backed
// case resolves to `severity` instead of the stored bson and the dictionary
// misses the real key), or start counting envelope members.
func TestUsedPropertyKeys_ResolvesTheChain(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"objects/a.json": `{"version":1,
			"property_internal_keys": {"severity": "6a32d4856761631534b22f85"},
			"properties": {"severity": "high", "due_date": "2026-01-01", "id": "a1", "type": "task"}}`,
		"types/t.json": `{"version":1,"kind":"object_type","key":"task",
			"type_settings":{"property_definitions":[{"key":"assignee","format":"objects"}]}}`,
	})

	used, err := UsedPropertyKeys(files)

	require.NoError(t, err)
	assert.True(t, used["6a32d4856761631534b22f85"], "the legend binds the spelling to the stored key")
	assert.True(t, used["dueDate"], "the bundled table binds the slug")
	assert.True(t, used["assignee"], "a property-definition entry is a reference")
	assert.False(t, used["severity"], "the spelling itself is not a key")
	assert.False(t, used["id"], "envelope facts are not property references")
	assert.False(t, used["type"], "envelope facts are not property references")
}
