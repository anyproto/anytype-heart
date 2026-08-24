package snapshotdiff

// omittedrelation_test.go pins the comparator's side of the §2f omission: a
// bundled-identical relation document travels as an `installed` key, and
// what comes back is the reader's reconstruction from the bundled table.
// The two skips that trip needs — install artifacts absent, definition
// defaults stamped — are scoped to snapshots the omission predicate itself
// admits, so the ordinary document round trip keeps its full sensitivity.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// omittableCopy is a field-identical installed copy of the bundled dueDate,
// carrying the install provenance a real copy does.
func omittableCopy(t *testing.T) *model.SmartBlockSnapshotBase {
	t.Helper()
	det, ok := anyblockjson.InstalledRelationDetails("dueDate", anyblockjson.Options{})
	require.True(t, ok)
	det.Fields["createdDate"] = &types.Value{Kind: &types.Value_NumberValue{NumberValue: 1700000000}}
	det.Fields["origin"] = &types.Value{Kind: &types.Value_NumberValue{NumberValue: 2}}
	det.Fields["apiObjectKey"] = &types.Value{Kind: &types.Value_StringValue{StringValue: "due_date"}}
	return &model.SmartBlockSnapshotBase{Details: det}
}

// reconstruction is what the reader builds from the `installed` key: the
// bundled table's facts and nothing of the install.
func reconstruction(t *testing.T) *model.SmartBlockSnapshotBase {
	t.Helper()
	det, ok := anyblockjson.InstalledRelationDetails("dueDate", anyblockjson.Options{})
	require.True(t, ok)
	return &model.SmartBlockSnapshotBase{Details: det}
}

// Across the omission trip the install artifacts come back absent —
// re-stamped by the next install — and that is normalization, not loss.
//
// How this can fail: remove the RelationInstallArtifactKey skip from
// Compare's orig-key loop, and createdDate/origin/apiObjectKey all report
// as changed-to-absent.
func TestCompare_OmittedRelationArtifactsComeBackAbsent(t *testing.T) {
	diffs := Compare(omittableCopy(t), reconstruction(t), model.SmartBlockType_STRelation, anyblockjson.Options{})
	assert.Empty(t, diffs)
}

// The reconstruction states the WHOLE definition, so a member the copy
// never stored arrives as its explicit empty default. Absent and empty say
// the same thing for a definition member with a defined default; a
// NON-empty invented member still reports.
//
// How this can fail: remove the InstallStampedDefault skip from the
// added-details loop (the stamped empty default reports as added), or widen
// it past empty values (the invented-name case goes green and a
// reconstruction bug ships as normalization).
func TestCompare_OmittedRelationStampedDefaults(t *testing.T) {
	t.Run("a stamped empty default is not an addition", func(t *testing.T) {
		orig := omittableCopy(t)
		delete(orig.Details.Fields, "isHidden") // the copy never stored it
		delete(orig.Details.Fields, "relationFormatObjectTypes")
		diffs := Compare(orig, reconstruction(t), model.SmartBlockType_STRelation, anyblockjson.Options{})
		assert.Empty(t, diffs)
	})
	t.Run("an invented non-empty member still reports", func(t *testing.T) {
		orig := omittableCopy(t)
		delete(orig.Details.Fields, "description")
		got := reconstruction(t)
		got.Details.Fields["description"] = &types.Value{Kind: &types.Value_StringValue{StringValue: "invented"}}
		diffs := Compare(orig, got, model.SmartBlockType_STRelation, anyblockjson.Options{})
		assert.NotEmpty(t, diffs)
	})
}

// Both skips are SCOPED to snapshots the omission predicate admits: on a
// divergent copy — one whose document is kept, so every key must survive —
// a missing install artifact is still loss.
//
// How this can fail: drop the `omittable` guard from either skip, and the
// comparator stops seeing real artifact-key loss on every kept relation
// document in the corpus.
func TestCompare_KeptRelationDocumentKeepsFullSensitivity(t *testing.T) {
	orig := omittableCopy(t)
	orig.Details.Fields["name"] = &types.Value{Kind: &types.Value_StringValue{StringValue: "End Date"}} // divergent: kept
	got := reconstruction(t)
	got.Details.Fields["name"] = &types.Value{Kind: &types.Value_StringValue{StringValue: "End Date"}}
	// createdDate/origin/apiObjectKey are in orig and not in got
	diffs := Compare(orig, got, model.SmartBlockType_STRelation, anyblockjson.Options{})
	assert.NotEmpty(t, diffs, "on a kept document a missing artifact key is loss, not normalization")
}

// The artifact skip must never swallow a DEFINITION member: an omittable
// original whose name goes missing on the way back is loss, whatever else
// the trip may drop.
//
// How this can fail: add a definition key (name, relationFormat, …) to
// relationInstallArtifactKeys — this is the admission test running in
// reverse, the §2a discipline.
func TestCompare_OmittedRelationDefinitionLossStillReports(t *testing.T) {
	got := reconstruction(t)
	delete(got.Details.Fields, "name")
	diffs := Compare(omittableCopy(t), got, model.SmartBlockType_STRelation, anyblockjson.Options{})
	assert.NotEmpty(t, diffs)
}

// relation_settings' object_types round-trips by type KEY, and legacy data
// mixes spellings: 27 corpus relations store a bare type key where the
// store speaks object ids, and import writes the id back — the SAME type,
// respelled. The comparator normalizes both sides to keys through the
// TypeResolver, exactly as it does for the recommended lists one namespace
// over, so a respelling is silent and a REBINDING still reports.
//
// How this can fail: drop the relationTargetsDetailKey arm from detailEqual
// (the respelled case reports as loss — the 27 false findings come back),
// or normalize one side only (the rebound case goes green and a genuine
// target substitution ships as normalization).
func TestCompare_RelationTargetsCompareByTypeKey(t *testing.T) {
	tr := &targetsResolver{idToKey: map[string]string{"bafyderivedgoal": "goal", "bafyderivedtask": "task"}}
	opts := anyblockjson.Options{ResolveProperties: tr}
	targets := func(entries ...string) *model.SmartBlockSnapshotBase {
		vals := make([]*types.Value, 0, len(entries))
		for _, e := range entries {
			vals = append(vals, &types.Value{Kind: &types.Value_StringValue{StringValue: e}})
		}
		return &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
			"relationFormatObjectTypes": {Kind: &types.Value_ListValue{
				ListValue: &types.ListValue{Values: vals}}},
		}}}
	}

	t.Run("a respelled target is the same type", func(t *testing.T) {
		diffs := Compare(targets("goal"), targets("bafyderivedgoal"), model.SmartBlockType_STRelation, opts)
		assert.Empty(t, diffs)
	})
	t.Run("a rebound target still reports", func(t *testing.T) {
		diffs := Compare(targets("goal"), targets("bafyderivedtask"), model.SmartBlockType_STRelation, opts)
		assert.NotEmpty(t, diffs)
	})
	t.Run("without the capability the raw comparison stands", func(t *testing.T) {
		// §2d: verbatim both ways without a resolver, so equal stays equal
		diffs := Compare(targets("goal"), targets("goal"), model.SmartBlockType_STRelation, anyblockjson.Options{})
		assert.Empty(t, diffs)
	})
}

// targetsResolver is the TypeResolver capability over a fixed id table.
type targetsResolver struct{ idToKey map[string]string }

func (r *targetsResolver) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	return anyblockjson.PropertyDefinition{}, false
}
func (r *targetsResolver) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	return "", false
}
func (r *targetsResolver) TypeKeyById(id string) (string, bool) {
	k, ok := r.idToKey[id]
	return k, ok
}
func (r *targetsResolver) TypeIdByKey(key string) (string, bool) {
	for id, k := range r.idToKey {
		if k == key {
			return id, true
		}
	}
	return "", false
}
