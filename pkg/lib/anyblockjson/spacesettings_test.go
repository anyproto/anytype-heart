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

	// The space object's own timestamps: when it was minted, not when the
	// bundle's content was written. A space restored from a bundle is created
	// when it is restored.
	t.Run("the object's own timestamps do not stop the omission", func(t *testing.T) {
		assert.True(t, OmittedSpaceSettings(model.SmartBlockType_Workspace,
			spaceSnapshot(map[string]*types.Value{
				"createdDate": num(1700000000), "lastModifiedDate": num(1700000001)})))
	})

	// 17 of 77 corpus spaces carry the editor's header scaffolding and no
	// content at all. Counting blocks kept every one of them.
	t.Run("header scaffolding is not content", func(t *testing.T) {
		snap := spaceSnapshot(nil)
		snap.Blocks = append(snap.Blocks,
			&model.Block{Id: "header", Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Header}}},
			&model.Block{Id: "title", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Style: model.BlockContentText_Title}}},
			&model.Block{Id: "featured", Content: &model.BlockContentOfFeaturedRelations{
				FeaturedRelations: &model.BlockContentFeaturedRelations{}}})
		assert.True(t, OmittedSpaceSettings(model.SmartBlockType_Workspace, snap))
	})

	t.Run("an empty block that is not scaffolding keeps the document", func(t *testing.T) {
		snap := spaceSnapshot(nil)
		snap.Blocks = append(snap.Blocks, &model.Block{Id: "p",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Style: model.BlockContentText_Paragraph}}})
		assert.False(t, OmittedSpaceSettings(model.SmartBlockType_Workspace, snap),
			"fail closed: an empty paragraph is still the author's block")
	})

	// The icon is the one member whose carrying can FAIL: an image that is
	// not an object id cannot be written at all. On an ordinary object that
	// warning travels with the document; here there would be no document.
	t.Run("an icon the index cannot carry keeps the document", func(t *testing.T) {
		assert.False(t, OmittedSpaceSettings(model.SmartBlockType_Workspace,
			spaceSnapshot(map[string]*types.Value{
				detailKeyIconImage: str("https://example.com/logo.png")})),
			"fail closed: the index would carry a lesser icon than the object holds")
	})
}

// The space icon travels in index.json, in the one shape every icon in this
// format has (§2b). Measured over 77 corpus spaces: 55 an image, 20 a bare
// colour — the letter avatar — and 2 no icon at all.
//
// How this can fail: give the index a narrower icon than the object surface,
// and the 20 letter avatars are deleted by an export that reports success.
func TestSpaceSettings_TheIconTravelsInTheIndex(t *testing.T) {
	t.Run("an image icon, with the colour it is tinted with", func(t *testing.T) {
		var idx Index
		IndexFromSpaceSettings(&idx, spaceSnapshot(map[string]*types.Value{
			detailKeyIconImage: str("bafyreiimage"), detailKeyIconOption: num(3)}))

		require.NotNil(t, idx.Icon)
		assert.Equal(t, "file", idx.Icon.Format)
		assert.Equal(t, "bafyreiimage", idx.Icon.File)
		assert.NotNil(t, idx.Icon.Color, "the colour rides along with the icon it tints")
		assert.Equal(t, "bafyreiimage", idx.IconImageId())
	})

	t.Run("a letter avatar is a colour and nothing else", func(t *testing.T) {
		var idx Index
		IndexFromSpaceSettings(&idx, spaceSnapshot(map[string]*types.Value{
			detailKeyIconOption: num(3)}))

		require.NotNil(t, idx.Icon, "20 of 77 real spaces have exactly this icon")
		assert.Equal(t, "color", idx.Icon.Format)
		assert.NotNil(t, idx.Icon.Color)
	})

	t.Run("an icon that cannot be carried whole is not carried at all", func(t *testing.T) {
		var idx Index
		IndexFromSpaceSettings(&idx, spaceSnapshot(map[string]*types.Value{
			detailKeyIconImage: str("https://example.com/logo.png")}))

		assert.Nil(t, idx.Icon, "and the document is kept instead, so nothing is lost")
	})

	t.Run("no icon at all", func(t *testing.T) {
		var idx Index
		IndexFromSpaceSettings(&idx, spaceSnapshot(nil))
		assert.Nil(t, idx.Icon)
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

	// and every key the predicate treats as index-carried is actually lifted.
	// Compared against the index the SAME snapshot without that key produces:
	// asserting merely that the result is non-empty proves nothing, because
	// the base snapshot already carries a name and a homepage.
	samples := map[string]*types.Value{
		"name": str("Another name"), "description": str("What it is for"),
		"homepage":         str("bafyreielsewhere"),
		detailKeyIconEmoji: str("📚"), detailKeyIconImage: str("bafyreiimage"),
		detailKeyIconName: str("folder"), detailKeyIconOption: num(3),
	}
	for stored := range spaceSettingsIndexKeys {
		v, ok := samples[stored]
		require.Truef(t, ok, "no sample value for index-carried key %q", stored)

		var without, with Index
		IndexFromSpaceSettings(&without, spaceSnapshot(nil))
		IndexFromSpaceSettings(&with, spaceSnapshot(map[string]*types.Value{stored: v}))
		require.NotEqualf(t, without, with,
			"%q is listed as index-carried but the lift writes nothing for it", stored)
	}
}

// The STORE spells a reserved homepage the way core/domain/homepage.go does —
// a bare `widgets` — while the format spells it `_widgets`, inside the `_`
// namespace no bundle object may claim (§1). The lift has to translate, the
// way the profile writer translates in the other direction.
//
// Measured before the fix: 8 of 77 exported indexes carried
// `"homepage": "widgets"`, which the batch checker read as an object id
// naming nothing in the bundle.
//
// How this can fail: lift the stored value verbatim and a reserved screen
// becomes a dangling object reference; translate an ordinary object id and a
// real homepage stops resolving.
func TestSpaceSettings_AReservedHomepageIsTranslatedOnTheWayIn(t *testing.T) {
	t.Run("the wire spelling becomes the format spelling", func(t *testing.T) {
		var idx Index
		IndexFromSpaceSettings(&idx, spaceSnapshot(map[string]*types.Value{
			"homepage": str("widgets")}))
		assert.Equal(t, HomepageWidgets, idx.Homepage)
		assert.True(t, IsReservedBundleId(idx.Homepage) || idx.Homepage[0] == '_',
			"it must land in the reserved namespace, not look like an object id")
	})

	t.Run("an ordinary object id is untouched", func(t *testing.T) {
		var idx Index
		IndexFromSpaceSettings(&idx, spaceSnapshot(map[string]*types.Value{
			"homepage": str("bafyreihome")}))
		assert.Equal(t, "bafyreihome", idx.Homepage)
	})

	t.Run("and the pair round-trips", func(t *testing.T) {
		for _, wire := range []string{"widgets", "graph"} {
			assert.Equal(t, wire, WireHomepage(FormatHomepage(wire)))
		}
	})
}
