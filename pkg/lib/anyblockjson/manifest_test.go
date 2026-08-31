package anyblockjson

// manifest_test.go pins the §2c index manifest: the one place a reader can
// find a type by stored key without a folder convention — which the spec has
// never defined — and without scanning.

import (
	"strings"
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
		_, err := UnmarshalIndex([]byte(`{"version":2,"manifest":{"templates":{}}}`))
		require.Error(t, err)
	})
	t.Run("a non-string path is refused", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version":2,"manifest":{"types":{"task":42}}}`))
		require.Error(t, err)
	})
	t.Run("an empty path is refused", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version":2,"manifest":{"properties":""}}`))
		require.Error(t, err)
	})
}

// The manifest located options until v0.46: 2,641 entries across a 77-space
// export, option object id → the option document's path.
//
// It went because a manifest answers a lookup a reader would otherwise scan
// for, and no reader has that lookup for an option. The dictionary states a
// property's whole vocabulary inline — every option's name, colour, position
// and, since the vocabulary learned `internal_key`, its stored key — so
// everything an option MEANS is in hand before a single document is opened.
// The entries pointed at documents nothing needed to read.
//
// `option_ids` is a different job and is untouched: it carries option OBJECT
// ids, resolved against the IMPORTING space's live store so a value survives
// a rename (§9a), never against the bundle. It never needed a path beside it.
//
// How this can fail: keep writing the member and every index carries a map
// no reader consults; refuse it instead of ignoring it and a bundle written
// last week stops importing.
func TestIndex_ManifestDoesNotLocateOptions(t *testing.T) {
	t.Run("export never writes it", func(t *testing.T) {
		data, err := MarshalIndex(&Index{
			Name:     "Corpus",
			Manifest: &Manifest{Types: map[string]string{"task": "types/t.anyblock.json"}, Properties: PropertiesFileName},
		})
		require.NoError(t, err)
		assert.NotContains(t, string(data), `"options"`)
		assert.Contains(t, string(data), `"types"`, "the lookup a reader DOES have stays")
	})

	// A bundle written before v0.46 is REFUSED rather than quietly ignored,
	// which is the opposite of the accept-and-drop rule stale PROPERTIES get
	// (§3). The manifest is closed — `additionalProperties: false` — and the
	// closure is the point: a manifest names where things are, so a member
	// the reader does not understand is a claim it cannot honour. Silently
	// ignoring it would leave an author believing their options are located.
	// Nothing has shipped on v0.45, so nothing is stranded.
	t.Run("a bundle that still carries it is refused, not silently misread", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version":2,"manifest":{
			"options":{"bafyopt1":"relationsOptions/bafyopt1.anyblock.json"}}}`))
		require.Error(t, err, "the manifest is closed (additionalProperties: false)")
		assert.Contains(t, err.Error(), "options")
	})
}

// The manifest binds file blobs (v0.47): file object id → the blob's
// archive-relative path, keys VERBATIM — an id is its own spelling, so
// unlike `types` there is nothing to re-key on either side. The map is the
// `source`-clobber's replacement: the legacy exporter stuffed the blob path
// into the real, editable Source relation on the document itself, and the
// destruction round-tripped through import. Here the document carries no
// path at all and the binding lives where every by-id lookup lives.
//
// How this can fail: drop Files from the Manifest struct or MarshalIndex
// (the binding vanishes on the way round and every blob is orphaned); stop
// sorting the map (the byte check goes red); re-key the ids through the
// type-spelling chain (an id that happens to fold onto a bundled key gets
// rewritten and the entry dangles); or forget empty() (a files-only
// manifest is dropped as "empty" and ships nothing).
func TestIndex_ManifestBindsFileBlobs(t *testing.T) {
	// given — two entries, deliberately out of sorted order
	in := &Index{
		Name: "Corpus",
		Manifest: &Manifest{
			Files: map[string]string{
				"bafyreigp3him": "files/bafyreigp3him.png",
				"bafyreiaaaaaa": "files/bafyreiaaaaaa.pdf",
			},
		},
	}

	// when
	data, err := MarshalIndex(in)
	require.NoError(t, err)
	got, err := UnmarshalIndex(data)
	require.NoError(t, err)
	data2, err := MarshalIndex(got)
	require.NoError(t, err)

	// then — byte-stable, keys verbatim, sorted; and a files-only manifest
	// is NOT empty (empty() must know the third member)
	assert.Equal(t, string(data), string(data2), "Marshal ∘ Unmarshal must be byte-stable")
	require.NotNil(t, got.Manifest)
	assert.Equal(t, in.Manifest.Files, got.Manifest.Files)
	assert.Less(t, strings.Index(string(data), "bafyreiaaaaaa"), strings.Index(string(data), "bafyreigp3him"),
		"the canonical form sorts the map's keys (§4)")

	t.Run("a non-string or empty blob path is refused", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version":2,"manifest":{"files":{"bafyx":42}}}`))
		require.Error(t, err)
		_, err = UnmarshalIndex([]byte(`{"version":2,"manifest":{"files":{"bafyx":""}}}`))
		require.Error(t, err)
	})
}

// An empty path VALUE in a manifest table is omitted on the way out, like
// every empty member (§4): writing it produced bytes the index's own
// Unmarshal refuses (minLength on every manifest path) — I1 broken from
// the Go API, reachable by any caller that left a map value blank.
func TestIndex_ManifestOmitsEmptyPaths(t *testing.T) {
	data, err := MarshalIndex(&Index{
		Name: "Corpus",
		Manifest: &Manifest{
			Types: map[string]string{"task": "types/t.anyblock.json", "ghost": ""},
			Files: map[string]string{"bafyx": ""},
		},
	})
	require.NoError(t, err)
	_, err = UnmarshalIndex(data)
	require.NoError(t, err, "what Marshal writes, Unmarshal accepts (I1)")
	assert.NotContains(t, string(data), "ghost")
	assert.NotContains(t, string(data), "bafyx")
	assert.Contains(t, string(data), "Task")

	// a manifest whose every entry is empty collapses to no manifest at all
	bare, err := MarshalIndex(&Index{Name: "Corpus", Manifest: &Manifest{Files: map[string]string{"bafyx": ""}}})
	require.NoError(t, err)
	assert.NotContains(t, string(bare), "manifest")
}
