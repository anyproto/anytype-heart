package anyblockjson

// fold_test.go — the participant fold (§9): `_participant_<space>_<identity>`
// exports as the bare identity when Options.SpaceId names the space, and a
// bare identity imports back as this space's participant id.

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The real shapes, from the account that produced the corpus: a checksummed
// 48-character identity and the 135-character composite built from it.
const (
	foldSpaceId   = "bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq.30afw2fe3tvff"
	foldIdentity  = "AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"
	foldComposite = "_participant_bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq_30afw2fe3tvff_" + foldIdentity

	// the same member seen from another space: NOT this run's, so it must
	// pass through whole
	foreignComposite = "_participant_bafyreiforeignspaceaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_2xyz_" + foldIdentity
)

// foldSnapshot puts a participant reference in every §9 slot the census
// found them in: object-format property values, items, a block object_id, a
// filter value, an object order — plus a foreign-space composite as the
// control.
func foldSnapshot() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{
				Id:          "bafyreifoldroot",
				ChildrenIds: []string{"lnk", "dv1"},
				Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
			},
			{Id: "lnk", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: foldComposite,
			}}},
			{Id: "dv1", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{
					Id: "view1", Name: "All",
					Filters: []*model.BlockContentDataviewFilter{{
						Id:          "f1",
						RelationKey: "assignee",
						Condition:   model.BlockContentDataviewFilter_In,
						Value:       strList(foldComposite),
					}},
				}},
				ObjectOrders: []*model.BlockContentDataviewObjectOrder{{
					ViewId:    "view1",
					ObjectIds: []string{foldComposite},
				}},
			}}},
		},
		Details: fields(map[string]*types.Value{
			"id":       str("bafyreifoldroot"),
			"name":     str("Fold host"),
			"assignee": strList(foldComposite),
			"owner":    strList(foldComposite, foreignComposite),
		}),
		Collections: fields(map[string]*types.Value{
			storeKeyItems: strList(foldComposite),
		}),
	}
}

// foldOptions resolves the space-minted `owner` key to the objects format —
// the corpus's heaviest participant slot is exactly such a custom property —
// and carries the space id that arms the fold.
func foldOptions() Options {
	o := refOptions()
	prev := o.ResolveFormat
	o.ResolveFormat = func(key domain.RelationKey) (model.RelationFormat, bool) {
		if key == "owner" {
			return model.RelationFormat_object, true
		}
		return prev(key)
	}
	o.SpaceId = foldSpaceId
	return o
}

// Every slot folds, the foreign-space composite does not, and the output
// still validates (I1). The trigger is the VALUE's shape, never the property
// name: `owner` is a space-minted custom property the bundle knows nothing
// about.
//
// How this can fail: unhook the fold from a slot and the 135-character
// composite shows up there; fold on the property name instead of the value
// and the custom `owner` slot keeps the composite; drop the same-space gate
// and the foreign composite folds too (silent re-homing on import).
func TestFold_ParticipantRefsFoldOnEverySlot(t *testing.T) {
	// when
	data, err := Marshal(model.SmartBlockType_Page, foldSnapshot(), foldOptions())
	require.NoError(t, err)
	require.NoError(t, Validate(data), "Marshal never emits what Validate rejects (§11 I1)")
	doc := string(data)

	// then
	assert.NotContains(t, doc, foldComposite, "no slot keeps this space's composite id")
	assert.Contains(t, doc, `"`+foldIdentity+`"`, "the bare identity stands in")
	assert.Contains(t, doc, foreignComposite, "a foreign space's composite passes through whole")
}

// The participant document's own envelope id folds too — a reader must be
// able to textually join a folded reference to the participant document it
// points at (§9) — and import rebuilds the composite as the object id.
//
// How this can fail: skip the fold on the `id` slot and the envelope keeps
// 135 characters; skip the unfold and the imported snapshot's id detail (and
// root block id) hold a bare identity where every store write expects the
// composite — the silent corruption Options.SpaceId exists to prevent.
func TestFold_ParticipantOwnEnvelopeId(t *testing.T) {
	// given a participant document
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{
			Id:      foldComposite,
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
		Details: fields(map[string]*types.Value{
			"id":   str(foldComposite),
			"name": str("Roma Kha"),
		}),
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, foldOptions())
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"id": "`+foldIdentity+`"`)
	assert.NotContains(t, string(data), foldComposite)

	// and back
	_, imported, err := Unmarshal(data, foldOptions())
	require.NoError(t, err)
	assert.Equal(t, foldComposite,
		imported.GetDetails().GetFields()["id"].GetStringValue(),
		"import rebuilds the composite as the object id")
	assert.Equal(t, foldComposite, imported.Blocks[0].Id,
		"and as the root block id")
}

// With no SpaceId the fold is OFF in both directions: the composite passes
// through export whole, and a bare identity is left alone on import rather
// than guessed into some space.
//
// How this can fail: fold on export regardless of SpaceId and the first
// assertion finds the identity; unfold against an empty space and the second
// case builds `_participant__<identity>` garbage.
func TestFold_DisabledWithoutSpaceId(t *testing.T) {
	// given
	opts := foldOptions()
	opts.SpaceId = ""

	// when
	data, err := Marshal(model.SmartBlockType_Page, foldSnapshot(), opts)
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), foldComposite, "the composite survives whole")
	assert.NotContains(t, string(data), `"`+foldIdentity+`"`)

	// and a bare identity on import stays bare
	doc := `{"version": 1, "properties": {"assignee": ["` + foldIdentity + `"]}}`
	_, snap, err := Unmarshal([]byte(doc), opts)
	require.NoError(t, err)
	assert.Equal(t, []string{foldIdentity},
		valueStringList(snap.GetDetails().GetFields()["assignee"]))
}

// Import rebuilds the composite from a bare identity — with or without the
// informative suffix — and leaves near-misses alone: the checksum is the
// classifier, so a 48-character base58 string that is not an identity does
// not unfold.
//
// How this can fail: skip the unfold and the bare identity lands in the
// snapshot (the corruption this change exists to fix); unfold by length or
// charset alone and the corrupted-checksum control rebuilds a participant id
// for a string that names nobody.
func TestFold_ImportRebuildsTheComposite(t *testing.T) {
	// a plausible-looking non-identity: same charset and length, bad checksum
	notAnIdentity := foldIdentity[:len(foldIdentity)-4] + "aaaa"

	// given
	doc := `{"version": 1, "properties": {
		"assignee": ["` + foldIdentity + `#roma_kha", "` + notAnIdentity + `"]}}`

	// when
	_, snap, err := Unmarshal([]byte(doc), foldOptions())

	// then
	require.NoError(t, err)
	assert.Equal(t, []string{foldComposite, notAnIdentity},
		valueStringList(snap.GetDetails().GetFields()["assignee"]),
		"the identity unfolds (suffix trimmed first); the near-miss passes verbatim")
}

// The round trip is byte-stable and snapshot-lossless: fold on export,
// rebuild on import, fold again identically.
//
// How this can fail: any asymmetry between the two halves — a slot that
// folds but does not unfold (the details stop matching), or unfolds into a
// different spelling (the bytes stop matching).
func TestFold_RoundTripLossless(t *testing.T) {
	// given
	opts := foldOptions()
	snap := foldSnapshot()

	// when
	first, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	sbType, imported, err := Unmarshal(first, opts)
	require.NoError(t, err)
	second, err := Marshal(sbType, imported, opts)
	require.NoError(t, err)

	// then
	assert.Equal(t, string(first), string(second), "byte-stable (§11)")
	for _, key := range []string{"assignee", "owner"} {
		assert.Equal(t,
			valueStringList(snap.Details.Fields[key]),
			valueStringList(imported.GetDetails().GetFields()[key]),
			"detail %q holds the original composites again", key)
	}
	assert.Equal(t,
		valueStringList(snap.Collections.Fields[storeKeyItems]),
		valueStringList(imported.GetCollections().GetFields()[storeKeyItems]),
		"items too")
}

// Fold and suffix compose: with RefNames on and a resolver that knows the
// COMPOSITE id (the id the space indexes), the document spells
// `<identity>#<name>`.
//
// How this can fail: ask the resolver about the folded identity instead of
// the stored composite and no name resolves, so the suffix vanishes.
func TestFold_ComposesWithTheNameSuffix(t *testing.T) {
	// given
	opts := foldOptions()
	opts.RefNames = true
	opts.ResolveObjectNames = testObjectNames{foldComposite: "Roma Kha"}

	// when
	data, err := Marshal(model.SmartBlockType_Page, foldSnapshot(), opts)
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"`+foldIdentity+`#roma_kha"`,
		"resolvable AND readable: the folded identity plus the informative name")
	assert.True(t, strings.Contains(string(data), foldIdentity))
}
