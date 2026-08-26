package compose

// plan_test.go pins the plan phase's whole value: a path is a pure
// per-document function of the id, fixed before the first emit task, with
// no collision machinery for the concurrent phase to disagree about
// (EXPORTER_DESIGN.md §1.1, §1.3).

import (
	"strings"
	"testing"

	"github.com/anyproto/anytype-heart/core/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Every kind lands in its design-§1.2 directory, named by its id verbatim
// plus the settled double extension; a file object additionally gets a blob
// path with the same stem, so document and blob sort adjacent.
//
// How this can fail: reintroduce the legacy store vocabulary (`relations`,
// `relationsOptions`) and the bundle's first impression breaks the format's
// own naming promise; derive the name from anything but the id and id→path
// stops being a pure function of a reference.
func TestBuildPlan_PathsAreAPureFunctionOfTheId(t *testing.T) {
	// given
	plan, err := BuildPlan("space1", []DocMeta{
		{Id: "bafypage", SbType: model.SmartBlockType_Page},
		{Id: "bafytype", SbType: model.SmartBlockType_STType},
		{Id: "bafytmpl", SbType: model.SmartBlockType_Template},
		{Id: "bafyrel", SbType: model.SmartBlockType_STRelation},
		{Id: "bafyopt", SbType: model.SmartBlockType_STRelationOption},
		{Id: "AAjEparticipant", SbType: model.SmartBlockType_Participant},
		{Id: "bafyfile", SbType: model.SmartBlockType_FileObject, FileExt: "png", FileMime: "image/png"},
		// the rare fail-closed widget document has no dedicated home
		{Id: "bafywidget", SbType: model.SmartBlockType_Widget},
	})
	require.NoError(t, err)

	// then
	want := map[string]string{
		"bafypage":        "objects/bafypage.anyblock.json",
		"bafytype":        "types/bafytype.anyblock.json",
		"bafytmpl":        "templates/bafytmpl.anyblock.json",
		"bafyrel":         "properties/bafyrel.anyblock.json",
		"bafyopt":         "options/bafyopt.anyblock.json",
		"AAjEparticipant": "participants/AAjEparticipant.anyblock.json",
		"bafyfile":        "files/bafyfile.anyblock.json",
		"bafywidget":      "objects/bafywidget.anyblock.json",
	}
	for id, path := range want {
		got, ok := plan.DocPath(id)
		require.True(t, ok, id)
		assert.Equal(t, path, got)
	}
	blob, ok := plan.BlobPath("bafyfile")
	require.True(t, ok)
	assert.Equal(t, "files/bafyfile.png", blob, "same stem as the document, real extension")
	_, ok = plan.BlobPath("bafypage")
	assert.False(t, ok, "only file objects plan a blob")
}

// The stored `fileExt` is dirty as a path component — measured on the
// corpus: 431 empty, 9 longer than 10 chars, dozens non-alphanumeric
// (`0-rc01`, `9-alpha`), 12 literally "json". The three-step rule: the
// extension when clean, the mime's conventional extension when not, `bin`
// when neither — and `anyblock.json` as a computed suffix is impossible by
// construction, so a blob can never impersonate a document.
//
// How this can fail: pass the extension through raw (a `9-alpha` blob name
// carries shrapnel and an empty one ends in a bare dot); consult the OS
// mime registry instead of the fixed table (the same space exports
// different bytes on different machines).
func TestBlobExtension_SanitizesTheMeasuredDirt(t *testing.T) {
	cases := []struct{ ext, mime, want string }{
		{"png", "", "png"},
		{".PNG", "", "png"},
		{"", "image/jpeg", "jpg"},
		{"", "application/octet-stream", "bin"},
		{"0-rc01", "application/zip", "zip"},
		{"9-alpha", "", "bin"},
		{"json", "", "json"}, // a JSON blob keeps its extension; the double doc extension is the discriminator
		{"averylongextension", "application/pdf", "pdf"},
		{"", "", "bin"},
	}
	for _, c := range cases {
		got := BlobExtension(c.ext, c.mime)
		assert.Equal(t, c.want, got, "ext=%q mime=%q", c.ext, c.mime)
		assert.False(t, strings.HasSuffix("x."+got, DocExtension), "a blob may never look like a document")
	}
}

// An id that cannot be a filename stem is refused up front — the
// containment guarantee. The corpus's two id populations can never trip
// this; a refusal means the store handed us something that is not an id.
func TestBuildPlan_RefusesAPathHostileId(t *testing.T) {
	for _, id := range []string{"", ".", "..", "a/b", `a\b`} {
		_, err := BuildPlan("space1", []DocMeta{{Id: id, SbType: model.SmartBlockType_Page}})
		assert.Error(t, err, "id %q", id)
	}
}

// A participant document's filename is its ENVELOPE id — the §9 fold of the
// store composite to the bare identity — never the store id: a reference
// carries the folded id, so only the folded stem keeps id→path a pure
// function of the reference, and the composite would claim a `_`-prefixed
// name in the platform's reserved namespace (§1). The foreign-space
// composite stays unfolded, exactly as Marshal keeps it as the envelope id
// — the plan and the envelope decline together.
//
// This was the native sweep's first real-data catch: the plan named the
// file by the store id while the document inside declared the folded one,
// and the very first exported space flagged its own member's document as
// stem_id_mismatch.
func TestBuildPlan_ParticipantStemIsTheFoldedIdentity(t *testing.T) {
	// a real, checksum-valid identity (the fold classifies by decoding it)
	const identity = "AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"
	const spaceId = "bafyreispace.lav62qbhdcf9"
	own := domain.NewParticipantId(spaceId, identity)
	foreign := domain.NewParticipantId("bafyreiother.zzz", identity)

	plan, err := BuildPlan(spaceId, []DocMeta{
		{Id: own, SbType: model.SmartBlockType_Participant},
		{Id: foreign, SbType: model.SmartBlockType_Participant},
	})
	require.NoError(t, err)

	got, ok := plan.DocPath(own)
	require.True(t, ok, "the plan stays keyed by the STORE id the emit loop holds")
	assert.Equal(t, "participants/"+identity+".anyblock.json", got)

	got, ok = plan.DocPath(foreign)
	require.True(t, ok)
	assert.Equal(t, "participants/"+foreign+".anyblock.json", got,
		"a foreign-space composite stays unfolded, like its envelope id")
}
