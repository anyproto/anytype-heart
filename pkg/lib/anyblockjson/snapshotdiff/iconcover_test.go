package snapshotdiff

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func text(s string) *types.Value { return &types.Value{Kind: &types.Value_StringValue{StringValue: s}} }
func number(n float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
}

// §2b lifted nine hidden keys into the typed `icon` and `cover` envelope
// fields, and a source whose stored value is EMPTY is not a source — so a key
// present and empty comes back absent. Measured over a 36 966-object account:
// 2 307 objects produce 3 038 such findings, nearly double the
// recommended-list noise that buried a previous sweep.
//
// The suppression is scoped to that step and nothing else. The rows below pin
// both halves, because a suppression that grew to the whole KEY would go blind
// to the 33 objects in that same account whose cover really is lost — an
// absolute filesystem path a Notion import wrote into coverId, which the typed
// field cannot carry.
//
// How this can fail: delete the isDroppedEmptyIconCover call in Compare and
// the first three rows report a finding each; widen it to ignore the key
// outright and the last three stop reporting.
func TestCompare_DroppedEmptyIconCoverIsNormalization(t *testing.T) {
	for name, tc := range map[string]struct {
		orig, got map[string]*types.Value
		report    []string
	}{
		"an empty emoji beside a real image": {
			orig: map[string]*types.Value{"iconEmoji": text(""), "iconImage": list("bafyimage")},
			got:  map[string]*types.Value{"iconImage": list("bafyimage")},
		},
		"an empty image list beside a real emoji": {
			orig: map[string]*types.Value{"iconImage": list(), "iconEmoji": text("📕")},
			got:  map[string]*types.Value{"iconEmoji": text("📕")},
		},
		"framing present and zero on a gradient cover": {
			orig: map[string]*types.Value{
				"coverId": text("pinkOrange"), "coverType": number(3),
				"coverScale": number(0), "coverX": number(0), "coverY": number(0)},
			got: map[string]*types.Value{"coverId": text("pinkOrange"), "coverType": number(3)},
		},

		"a cover that really was lost": {
			orig: map[string]*types.Value{"coverId": text("/var/folders/j0/T/leaked.png"), "coverType": number(1)},
			got:  map[string]*types.Value{},
			// both keys report: the pair is one cover, and losing it loses
			// both halves. 33 objects in the corpus produce these 66 findings
			report: []string{"coverId", "coverType"},
		},
		"an emoji that really was lost": {
			orig:   map[string]*types.Value{"iconEmoji": text("📕")},
			got:    map[string]*types.Value{},
			report: []string{"iconEmoji"},
		},
		"framing that really was lost": {
			orig:   map[string]*types.Value{"coverY": number(-0.25)},
			got:    map[string]*types.Value{},
			report: []string{"coverY"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			found := Compare(snapWith(tc.orig), snapWith(tc.got), model.SmartBlockType_Page, anyblockjson.Options{})
			if len(tc.report) == 0 {
				assert.Empty(t, found)
				return
			}
			require.Len(t, found, len(tc.report))
			for i, want := range tc.report {
				assert.Contains(t, found[i], want)
			}
		})
	}
}

// The suppression asks the format which values are sources, rather than
// deciding for itself. Two lists would drift, which is exactly how this
// comparator once reported 10 378 false data-loss issues.
func TestCompare_TheLiftedSetIsTheFormatsOwn(t *testing.T) {
	assert.Len(t, anyblockjson.LiftedPropertyKeys(), 9)
	assert.True(t, anyblockjson.DroppedEmptyIconCover("iconEmoji", text("")))
	assert.False(t, anyblockjson.DroppedEmptyIconCover("iconEmoji", text("📕")))
	assert.False(t, anyblockjson.DroppedEmptyIconCover("name", text("")),
		"a key outside the lift list is nothing to do with this rule")
}
