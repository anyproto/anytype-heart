package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
)

// The two tools must agree about one bundle. §2f gave a property format TWO
// homes — a type's `type_settings.property_definitions` and the dictionary —
// and anyblockconvert merges the dictionary into the format table before
// running CheckPropertyFormats. When this tool did not, it refused a bundle
// the converter converted cleanly, and the repair text sent the author to the
// type home, undoing the dictionary they had correctly written.
//
// How this can fail: drop the dictionary merge in main.go and the bundle
// below reports undeclared formats that anyblockconvert accepts.
func TestValidate_AgreesWithConvertOnADictionaryDeclaredBundle(t *testing.T) {
	// given a bundle whose formats are declared ONLY in the dictionary
	dir := t.TempDir()
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	write("properties.json", `{"version":1,"properties":[
		{"property":"episode_number","name":"Episode Number","format":"number"},
		{"property":"release_date","name":"Release Date","format":"date"}]}`)
	write("ep1.json", `{"version":1,"kind":"page","id":"bafyreiep1",
		"properties":{"name":"Ep 1","episode_number":1,"release_date":"2026-01-01T00:00:00Z"}}`)

	files := []string{filepath.Join(dir, "ep1.json")}

	// when the format table is built the way this tool builds it
	formats, err := anyblockbatch.ScanFormats(files)
	require.NoError(t, err)
	dictFormats, _, err := anyblockbatch.DictionaryFormats(filepath.Join(dir, "properties.json"))
	require.NoError(t, err)
	merged := anyblockbatch.MergeDictionaryFormats(formats, dictFormats, func(string, ...any) {})

	// then nothing is undeclared — which is what anyblockconvert concludes
	undeclared, err := anyblockbatch.CheckPropertyFormats(files, merged)
	require.NoError(t, err)
	assert.Empty(t, undeclared, "a dictionary-declared format is declared (§2f)")

	// and without the merge it would have been refused, which is the bug.
	// Re-scan rather than reusing `formats`: MergeDictionaryFormats folds
	// into the map it is given, so the pre-merge table is gone by now.
	bare, err := anyblockbatch.ScanFormats(files)
	require.NoError(t, err)
	withoutDict, err := anyblockbatch.CheckPropertyFormats(files, bare)
	require.NoError(t, err)
	assert.Len(t, withoutDict, 2, "the merge is what makes the two tools agree")
}

// The repair sentence must name BOTH homes. Naming only the type home sends
// an author who declared the property in the dictionary to declare it again
// somewhere else.
//
// How this can fail: drop either home from Report's message.
func TestReport_NamesBothDeclarationHomes(t *testing.T) {
	msg := anyblockbatch.Report([]anyblockbatch.Undeclared{{File: "x.json", Key: "episode_number"}})
	assert.Contains(t, msg, "properties.json")
	assert.Contains(t, msg, "type_settings.property_definitions")
}
