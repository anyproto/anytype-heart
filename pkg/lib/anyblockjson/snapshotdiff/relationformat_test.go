package snapshotdiff

// relationformat_test.go — the §2d lift moved relationFormat,
// relationFormatIncludeTime and relationFormatObjectTypes onto a relation
// document's envelope, and this comparator needed NO new rule for it. That
// was a design obligation, not luck, and it is pinned here: the envelope
// fields mirror stored presence exactly (false, [] and null all travel), and
// the target-type id↔key translation is an inverse (TypeResolver), so the
// details that go in are the details that come out — unlike §2a's
// recommended lists and §2b's icon/cover, which both needed suppression
// rules above.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// relTypeResolver is the storeresolver shape reduced to what §2d consults: a
// PropertyResolver carrying the TypeResolver capability.
type relTypeResolver struct{}

func (relTypeResolver) PropertyById(string) (anyblockjson.PropertyDefinition, bool) {
	return anyblockjson.PropertyDefinition{}, false
}

func (relTypeResolver) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	return "", false
}

func (relTypeResolver) TypeKeyById(id string) (string, bool) {
	if id == "typeid-page" {
		return "page", true
	}
	return "", false
}

func (relTypeResolver) TypeIdByKey(key string) (string, bool) {
	if key == "page" {
		return "typeid-page", true
	}
	return "", false
}

func relationSnap(details map[string]*types.Value) *model.SmartBlockSnapshotBase {
	details["id"] = text("relObjectId")
	details["name"] = text("Budget")
	return &model.SmartBlockSnapshotBase{
		Key:     "budget",
		Details: &types.Struct{Fields: details},
		Blocks: []*model.Block{{
			Id:      "relObjectId",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
	}
}

// A relation document round-trips with zero findings, resolver or no
// resolver — including the shapes real data carries that a lossy lift would
// have normalized away: a false include_time (8,412 production relations), a
// null one (80), an empty target list (8,903), and target ids beside a
// verbatim survivor.
//
// How this can fail: make the §2d export omit a present-and-empty value, or
// import write keys where ids were (drop the TypeIdByKey arm), and the
// matching row reports a changed or added detail — the noise class every
// suppression rule above exists to record, which §2d was designed not to
// produce.
func TestCompare_RelationDocumentRoundTripsClean(t *testing.T) {
	for name, tc := range map[string]struct {
		details map[string]*types.Value
		opts    anyblockjson.Options
	}{
		"false and empty, no resolver": {
			details: map[string]*types.Value{
				"relationFormat":            number(6),
				"relationFormatIncludeTime": {Kind: &types.Value_BoolValue{BoolValue: false}},
				"relationFormatObjectTypes": list(),
			},
		},
		"null include_time, no resolver": {
			details: map[string]*types.Value{
				"relationFormat":            number(4),
				"relationFormatIncludeTime": {Kind: &types.Value_NullValue{}},
			},
		},
		"target ids under the TypeResolver capability": {
			details: map[string]*types.Value{
				"relationFormat":            number(100),
				"relationFormatObjectTypes": list("typeid-page", "bafyreidangling"),
			},
			opts: anyblockjson.Options{ResolveProperties: relTypeResolver{}},
		},
		"map format": {
			details: map[string]*types.Value{"relationFormat": number(102)},
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given
			orig := relationSnap(tc.details)

			// when
			data, err := anyblockjson.Marshal(model.SmartBlockType_STRelation, orig, tc.opts)
			require.NoError(t, err)
			sbType, got, err := anyblockjson.Unmarshal(data, tc.opts)
			require.NoError(t, err)

			// then
			assert.Empty(t, Compare(orig, got, sbType, tc.opts),
				"the §2d lift is a spelling change: the same details go in and out")
		})
	}
}

// The comparator still SEES a §2d detail that really changes — the rows
// above are clean because the codec is faithful, not because the keys are
// ignored.
//
// How this can fail: add the three keys to a suppression list (the §2b
// shape) and this loss goes dark.
func TestCompare_ARelationFormatChangeStillReports(t *testing.T) {
	// given a format rewrite — the exact loss the §2d lift exists to prevent
	orig := relationSnap(map[string]*types.Value{"relationFormat": number(2)})
	got := relationSnap(map[string]*types.Value{"relationFormat": number(0)})

	// when
	found := Compare(orig, got, model.SmartBlockType_STRelation, anyblockjson.Options{})

	// then
	require.Len(t, found, 1)
	assert.Contains(t, found[0], "relationFormat")
}
