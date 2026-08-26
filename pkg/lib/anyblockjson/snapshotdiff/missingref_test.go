package snapshotdiff

// missingref_test.go — the missing-reference rule (§9) reaches this
// comparator in the SAME change that taught export: an objects/files value
// entry or an `object_types` entry naming an object the space does not hold
// is dropped by design, and the comparison applies the format's own
// predicate (DroppedMissingObjectRef) to both sides. Without that, every
// document the rule touches would report its dropped entries as data loss —
// the drift class that once produced 1,344 false failures in one sweep.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// testCid mints a REAL content id — the §9 shape gate only lets the
// existence question reach CID-shaped entries.
func testCid(seed string) string {
	sum, err := mh.Sum([]byte(seed), mh.SHA2_256, -1)
	if err != nil {
		panic(err)
	}
	return cid.NewCidV1(cid.DagCBOR, sum).String()
}

var (
	liveCid = testCid("live")
	deadCid = testCid("dead")
)

// existenceStore is the storeresolver shape reduced to the object-namespace
// pair: ids in the set exist, everything else does not.
type existenceStore map[string]bool

func (m existenceStore) ObjectName(string) (string, bool) { return "", false }

func (m existenceStore) ObjectExists(id string) (exists, known bool) {
	return m[id], true
}

func missingRefOpts() anyblockjson.Options {
	return anyblockjson.Options{
		ResolveFormat: func(key domain.RelationKey) (model.RelationFormat, bool) {
			if key == "related" {
				return model.RelationFormat_object, true
			}
			return 0, false
		},
		ResolveObjectNames: existenceStore{liveCid: true},
	}
}

func pageSnap(related *types.Value) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Details: &types.Struct{Fields: map[string]*types.Value{
			"id":      text("obj1"),
			"name":    text("Host"),
			"related": related,
		}},
		Blocks: []*model.Block{{
			Id:      "obj1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
	}
}

// The real shape of the sweep: an actual round trip through the codec, with
// the SAME options handed to export, import and Compare — exactly how
// cmd/anyblockroundtrip wires it. The dropped entries must not report.
//
// How this can fail: teach export the drop without this file's normalization
// and the objects-format row reports `detail "related" changed` on every
// object carrying a dangling reference — ~990 documents in the last corpus.
func TestCompare_MissingReferenceDropIsNotLoss(t *testing.T) {
	t.Run("objects-format value through a real round trip", func(t *testing.T) {
		// given
		opts := missingRefOpts()
		orig := pageSnap(list(liveCid, deadCid, missingObjectSentinel))
		data, err := anyblockjson.Marshal(model.SmartBlockType_Page, orig, opts)
		require.NoError(t, err)
		sbType, got, err := anyblockjson.Unmarshal(data, opts)
		require.NoError(t, err)

		// when
		diffs := Compare(orig, got, sbType, opts)

		// then
		assert.Empty(t, diffs, "a dropped-by-design entry is a normalization, not loss")
	})

	t.Run("object_types on a property document through a real round trip", func(t *testing.T) {
		// given — the corpus shape: an object id naming nothing beside a
		// live type id and a legacy bare key
		opts := missingRefOpts()
		opts.ResolveProperties = relTypeResolver{}
		orig := relationSnap(map[string]*types.Value{
			"relationFormat":            number(float64(model.RelationFormat_object)),
			"relationFormatObjectTypes": list("typeid-page", deadCid, "wine"),
		})
		data, err := anyblockjson.Marshal(model.SmartBlockType_STRelation, orig, opts)
		require.NoError(t, err)
		sbType, got, err := anyblockjson.Unmarshal(data, opts)
		require.NoError(t, err)

		// when
		diffs := Compare(orig, got, sbType, opts)

		// then
		assert.Empty(t, diffs)
	})
}

// The suppression is scoped exactly to what export drops: a LIVE entry that
// vanishes still reports, and with no existence capability in the options
// nothing is suppressed — export dropped nothing, so a shorter list really
// is loss.
func TestCompare_MissingReferenceScopeStaysTight(t *testing.T) {
	t.Run("a live entry that vanishes still reports", func(t *testing.T) {
		// given
		opts := missingRefOpts()
		orig := pageSnap(list(liveCid, deadCid))
		got := pageSnap(list())

		// when
		diffs := Compare(orig, got, model.SmartBlockType_Page, opts)

		// then
		require.Len(t, diffs, 1)
		assert.Contains(t, diffs[0], `detail "related" changed`)
	})

	t.Run("no capability in the options: a dropped entry is loss", func(t *testing.T) {
		// given — the gate the export side has, mirrored: absence of an
		// answer is not evidence of absence, so a comparator wired without
		// the store must not excuse a missing entry
		opts := missingRefOpts()
		opts.ResolveObjectNames = nil
		orig := pageSnap(list(liveCid, deadCid))
		got := pageSnap(list(liveCid))

		// when
		diffs := Compare(orig, got, model.SmartBlockType_Page, opts)

		// then
		require.Len(t, diffs, 1)
		assert.Contains(t, diffs[0], `detail "related" changed`)
	})
}
