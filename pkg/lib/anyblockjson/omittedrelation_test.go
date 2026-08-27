package anyblockjson

// omittedrelation_test.go pins the §2f omission rule: which relation
// documents a bundle composition may leave out, and the fail-closed
// discipline that keeps every other one. A predicate that omits a document
// carrying real data deletes that data silently — the disqualifying failure
// for a backup format — so every widening here has to be red first.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
)

// installedCopySnapshot builds the snapshot of a field-identical installed
// copy of a bundled relation: the reconstruction's own details plus the
// install provenance a real copy carries.
func installedCopySnapshot(t *testing.T, key string, opts Options) *model.SmartBlockSnapshotBase {
	t.Helper()
	det, ok := InstalledRelationDetails(key, opts)
	require.True(t, ok)
	det.Fields["createdDate"] = &types.Value{Kind: &types.Value_NumberValue{NumberValue: 1700000000}}
	det.Fields["origin"] = &types.Value{Kind: &types.Value_NumberValue{NumberValue: 2}}
	det.Fields["sourceObject"] = &types.Value{Kind: &types.Value_StringValue{StringValue: "_br" + key}}
	det.Fields["layout"] = &types.Value{Kind: &types.Value_NumberValue{NumberValue: float64(model.ObjectType_relation)}}
	return &model.SmartBlockSnapshotBase{Details: det}
}

// A field-identical installed copy is omitted, install provenance
// notwithstanding: the artifact keys may hold ANY value, because the next
// install re-stamps them (§2f). And the reconstruction the reader builds
// from the `installed` key states the table's own facts.
//
// How this can fail: drop an artifact key (createdDate, origin, …) from
// relationInstallArtifactKeys — the copy stops being omittable and the
// first assertion goes red; or make InstalledRelationDetails restate the
// key instead of the table's name, and the anchor assertion catches the
// reconstruction drifting from the table.
func TestOmittedBundledRelation_IdenticalCopyOmits(t *testing.T) {
	// given
	base := installedCopySnapshot(t, "dueDate", Options{})

	// when
	key, omitted := OmittedBundledRelation(model.SmartBlockType_STRelation, base, Options{})

	// then
	require.True(t, omitted)
	assert.Equal(t, "dueDate", key)
	// the reconstruction anchors to the TABLE, not to the copy
	det, ok := InstalledRelationDetails("dueDate", Options{})
	require.True(t, ok)
	assert.Equal(t, "Due date", det.Fields["name"].GetStringValue())
	assert.Equal(t, float64(model.RelationFormat_date), det.Fields["relationFormat"].GetNumberValue())
	assert.Equal(t, float64(1), det.Fields["relationMaxCount"].GetNumberValue())
}

// Everything that must KEEP the document, case by case — the fail-closed
// half of the rule. Each case is one way real data could hide in a relation
// document, and each mutation that would lose it is named.
//
// How this can fail: add the unclassified key to the artifact map (its case
// goes red — that is the admission test running in reverse); compare a
// definition field against the copy instead of the table (the divergent
// cases go red); read an alien-kinded value through a coercing getter (the
// alien-kind case); or stop looking at blocks (the dataview case).
func TestOmittedBundledRelation_FailClosed(t *testing.T) {
	strVal := func(s string) *types.Value {
		return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
	}
	for name, mutate := range map[string]func(base *model.SmartBlockSnapshotBase){
		"a divergent name is the §2f rename case": func(base *model.SmartBlockSnapshotBase) {
			base.Details.Fields["name"] = strVal("End Date")
		},
		"a divergent isHidden is the 132-document case": func(base *model.SmartBlockSnapshotBase) {
			base.Details.Fields["isHidden"] = &types.Value{Kind: &types.Value_BoolValue{BoolValue: true}}
		},
		"an unclassified key is real data": func(base *model.SmartBlockSnapshotBase) {
			base.Details.Fields["somethingNobodyVetted"] = strVal("x")
		},
		"isUninstalled is user intent, not an artifact": func(base *model.SmartBlockSnapshotBase) {
			base.Details.Fields["isUninstalled"] = &types.Value{Kind: &types.Value_BoolValue{BoolValue: true}}
		},
		"an alien-kinded value never coerces to a match": func(base *model.SmartBlockSnapshotBase) {
			// GetBoolValue would read this as false == the table's false
			base.Details.Fields["isHidden"] = strVal("false")
		},
		"a stored null include_time is presence §2d carries": func(base *model.SmartBlockSnapshotBase) {
			base.Details.Fields["relationFormatIncludeTime"] = &types.Value{Kind: &types.Value_NullValue{}}
		},
		"a dataview block is content only a document carries": func(base *model.SmartBlockSnapshotBase) {
			base.Blocks = []*model.Block{{Id: "dv", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{}}}}
		},
		"free text on the page is content too": func(base *model.SmartBlockSnapshotBase) {
			base.Blocks = []*model.Block{{Id: "t", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "notes", Style: model.BlockContentText_Paragraph}}}}
		},
		"no relationKey, no identity to match": func(base *model.SmartBlockSnapshotBase) {
			delete(base.Details.Fields, "relationKey")
		},
	} {
		t.Run(name, func(t *testing.T) {
			base := installedCopySnapshot(t, "dueDate", Options{})
			mutate(base)
			_, omitted := OmittedBundledRelation(model.SmartBlockType_STRelation, base, Options{})
			assert.False(t, omitted, "the document must be kept")
		})
	}
	t.Run("a non-relation kind is never omitted", func(t *testing.T) {
		base := installedCopySnapshot(t, "dueDate", Options{})
		_, omitted := OmittedBundledRelation(model.SmartBlockType_Page, base, Options{})
		assert.False(t, omitted)
	})
	t.Run("title and description scaffolding does not keep the document", func(t *testing.T) {
		base := installedCopySnapshot(t, "dueDate", Options{})
		base.Blocks = []*model.Block{
			{Id: "r", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "t", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text: "Due date", Style: model.BlockContentText_Title}}},
		}
		_, omitted := OmittedBundledRelation(model.SmartBlockType_STRelation, base, Options{})
		assert.True(t, omitted, "the editor regenerates the scaffolding; the format drops it as structural (§7)")
	})
}

// omittedTypeResolver is the TypeResolver capability over one derived id.
type omittedTypeResolver struct {
	capturingPropertyResolver
	idToKey map[string]string
}

func (r *omittedTypeResolver) TypeKeyById(id string) (string, bool) {
	k, ok := r.idToKey[id]
	return k, ok
}

func (r *omittedTypeResolver) TypeIdByKey(key string) (string, bool) {
	for id, k := range r.idToKey {
		if k == key {
			return id, true
		}
	}
	return "", false
}

// The store keeps target types as derived OBJECT ids (objectcreator rewrites
// bundled urls at creation), and only the TypeResolver capability can turn
// them back into the keys the bundled table speaks. With it, a copy whose
// targets are derived ids still matches; without it, the comparison runs
// verbatim and the copy is KEPT — fewer omissions, never a wrong one.
//
// How this can fail: drop the TypeResolver arm from installedTargetKeys
// (the with-resolver case stops matching), or "fix" the degradation by
// treating an untranslatable id as its key (the without-resolver case
// starts omitting on a match nobody proved).
func TestOmittedBundledRelation_TargetTypesTranslate(t *testing.T) {
	// given: `tasks` targets the task type; the copy stores a derived id
	rel, err := bundle.GetRelation(domain.RelationKey("tasks"))
	require.NoError(t, err)
	require.NotEmpty(t, rel.ObjectTypes)
	tr := &omittedTypeResolver{idToKey: map[string]string{"bafyderivedtask": "task"}}
	withResolver := Options{ResolveProperties: tr}

	base := installedCopySnapshot(t, "tasks", withResolver)
	require.Equal(t, "bafyderivedtask",
		base.Details.Fields["relationFormatObjectTypes"].GetListValue().Values[0].GetStringValue(),
		"the fixture stores the derived id, as a real space does")

	// when / then
	_, omitted := OmittedBundledRelation(model.SmartBlockType_STRelation, base, withResolver)
	assert.True(t, omitted, "the resolver inverts the id to the table's key")

	_, omitted = OmittedBundledRelation(model.SmartBlockType_STRelation, base, Options{})
	assert.False(t, omitted, "without the capability the id stays opaque and the document is kept")
}
