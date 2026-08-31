package anyblockjson

// iconcover_test.go covers §2b: the typed `icon` and `cover` envelope fields.
//
// Every test here goes through a PUBLIC entry point — Marshal, Unmarshal,
// Validate — rather than calling buildIcon/applyIcon, because a test that
// calls the builder it is testing passes whatever the builder does.

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// iconSnapshot is a minimal page carrying the given details.
func iconSnapshot(details map[string]*types.Value) *model.SmartBlockSnapshotBase {
	all := map[string]*types.Value{"id": str("obj1")}
	for k, v := range details {
		all[k] = v
	}
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "obj1", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
		},
		Details: fields(all),
	}
}

// exportedIconCover marshals a details bag and hands back the two envelope
// fields as raw JSON, plus the property members that survived.
func exportedIconCover(t *testing.T, details map[string]*types.Value) (icon, cover string, props map[string]any, warnings []Issue) {
	t.Helper()
	opts := Options{OnWarning: func(i Issue) { warnings = append(warnings, i) }}
	data, err := Marshal(model.SmartBlockType_Page, iconSnapshot(details), opts)
	require.NoError(t, err)
	// Marshal must never emit what its own Validate rejects (§11, I1) — the
	// whole reason the export builders admit values before writing them
	require.NoError(t, Validate(data), "%s", data)

	var doc struct {
		Icon       json.RawMessage `json:"icon"`
		Cover      json.RawMessage `json:"cover"`
		Properties map[string]any  `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	return string(doc.Icon), string(doc.Cover), doc.Properties, warnings
}

// TestExport_EveryEmittedIconShape walks the eight icon shapes and six cover
// shapes a 36 966-object corpus actually produces, each from the stored keys
// that produce it. The counts in the names are that corpus's, measured.
//
// How this can fail: change the precedence (name → emoji → file), drop the
// orthogonal colour, treat iconOption 0 as grey, or let an empty source count
// as a source, and exactly the affected rows fail. The shapes are asserted as
// whole JSON objects, so an extra or missing member fails too.
func TestExport_EveryEmittedIconShape(t *testing.T) {
	for name, tc := range map[string]struct {
		details map[string]*types.Value
		icon    string
		cover   string
	}{
		"file — 11 956 documents": {
			details: map[string]*types.Value{"iconImage": strList("bafyreicfdcmfn")},
			icon:    `{"format":"file","file":"bafyreicfdcmfn"}`,
		},
		"emoji — 1 376 documents": {
			details: map[string]*types.Value{"iconEmoji": str("📕")},
			icon:    `{"format":"emoji","emoji":"📕"}`,
		},
		"named icon with a colour — 1 325 documents, always on a type": {
			details: map[string]*types.Value{"iconName": str("hammer"), "iconOption": num(3)},
			icon:    `{"format":"icon","name":"hammer","color":"orange"}`,
		},
		"the conflict carry-over — 200 documents": {
			details: map[string]*types.Value{
				"iconName": str("folder"), "iconOption": num(10), "iconEmoji": str("🌎")},
			icon: `{"format":"icon","name":"folder","color":"lime","emoji":"🌎"}`,
		},
		"an avatar image and its colour — 55 documents": {
			details: map[string]*types.Value{"iconImage": strList("bafybeic7zrh5fa"), "iconOption": num(5)},
			icon:    `{"format":"file","file":"bafybeic7zrh5fa","color":"pink"}`,
		},
		"a colour with no source — 29 documents": {
			details: map[string]*types.Value{"iconOption": num(9)},
			icon:    `{"format":"color","color":"teal"}`,
		},
		"a named icon with no colour — 5 documents": {
			details: map[string]*types.Value{"iconName": str("folder")},
			icon:    `{"format":"icon","name":"folder"}`,
		},
		"an emoji with a colour — 3 documents": {
			details: map[string]*types.Value{"iconEmoji": str("🍷"), "iconOption": num(7)},
			icon:    `{"format":"emoji","emoji":"🍷","color":"blue"}`,
		},
		"the integer colour escape — 6 documents": {
			details: map[string]*types.Value{"iconOption": num(13)},
			icon:    `{"format":"color","color":13}`,
		},

		"a framed image cover — 73 documents": {
			details: map[string]*types.Value{
				"coverId": str("bafyreigejpawpb"), "coverType": num(1), "coverY": num(-0.25)},
			cover: `{"format":"image","file":"bafyreigejpawpb","y":-0.25}`,
		},
		"a bare image cover — 23 documents": {
			details: map[string]*types.Value{"coverId": str("bafyreif3z354yh"), "coverType": num(1)},
			cover:   `{"format":"image","file":"bafyreif3z354yh"}`,
		},
		"a gradient — 14 documents": {
			details: map[string]*types.Value{"coverId": str("pinkOrange"), "coverType": num(3)},
			cover:   `{"format":"gradient","gradient":"pinkOrange"}`,
		},
		"an unsplash image, provenance carried — 11 documents": {
			details: map[string]*types.Value{
				"coverId": str("bafyreibzmdjk"), "coverType": num(5), "coverY": num(0.25)},
			cover: `{"format":"image","file":"bafyreibzmdjk","source":"unsplash","y":0.25}`,
		},
		"a colour cover — 4 documents. `black` is NOT in the option palette": {
			details: map[string]*types.Value{"coverId": str("black"), "coverType": num(2)},
			cover:   `{"format":"color","color":"black"}`,
		},
		"a fully framed image — 1 document": {
			details: map[string]*types.Value{
				"coverId": str("bafyone"), "coverType": num(1),
				"coverScale": num(0.5), "coverX": num(0.1), "coverY": num(0.2)},
			cover: `{"format":"image","file":"bafyone","scale":0.5,"x":0.1,"y":0.2}`,
		},

		// the discriminator is load-bearing: the SAME cover_id under two
		// types is two different covers, and both spellings occur in the
		// corpus
		"`blue` as a colour": {
			details: map[string]*types.Value{"coverId": str("blue"), "coverType": num(2)},
			cover:   `{"format":"color","color":"blue"}`,
		},
		"`blue` as a gradient": {
			details: map[string]*types.Value{"coverId": str("blue"), "coverType": num(3)},
			cover:   `{"format":"gradient","gradient":"blue"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			icon, cover, props, _ := exportedIconCover(t, tc.details)
			assert.Equal(t, tc.icon, compactJSON(t, icon))
			assert.Equal(t, tc.cover, compactJSON(t, cover))
			for key := range tc.details {
				assert.NotContains(t, props, key, "a lifted key is written nowhere else")
			}
		})
	}
}

// compactJSON strips the whitespace out of a JSON fragment so a shape can be
// asserted as one readable line. It compacts the BYTES rather than re-encoding
// through a Go map, because member order is part of the canonical form (§4)
// and a map would silently sort it away. Empty in, empty out.
func compactJSON(t *testing.T, raw string) string {
	t.Helper()
	if raw == "" {
		return ""
	}
	var buf bytes.Buffer
	require.NoError(t, json.Compact(&buf, []byte(raw)))
	return buf.String()
}

// An EMPTY source is not a source. This is the format's largest class of fake
// ambiguity: 883 corpus objects carry both `iconEmoji` and `iconImage`, and in
// NOT ONE of them are both non-empty — the 737 "conflicts" a brief reported
// were empty siblings, present because §3 makes key presence meaningful.
//
// How this can fail: drop the emptiness guard in buildIcon/buildCover and the
// first row emits `{"format":"emoji","emoji":""}`, which Validate then refuses
// (minLength 1) — so exportedIconCover's I1 check fails too.
func TestExport_AnEmptySourceIsNotASource(t *testing.T) {
	for name, details := range map[string]map[string]*types.Value{
		"an empty emoji beside an image": {
			"iconEmoji": str(""), "iconImage": strList("bafyimage")},
		"an empty image list beside an emoji": {
			"iconImage": strList(), "iconEmoji": str("📕")},
	} {
		t.Run(name, func(t *testing.T) {
			icon, _, props, _ := exportedIconCover(t, details)
			assert.NotContains(t, icon, `""`, "the empty sibling is gone, not written empty")
			assert.NotContains(t, icon, "[]")
			assert.Empty(t, props, "and it does not survive in properties either")
		})
	}

	t.Run("every source empty means no icon at all", func(t *testing.T) {
		icon, cover, props, _ := exportedIconCover(t, map[string]*types.Value{
			"iconEmoji": str(""), "iconImage": strList(), "iconName": str(""),
			"iconOption": num(0), "coverId": str(""), "coverType": num(0),
			"coverScale": num(0), "coverX": num(0), "coverY": num(0),
		})
		assert.Empty(t, icon)
		assert.Empty(t, cover)
		assert.Empty(t, props, "1 358 corpus objects are exactly this shape")
	})

	t.Run("iconOption 0 is the proto zero, not the first colour", func(t *testing.T) {
		icon, _, _, _ := exportedIconCover(t, map[string]*types.Value{
			"iconEmoji": str("📕"), "iconOption": num(0)})
		assert.Equal(t, `{"format":"emoji","emoji":"📕"}`, compactJSON(t, icon),
			"145 corpus objects carry iconOption 0; none of them is grey")
	})
}

// A value the schema cannot hold is dropped with a warning rather than
// written, because Marshal emitting what its own Validate rejects is the
// failure nobody sees until the archive is needed (§11, I1).
//
// The cover case is real data: 33 corpus objects carry an absolute path into a
// long-gone temp directory, written by core/block/import/notion straight into
// coverId as if it were a file reference. 25% of every image cover in that
// corpus is permanently corrupt by exactly the mechanism the typed field
// exists to prevent.
//
// How this can fail: remove the isObjectRef guard and the Validate call inside
// exportedIconCover fails on `does not match pattern '^[^/]+$'`.
func TestExport_AValueTheFormatCannotWriteIsDropped(t *testing.T) {
	const notionPath = "/var/folders/j0/T/anytype_notion_import/0df562e1.png"

	t.Run("a leaked filesystem path in coverId", func(t *testing.T) {
		_, cover, props, warnings := exportedIconCover(t, map[string]*types.Value{
			"coverId": str(notionPath), "coverType": num(1)})
		assert.Empty(t, cover, "the cover is dropped: there is no way to write it")
		assert.Empty(t, props)
		require.Len(t, warnings, 1)
		assert.Equal(t, "/cover", warnings[0].Path)
		assert.Contains(t, warnings[0].Message, "never a URL or a path")
	})

	t.Run("a URL in iconImage falls through to the colour", func(t *testing.T) {
		icon, _, _, warnings := exportedIconCover(t, map[string]*types.Value{
			"iconImage": strList("https://images.example/hero.jpg"), "iconOption": num(2)})
		assert.Equal(t, `{"format":"color","color":"yellow"}`, compactJSON(t, icon),
			"an unusable source is not a source, so the chain continues")
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "not an object id")
	})

	t.Run("a cover type outside 0..5", func(t *testing.T) {
		_, cover, _, warnings := exportedIconCover(t, map[string]*types.Value{
			"coverId": str("bafy"), "coverType": num(9)})
		assert.Empty(t, cover)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "not one of 0..5")
	})

	t.Run("a cover id with no type says nothing", func(t *testing.T) {
		_, cover, _, warnings := exportedIconCover(t, map[string]*types.Value{
			"coverId": str("blue")})
		assert.Empty(t, cover, "`blue` under no type is a colour or a gradient and nothing says which")
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "no cover type")
	})
}

// The conflict carry-over is the only emitted shape with two icon sources, and
// it is warned about rather than silent — 200 corpus objects, every one a
// bundled type mid-migration from an emoji to a named icon.
func TestExport_TheConflictCarryOverIsWarned(t *testing.T) {
	icon, _, _, warnings := exportedIconCover(t, map[string]*types.Value{
		"iconName": str("extension-puzzle"), "iconEmoji": str("🥚"), "iconOption": num(6)})
	assert.Equal(t, `{"format":"icon","name":"extension-puzzle","color":"purple","emoji":"🥚"}`,
		compactJSON(t, icon))
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "the name wins")
}

// Every emitted shape inverts to exactly the details that produced it, which
// is what makes Export ∘ Import a fixpoint over these fields (§11 guarantees
// 2 and 3). Measured over the whole corpus: 36 966 objects, 0 byte-unstable.
//
// How this can fail: any disagreement between the export builder and the
// import inverter — a colour that maps one way and back another, a cover
// source that loses its type, an iconImage written as a scalar where the
// store holds a list — shows up either as a changed detail or as different
// bytes on the second export.
func TestRoundTrip_IconAndCoverAreAFixpoint(t *testing.T) {
	for name, details := range map[string]map[string]*types.Value{
		"emoji":            {"iconEmoji": str("📕")},
		"file":             {"iconImage": strList("bafyimage")},
		"file with colour": {"iconImage": strList("bafyimage"), "iconOption": num(5)},
		"named icon":       {"iconName": str("hammer"), "iconOption": num(3)},
		"carry-over":       {"iconName": str("folder"), "iconOption": num(10), "iconEmoji": str("🌎")},
		"colour only":      {"iconOption": num(9)},
		"integer colour":   {"iconOption": num(13)},
		"image cover":      {"coverId": str("bafycover"), "coverType": num(1), "coverY": num(-0.25)},
		"unsplash cover":   {"coverId": str("bafycover"), "coverType": num(5)},
		"prebuilt cover":   {"coverId": str("bafycover"), "coverType": num(4)},
		"colour cover":     {"coverId": str("blue"), "coverType": num(2)},
		"gradient cover":   {"coverId": str("blue"), "coverType": num(3)},
		"framed cover": {"coverId": str("bafycover"), "coverType": num(1),
			"coverScale": num(0.5), "coverX": num(0.1), "coverY": num(0.2)},
		"both at once": {"iconEmoji": str("📕"), "coverId": str("pinkOrange"), "coverType": num(3)},
	} {
		t.Run(name, func(t *testing.T) {
			first, err := Marshal(model.SmartBlockType_Page, iconSnapshot(details), Options{})
			require.NoError(t, err)

			sbType, snap, err := Unmarshal(first, Options{GenerateId: seqIds("g")})
			require.NoError(t, err)

			for key, want := range details {
				got := snap.Details.Fields[key]
				require.NotNil(t, got, "%q must come back", key)
				if key == "iconImage" {
					// a `file` relation stores a list, which is what the
					// ordinary property path writes too (wrapToList)
					assert.Equal(t, valueStringList(want), valueStringList(got), key)
					continue
				}
				assert.Equal(t, want.String(), got.String(), key)
			}

			second, err := Marshal(sbType, snap, Options{})
			require.NoError(t, err)
			assert.Equal(t, string(first), string(second), "Export ∘ Import must be byte-stable")
		})
	}
}

// Import refuses the nine flat spellings in `properties`, on the RESOLVED
// stored key, and the refusal names the repair. The refusal is derived from
// the export side's own lift list, never restated (deniedPropertyKey) — a
// restated list is how the two surfaces drifted apart the last time.
//
// How this can fail: drop the liftedDetailKeys arm of deniedPropertyKey and
// every row here accepts the document, storing an icon in a slot export never
// writes — so the value would be invisible to the next export.
func TestImport_TheFlatSpellingsAreRefused(t *testing.T) {
	// The spellings that RESOLVE onto the lifted keys are their display
	// names and their verbatim stored keys. The pre-change flat slugs
	// (`icon_emoji`, `cover_id`) resolve nothing at all now — a denied
	// key's fold class answers nothing — so they are ordinary custom keys
	// that land on no icon slot, which the last arm pins.
	for name, tc := range map[string]struct{ doc, repair string }{
		"the display name": {
			doc:    `{"version": 2, "properties": {"Emoji": "🔥"}}`,
			repair: `"icon": {"format": "emoji", "emoji": "…"}`,
		},
		"the stored key spelled verbatim": {
			doc:    `{"version": 2, "properties": {"iconImage": ["bafy"]}}`,
			repair: `"icon": {"format": "file", "file": "<object id>"}`,
		},
		"an icon name": {
			doc:    `{"version": 2, "properties": {"iconName": "folder"}}`,
			repair: `"icon": {"format": "icon", "name": "…"}`,
		},
		"an icon option": {
			doc:    `{"version": 2, "properties": {"iconOption": 3}}`,
			repair: `the "color" member of "icon"`,
		},
		"a cover id": {
			doc:    `{"version": 2, "properties": {"coverId": "blue"}}`,
			repair: `"cover": {"format": "image"|"color"|"gradient", …}`,
		},
		"cover framing": {
			doc:    `{"version": 2, "properties": {"coverY": -0.25}}`,
			repair: `the "scale"/"x"/"y" members of an image "cover"`,
		},
		// the laundering case: a legend can bind any spelling to any stored
		// key, so admission runs on what the spelling RESOLVES to
		"a spelling the legend binds to a lifted key": {
			doc: `{"version": 2, "property_internal_keys": {"sneaky": "iconEmoji"},
				"properties": {"sneaky": "🔥"}}`,
			repair: `"icon": {"format": "emoji", "emoji": "…"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := Validate([]byte(tc.doc))
			require.Error(t, err, "Unmarshal refuses this, so Validate must too (I2)")
			assert.Contains(t, err.Error(), tc.repair, "the refusal names what to write instead")

			_, _, unmErr := Unmarshal([]byte(tc.doc), Options{GenerateId: seqIds("g")})
			require.Error(t, unmErr, "and the seam refuses it whatever vocabulary is wired")
		})
	}

	t.Run("the retired flat slug is an ordinary custom key", func(t *testing.T) {
		doc := `{"version": 2, "properties": {"icon_emoji": "🔥"}}`
		require.NoError(t, Validate([]byte(doc)))
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Nil(t, snap.Details.Fields["iconEmoji"], "it lands on no icon slot")
		assert.NotNil(t, snap.Details.Fields["icon_emoji"], "it is its own key, verbatim")
	})
}

// The refusal runs on the RESOLVED stored key, which is what keeps a
// space-minted relation whose OWN stored key is `icon_emoji` an ordinary
// property. 54 corpus objects are exactly this: a bundled `iconEmoji` that is
// empty, and a custom relation stored as `icon_emoji` holding a real emoji.
// Anything reading "the icon" out of `icon_emoji` in those documents today
// reads a coffee-tasting note.
//
// How this can fail: run the deny check on the SPELLING instead of the
// resolved key and this document is refused, taking a legitimate user
// relation with it.
func TestImport_ASpaceMintedIconEmojiRelationIsAnOrdinaryProperty(t *testing.T) {
	doc := `{"version": 2, "id": "o1",
		"property_internal_keys": {"icon_emoji": "icon_emoji"},
		"icon": {"format": "emoji", "emoji": "📕"},
		"properties": {"icon_emoji": "☕"}}`

	require.NoError(t, Validate([]byte(doc)))
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)

	assert.Equal(t, "☕", snap.Details.Fields["icon_emoji"].GetStringValue(),
		"the space's own relation keeps its value")
	assert.Equal(t, "📕", snap.Details.Fields["iconEmoji"].GetStringValue(),
		"and the envelope field wrote the bundled one")

	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"icon": {`, "the two are visibly different things")
	assert.Contains(t, string(data), `"icon_emoji": "☕"`)
}

// The typed fields live in the ENVELOPE, and one reason is not aesthetic:
// `cover` is already a stored property key in real data — 30 corpus objects
// mint a relation whose stored key is literally `cover`, and 66 more spell it
// `pageCover`, both Notion imports, neither bundled. An envelope field is
// outside the key namespace the legend can rebind.
func TestImport_AStoredRelationNamedCoverIsUntouched(t *testing.T) {
	doc := `{"version": 2, "id": "o1",
		"property_internal_keys": {"cover": "cover"},
		"cover": {"format": "gradient", "gradient": "pinkOrange"},
		"properties": {"cover": "a photograph of the team"}}`

	require.NoError(t, Validate([]byte(doc)))
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)

	assert.Equal(t, "a photograph of the team", snap.Details.Fields["cover"].GetStringValue())
	assert.Equal(t, "pinkOrange", snap.Details.Fields["coverId"].GetStringValue())
	assert.Equal(t, float64(3), snap.Details.Fields["coverType"].GetNumberValue())
}

// A callout carries the same typed icon, restricted to the two kinds a block
// can hold (§5.2). Shipping the envelope field without this would leave two
// icon conventions inside one document — the exact defect being removed.
func TestRoundTrip_CalloutIcon(t *testing.T) {
	callout := func(emoji, image string) *model.SmartBlockSnapshotBase {
		return &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{
				{Id: "obj1", ChildrenIds: []string{"c1"},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				{Id: "c1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Style: model.BlockContentText_Callout, Text: "Ship it.",
					IconEmoji: emoji, IconImage: image}}},
			},
			Details: fields(map[string]*types.Value{"id": str("obj1")}),
		}
	}
	for name, tc := range map[string]struct {
		snap *model.SmartBlockSnapshotBase
		want string
	}{
		"an emoji": {callout("💡", ""), `{"format":"emoji","emoji":"💡"}`},
		"an image": {callout("", "bafyimage"), `{"format":"file","file":"bafyimage"}`},
		"neither":  {callout("", ""), ``},
	} {
		t.Run(name, func(t *testing.T) {
			first, err := Marshal(model.SmartBlockType_Page, tc.snap, Options{})
			require.NoError(t, err)
			require.NoError(t, Validate(first), "%s", first)

			var doc struct {
				Blocks []struct {
					Icon json.RawMessage `json:"icon"`
				} `json:"blocks"`
			}
			require.NoError(t, json.Unmarshal(first, &doc))
			require.Len(t, doc.Blocks, 1)
			assert.Equal(t, tc.want, compactJSON(t, string(doc.Blocks[0].Icon)))

			sbType, snap, err := Unmarshal(first, Options{GenerateId: seqIds("g")})
			require.NoError(t, err)
			second, err := Marshal(sbType, snap, Options{})
			require.NoError(t, err)
			assert.Equal(t, string(first), string(second))
		})
	}

	t.Run("the four-variant object icon is not a callout icon", func(t *testing.T) {
		err := Validate([]byte(`{"version": 2, "blocks": [
			{"type": "callout", "icon": {"format": "icon", "name": "folder"}, "text": "x"}]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value must be one of 'emoji', 'file'")
	})
}

// §12 promises one fault, one issue, and for a typed union that promise is the
// whole design: a model spends most of its time in the failure path, and the
// message is where the alternatives have to appear.
//
// Measured against the `oneOf` encoding both original designs specified: the
// first row below reported ELEVEN issues, three of them contradictory verdicts
// on `format`, twice telling the author to delete the CORRECT member. The
// `if`/`then` encoding reports one. `branchLeaves` cannot prune the difference
// — it prunes branches that failed on the instance's own TYPE, and every icon
// branch is `type: object`.
//
// How this can fail: re-encode the union as `oneOf` and every row here reports
// a handful of issues instead of one.
func TestValidate_AWrongIconGetsExactlyOneIssue(t *testing.T) {
	for _, tc := range []struct{ doc, path, message string }{
		{`{"version":2,"icon":{"format":"emoji","emoji":"📕","name":"rocket"}}`,
			"/icon/name", `property "name" is not allowed`},
		{`{"version":2,"icon":{"format":"image","url":"https://x/y.png"}}`,
			"/icon/format", "value must be one of 'emoji', 'file', 'icon', 'color'"},
		{`{"version":2,"icon":{"format":"url","url":"https://x/y.png"}}`,
			"/icon/format", "value must be one of 'emoji', 'file', 'icon', 'color'"},
		{`{"version":2,"icon":{"emoji":"🚀"}}`,
			"/icon", "an icon is one of 'emoji', 'file', 'icon', 'color'"},
		{`{"version":2,"icon":{"format":"emoji"}}`,
			"/icon", "missing property 'emoji'"},
		{`{"version":2,"icon":{"format":"icon","name":"rocket","color":"turquoise"}}`,
			"/icon/color", "value must be one of 'grey', 'yellow'"},
		{`{"version":2,"icon":{"format":"file","file":"https://images.example/hero.jpg"}}`,
			"/icon/file", `does not match pattern '^[^/]+$'`},
		{`{"version":2,"cover":{"format":"image","file":"/var/folders/j0/T/x.png"}}`,
			"/cover/file", `does not match pattern '^[^/]+$'`},
		{`{"version":2,"cover":{"format":"unsplash","file":"bafy"}}`,
			"/cover/format", "value must be one of 'image', 'color', 'gradient'"},
		{`{"version":2,"cover":{"gradient":"pinkOrange"}}`,
			"/cover", "a cover is one of 'image', 'color', 'gradient'"},
		{`{"version":2,"blocks":[{"type":"callout","icon":{"emoji":"💡"},"text":"x"}]}`,
			"/blocks/0/icon", "a callout icon is one of 'emoji', 'file'"},
	} {
		t.Run(tc.doc, func(t *testing.T) {
			err := Validate([]byte(tc.doc))
			require.Error(t, err)
			var ve *ValidationError
			require.ErrorAs(t, err, &ve)
			require.Len(t, ve.Issues, 1, "one fault, one issue (§12): %v", ve.Issues)
			assert.Equal(t, tc.path, ve.Issues[0].Path)
			assert.Contains(t, ve.Issues[0].Message, tc.message)
		})
	}
}

// The union a `format` verdict names is read out of the published schema, not
// restated in Go — which is what makes the extension seam free: a layer that
// appends a `url` variant to the schema gets it named in the reader's own
// diagnostics without touching this package (§2b).
//
// How this can fail: hardcode the variant names in iconFormatIssues and this
// test still passes on the happy path but the appended variant never appears.
func TestValidate_TheFormatUnionIsReadFromTheSchema(t *testing.T) {
	assert.Equal(t, []string{"emoji", "file", "icon", "color"}, schemaFormatEnum("icon"))
	assert.Equal(t, []string{"image", "color", "gradient"}, schemaFormatEnum("cover"))
	assert.Equal(t, []string{"emoji", "file"}, schemaFormatEnum("plainIcon"),
		"the narrowed definition answers with the narrowed set, not the one it refs")
}

// The nine lifted keys are the whole family and nothing else, and every one is
// `hidden: true`. The hidden-ness is the entire justification for overriding
// §3's presence-is-meaningful rule: a hidden relation has no property row for
// presence to be meaningful TO. Add a visible relation to the lift list and
// this fails, which is the point.
func TestLiftedKeysAreHiddenRelations(t *testing.T) {
	want := []string{
		"coverId", "coverScale", "coverType", "coverX", "coverY",
		"iconEmoji", "iconImage", "iconName", "iconOption",
	}
	got := make([]string, 0, len(LiftedPropertyKeys()))
	for k := range LiftedPropertyKeys() {
		got = append(got, k)
	}
	assert.ElementsMatch(t, want, got)

	for _, key := range want {
		rel, err := bundle.GetRelation(domain.RelationKey(key))
		require.NoError(t, err, key)
		assert.True(t, rel.Hidden, "%q must be hidden, or dropping it when empty is real loss", key)
	}
}

// An icon's `file` holds TWO address spaces, and the format has to say so
// because a reader cannot tell from the slot alone.
//
// Normally it is the id of a file object in the bundle — 11,251 of 12,378 in
// a 77-space export. But a participant avatar and a space invite icon are
// stored by the app as the raw content cid of the image itself
// (core/acl/aclservice.go writes SpaceIconCid into iconImage, whose format is
// `file`), and those 992 can never resolve as object ids: a content cid is
// raw/dag-pb and begins `bafybei`, an object id is dag-cbor and begins
// `bafyrei`. A reader dereferencing one against the bundle finds nothing —
// not because the object is missing, but because it was never an object.
//
// Both shapes are legal and both must stay legal; what the format owes is the
// distinction, which now lives in $defs/objectRef and on the file variant.
//
// How this can fail: constrain the slot to one address space and 992 real
// participant icons become invalid; drop the description and a reader has no
// way to learn the rule.
func TestIcon_FileHoldsEitherAnObjectIdOrAContentCid(t *testing.T) {
	for _, tc := range []struct{ name, id string }{
		{"an object id", "bafyreiarrls75xlsmbc4hwhjuht34fgkzz5xpvpkexmzqov4oxssgckohy"},
		{"a raw content cid, as a participant avatar carries", "bafybeig56mk6qmlv624q3ykecbok5v7wmzeqc5zlci2h6d42w5pg3hx6bu"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := `{"version": 2, "id": "o1", "icon": {"format": "file", "file": "` + tc.id + `"}}`
			require.NoError(t, Validate([]byte(doc)))
			_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
			require.NoError(t, err)
			assert.Equal(t, strList(tc.id), snap.Details.Fields[detailKeyIconImage],
				"the stored value is the same slot either way")
		})
	}

	t.Run("the schema says how to tell them apart", func(t *testing.T) {
		var s struct {
			Defs map[string]struct {
				Description string `json:"description"`
			} `json:"$defs"`
		}
		require.NoError(t, json.Unmarshal(schemaJSON, &s))
		d := s.Defs["objectRef"].Description
		assert.Contains(t, d, "bafybei", "the content-cid codec must be named")
		assert.Contains(t, d, "bafyrei", "and the object-id codec beside it")
	})
}
