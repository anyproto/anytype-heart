package anyblockjson

// manifest_test.go pins the §2c index manifest: the one place a reader can
// find a type by stored key or an option by id without a folder convention
// — which the spec has never defined — and without scanning.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The manifest round-trips byte-stably through the index's own two entry
// points, keys sorted — the canonical form (§4).
//
// How this can fail: drop Manifest from the Index struct or from
// MarshalIndex (the maps vanish on the way round); stop sorting the two
// lookup tables (the second marshal reorders and the byte check goes red);
// or write an empty manifest object (the §4 omit-empty canon breaks and a
// bare `"manifest": {}` ships on every index).
func TestIndex_ManifestRoundTrip(t *testing.T) {
	// given
	in := &Index{
		Name: "Corpus",
		Manifest: &Manifest{
			Types:      map[string]string{"task": "types/bafytask.anyblock.json", "page": "types/bafypage.anyblock.json"},
			Options:    map[string]string{"bafyopt1": "relationsOptions/bafyopt1.anyblock.json"},
			Properties: PropertiesFileName,
		},
	}

	// when
	data, err := MarshalIndex(in)
	require.NoError(t, err)
	got, err := UnmarshalIndex(data)
	require.NoError(t, err)
	data2, err := MarshalIndex(got)
	require.NoError(t, err)

	// then
	assert.Equal(t, string(data), string(data2), "Marshal ∘ Unmarshal must be byte-stable")
	require.NotNil(t, got.Manifest)
	assert.Equal(t, in.Manifest.Types, got.Manifest.Types)
	assert.Equal(t, in.Manifest.Options, got.Manifest.Options)
	assert.Equal(t, PropertiesFileName, got.Manifest.Properties)

	// an empty manifest is not written at all
	bare, err := MarshalIndex(&Index{Name: "Corpus", Manifest: &Manifest{}})
	require.NoError(t, err)
	assert.NotContains(t, string(bare), "manifest")
}

// The manifest is closed: `additionalProperties: false` on the index root
// already made an undeclared root member invalid, and the manifest's own
// gate extends that inside — an undeclared member here would be a fourth
// place to put a lookup table, unread by every reader.
//
// How this can fail: drop additionalProperties: false from the manifest
// $defs (first case green), or loosen the value type of a lookup table
// (second case green on a path nobody can open).
func TestIndex_ManifestRefusals(t *testing.T) {
	t.Run("an undeclared manifest member is refused", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version":1,"manifest":{"templates":{}}}`))
		require.Error(t, err)
	})
	t.Run("a non-string path is refused", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version":1,"manifest":{"types":{"task":42}}}`))
		require.Error(t, err)
	})
	t.Run("an empty path is refused", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version":1,"manifest":{"properties":""}}`))
		require.Error(t, err)
	})
}
