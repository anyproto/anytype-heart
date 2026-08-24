package anyblockjson

// spacesettings_test.go — the space's own object, and why a bundle carries
// index.json instead of one (§2c).

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func spaceSnapshot(extra map[string]*types.Value) *model.SmartBlockSnapshotBase {
	det := map[string]*types.Value{
		"id": str("bafyreispace"), "name": str("My space"),
		"homepage": str("bafyreihome"), "layout": num(9), "resolvedLayout": num(10),
		"isHidden": {Kind: &types.Value_BoolValue{BoolValue: true}},
	}
	for k, v := range extra {
		det[k] = v
	}
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "bafyreispace",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(det),
	}
}

// After every rule already in this package has run, a space document reduces
// to a restatement of index.json — measured over 77 corpus space documents:
// homepage 77, name 75, description 12, featuredRelations 12, and nothing
// else. index.json says the first three and exists exactly once per bundle,
// because an export is a single space.
//
// The predicate is FAIL-CLOSED: a member this package cannot account for
// keeps the document, so a space carrying something unforeseen travels
// rather than vanishing.
//
// How this can fail: make the default arm return true and an unaccounted
// member disappears with the document; drop the kind gate and an ordinary
// page stops being exported.
func TestSpaceSettings_OmittedOnlyWhenTheIndexSaysItAll(t *testing.T) {
	t.Run("a plain space document is omitted", func(t *testing.T) {
		assert.True(t, OmittedSpaceSettings(model.SmartBlockType_Workspace, spaceSnapshot(nil)))
	})

	t.Run("the secrets it used to carry do not stop the omission", func(t *testing.T) {
		// they are refused by their own rule (§3), so they are accounted for
		assert.True(t, OmittedSpaceSettings(model.SmartBlockType_Workspace,
			spaceSnapshot(map[string]*types.Value{
				"spaceInviteFileKey": str("SECRET"), "analyticsSpaceId": str("abc")})))
	})

	t.Run("an unforeseen member keeps the document", func(t *testing.T) {
		assert.False(t, OmittedSpaceSettings(model.SmartBlockType_Workspace,
			spaceSnapshot(map[string]*types.Value{"somethingNobodyPlannedFor": str("x")})),
			"fail closed: a space carrying something unaccounted must travel")
	})

	t.Run("real content on its page keeps the document", func(t *testing.T) {
		snap := spaceSnapshot(nil)
		snap.Blocks = append(snap.Blocks, &model.Block{Id: "p",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "hello"}}})
		assert.False(t, OmittedSpaceSettings(model.SmartBlockType_Workspace, snap))
	})

	t.Run("no other kind is ever omitted here", func(t *testing.T) {
		assert.False(t, OmittedSpaceSettings(model.SmartBlockType_Page, spaceSnapshot(nil)))
	})
}

// The lift is the composer's half of the omission: a bundle that drops the
// document MUST write what it held, or the space loses its name.
//
// How this can fail: drop a field from IndexFromSpaceSettings and the
// omission starts losing it silently — the predicate would still say yes,
// because spaceSettingsIndexKeys claims the index carries it.
func TestSpaceSettings_TheIndexTakesWhatTheDocumentHeld(t *testing.T) {
	// given
	var idx Index

	// when
	IndexFromSpaceSettings(&idx, spaceSnapshot(map[string]*types.Value{
		"description": str("What it is for")}))

	// then
	assert.Equal(t, "My space", idx.Name)
	assert.Equal(t, "What it is for", idx.Description)
	assert.Equal(t, "bafyreihome", idx.Homepage)

	// and every key the predicate treats as index-carried is actually lifted
	for stored := range spaceSettingsIndexKeys {
		var one Index
		IndexFromSpaceSettings(&one, spaceSnapshot(map[string]*types.Value{stored: str("value")}))
		require.NotEqualf(t, Index{}, one,
			"%q is listed as index-carried but the lift writes nothing for it", stored)
	}
}
