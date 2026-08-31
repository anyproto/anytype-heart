package compose

// compose_test.go pins the composer against the §2c/§2f composition it
// re-homes from the roundtrip harness's spaceComposer: the lift-before-omit
// discipline, the used-only dictionary, the manifest's three tables, and the
// bundle-level I1 re-read. The corpus sweep exercises the same code end to
// end over 38k real documents; these tests pin the mechanism on a space
// small enough to read.

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func strVal(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}
func numVal(n float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
}
func boolVal(b bool) *types.Value { return &types.Value{Kind: &types.Value_BoolValue{BoolValue: b}} }

func detFields(det map[string]*types.Value) *types.Struct {
	return &types.Struct{Fields: det}
}

// testSpaceSnapshot is a space document index.json fully states — the
// omission's happy case (§2c).
func testSpaceSnapshot() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "bafyreispace",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: detFields(map[string]*types.Value{
			"id": strVal("bafyreispace"), "name": strVal("Corpus"),
			"homepage": strVal("bafyreihome"), "layout": numVal(9), "resolvedLayout": numVal(10),
			"isHidden": boolVal(true),
		}),
	}
}

// testInstalledCopy is a field-identical installed copy of a bundled
// relation — the omit-into-`installed` case (§2f), install provenance and
// all.
func testInstalledCopy(t *testing.T, key string) *model.SmartBlockSnapshotBase {
	t.Helper()
	det, ok := anyblockjson.InstalledRelationDetails(key, anyblockjson.Options{})
	require.True(t, ok)
	det.Fields["createdDate"] = numVal(1700000000)
	det.Fields["origin"] = numVal(2)
	det.Fields["sourceObject"] = strVal("_br" + key)
	det.Fields["layout"] = numVal(float64(model.ObjectType_relation))
	return &model.SmartBlockSnapshotBase{Details: det}
}

// One small space, end to end: the two omitted documents lift into the
// bundle files, the written ones feed the manifest, the option document's
// vocabulary lands inline on the property that owns it, and both files
// re-read through the package's own Unmarshal (I1 at bundle scope).
//
// How this can fail: record the omission before the lift (the space's name
// vanishes with its document); build the dictionary from ALL keys instead
// of used ones (§2f's used-only rule breaks); key the manifest by the
// document spelling instead of the stored key; or skip the re-read and ship
// a bundle the package itself refuses — found at restore time instead of
// here.
func TestComposer_ComposesTheBundleFiles(t *testing.T) {
	// given
	c := NewComposer(anyblockjson.Options{}, "Fallback name")

	omitted, issues := c.Observe(model.SmartBlockType_Workspace, testSpaceSnapshot())
	require.True(t, omitted, "index.json states everything the space document holds")
	require.Empty(t, issues)

	omitted, issues = c.Observe(model.SmartBlockType_STRelation, testInstalledCopy(t, "dueDate"))
	require.True(t, omitted, "a field-identical installed copy travels as its key")
	require.Empty(t, issues)

	typeSnap := &model.SmartBlockSnapshotBase{Details: detFields(map[string]*types.Value{
		"id": strVal("bafytask"), "uniqueKey": strVal("ot-task"),
	})}
	omitted, _ = c.Observe(model.SmartBlockType_STType, typeSnap)
	require.False(t, omitted)
	require.NoError(t, c.ObserveWritten(model.SmartBlockType_STType, typeSnap,
		[]byte(`{"version":2}`), "types/bafytask.anyblock.json"))

	optSnap := &model.SmartBlockSnapshotBase{Details: detFields(map[string]*types.Value{
		"id": strVal("bafyurgent"), "relationKey": strVal("tag"),
		"name": strVal("urgent"), "relationOptionColor": strVal("red"),
		"uniqueKey": strVal("opt-abcd1234"),
	})}
	omitted, _ = c.Observe(model.SmartBlockType_STRelationOption, optSnap)
	require.False(t, omitted)
	require.NoError(t, c.ObserveWritten(model.SmartBlockType_STRelationOption, optSnap,
		[]byte(`{"version":2}`), "options/bafyurgent.anyblock.json"))

	pageSnap := &model.SmartBlockSnapshotBase{Details: detFields(map[string]*types.Value{
		"id": strVal("bafypage"),
	})}
	pageDoc := []byte(`{"version":2,"properties":{"due_date":"2026-01-01","tag":["urgent"]}}`)
	omitted, _ = c.Observe(model.SmartBlockType_Page, pageSnap)
	require.False(t, omitted)
	require.NoError(t, c.ObserveWritten(model.SmartBlockType_Page, pageSnap,
		pageDoc, "objects/bafypage.anyblock.json"))

	c.ObserveFileBlob("bafyfile", "files/bafyfile.png")

	// when
	indexData, dictData, stats, err := c.Finish()
	require.NoError(t, err)

	// then — the index carries the lift and the manifest's three tables
	idx, err := anyblockjson.UnmarshalIndex(indexData)
	require.NoError(t, err)
	assert.Equal(t, "Corpus", idx.Name, "the space document's own name wins over the fallback")
	assert.Equal(t, "bafyreihome", idx.Homepage)
	require.NotNil(t, idx.Manifest)
	assert.Equal(t, map[string]string{"task": "types/bafytask.anyblock.json"}, idx.Manifest.Types)
	assert.Equal(t, map[string]string{"bafyfile": "files/bafyfile.png"}, idx.Manifest.Files)
	assert.Equal(t, anyblockjson.PropertiesFileName, idx.Manifest.Properties)

	// the dictionary: the installed key, and one entry per USED key — with
	// the minted vocabulary inline on the property that owns it
	dict, err := anyblockjson.UnmarshalPropertyDictionary(dictData)
	require.NoError(t, err)
	assert.Equal(t, []string{"dueDate"}, dict.Installed)
	byKey := map[string]anyblockjson.PropertyDefinition{}
	for _, def := range dict.Properties {
		byKey[string(def.Key)] = def
	}
	require.Contains(t, byKey, "tag")
	require.Len(t, byKey["tag"].Options, 1)
	assert.Equal(t, "urgent", byKey["tag"].Options[0].Name)
	assert.Equal(t, "red", byKey["tag"].Options[0].Color)
	assert.Equal(t, "abcd1234", byKey["tag"].Options[0].InternalKey)

	assert.Equal(t, 1, stats.DictionaryInstalled)
	assert.Equal(t, 1, stats.ManifestTypes)
	assert.Equal(t, 1, stats.ManifestFiles)
	assert.Equal(t, 1, stats.OptionDocs)
	assert.Equal(t, 2, stats.OmittedDocs)
	assert.Empty(t, stats.OrphanUsedKeys)
}

// The emit phase is concurrent and unordered; the composer's aggregates are
// commutative and Finish sorts everything it writes — so observation ORDER
// must never reach the bytes. This is the §1.5 determinism claim, proved on
// the aggregate rather than asserted in a comment.
//
// How this can fail: accumulate anything order-sensitive (first-writer-wins
// naming, an append the finish does not sort) and the reversed run produces
// different bytes.
func TestComposer_ObservationOrderNeverReachesTheBytes(t *testing.T) {
	type obs struct {
		sbType model.SmartBlockType
		base   *model.SmartBlockSnapshotBase
		doc    []byte
		path   string
	}
	build := func(t *testing.T) []obs {
		return []obs{
			{model.SmartBlockType_Workspace, testSpaceSnapshot(), nil, ""},
			{model.SmartBlockType_STRelation, testInstalledCopy(t, "dueDate"), nil, ""},
			{model.SmartBlockType_STRelation, testInstalledCopy(t, "assignee"), nil, ""},
			{model.SmartBlockType_STType, &model.SmartBlockSnapshotBase{Details: detFields(map[string]*types.Value{
				"id": strVal("bafytask"), "uniqueKey": strVal("ot-task"),
			})}, []byte(`{"version":2}`), "types/bafytask.anyblock.json"},
			{model.SmartBlockType_Page, &model.SmartBlockSnapshotBase{Details: detFields(map[string]*types.Value{
				"id": strVal("bafypage"),
			})}, []byte(`{"version":2,"properties":{"due_date":"2026-01-01"}}`), "objects/bafypage.anyblock.json"},
		}
	}
	run := func(t *testing.T, seq []obs) (string, string) {
		c := NewComposer(anyblockjson.Options{}, "Corpus")
		for _, o := range seq {
			omitted, _ := c.Observe(o.sbType, o.base)
			if !omitted && o.doc != nil {
				require.NoError(t, c.ObserveWritten(o.sbType, o.base, o.doc, o.path))
			}
		}
		c.ObserveFileBlob("bafyfile", "files/bafyfile.png")
		index, dict, _, err := c.Finish()
		require.NoError(t, err)
		return string(index), string(dict)
	}

	fwd := build(t)
	rev := build(t)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}

	i1, d1 := run(t, fwd)
	i2, d2 := run(t, rev)
	assert.Equal(t, i1, i2, "index bytes must not depend on observation order")
	assert.Equal(t, d1, d2, "dictionary bytes must not depend on observation order")
}

// An empty composition states nothing: no written document, no bundle
// files — the harness's own rule for a space whose dump produced nothing.
func TestComposer_NothingWrittenNothingStated(t *testing.T) {
	c := NewComposer(anyblockjson.Options{}, "Corpus")
	index, dict, stats, err := c.Finish()
	require.NoError(t, err)
	assert.Nil(t, index)
	assert.Nil(t, dict)
	assert.Zero(t, stats.DictionaryEntries)
}

// Two options of one property may share a NAME — real accounts hold such
// pairs — and (order, name) alone is then not a total order: the tie used
// to fall back to insertion order, which under the concurrent emit is
// scheduling order. The corpus sweep caught it as two exports of one space
// disagreeing about which colour sat at which vocabulary position. The
// option document's own id is the tie-break, because it is the one member
// that cannot tie.
//
// How this can fail: drop the id from the sort key (the reversed run puts
// the twins in arrival order and the bytes differ); or dedupe by name
// instead of ordering (one of two real options silently vanishes from the
// vocabulary).
func TestComposer_SameNamedOptionsHaveATotalOrder(t *testing.T) {
	optSnap := func(id, color string) *model.SmartBlockSnapshotBase {
		return &model.SmartBlockSnapshotBase{Details: detFields(map[string]*types.Value{
			"id": strVal(id), "relationKey": strVal("tag"),
			"name": strVal("urgent"), "relationOptionColor": strVal(color),
		})}
	}
	run := func(t *testing.T, reversed bool) string {
		c := NewComposer(anyblockjson.Options{}, "Corpus")
		twins := []*model.SmartBlockSnapshotBase{optSnap("bafyaaa", "teal"), optSnap("bafyzzz", "purple")}
		if reversed {
			twins[0], twins[1] = twins[1], twins[0]
		}
		for _, snap := range twins {
			omitted, _ := c.Observe(model.SmartBlockType_STRelationOption, snap)
			require.False(t, omitted)
			require.NoError(t, c.ObserveWritten(model.SmartBlockType_STRelationOption, snap,
				[]byte(`{"version":2}`), "options/"+snap.Details.Fields["id"].GetStringValue()+".anyblock.json"))
		}
		pageSnap := &model.SmartBlockSnapshotBase{Details: detFields(map[string]*types.Value{"id": strVal("bafypage")})}
		require.NoError(t, c.ObserveWritten(model.SmartBlockType_Page, pageSnap,
			[]byte(`{"version":2,"properties":{"tag":["urgent"]}}`), "objects/bafypage.anyblock.json"))
		_, dict, _, err := c.Finish()
		require.NoError(t, err)
		return string(dict)
	}

	fwd := run(t, false)
	rev := run(t, true)
	assert.Equal(t, fwd, rev, "vocabulary bytes must not depend on observation order")
	assert.Contains(t, fwd, "teal")
	assert.Contains(t, fwd, "purple", "both real options stay; ordering, not deduping")
	assert.Less(t, strings.Index(fwd, "teal"), strings.Index(fwd, "purple"),
		"the id tie-break is ascending: bafyaaa's colour sits first")
}
