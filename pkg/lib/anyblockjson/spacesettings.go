package anyblockjson

// spacesettings.go — the space's own object, and why a bundle does not carry
// one (§2c).
//
// `kind: "space_settings"` holds the space's name, description and homepage.
// A bundle already says all three, in `index.json`, which exists to "describe
// the bundle as a whole" and of which there is exactly one — an export is a
// single space. So the document restates the index and nothing else.
//
// That is not an assumption. Measured over a 77-space export, after every
// rule already in this package has run — attribution stripped as values, the
// one-distinct-value constants dropped, the source space's invite
// credentials and analytics identity refused (§3), and the deprecated
// `spaceDashboardId`/`spaceUxType`/`hasChat` with them — a space document
// reduces to exactly four members:
//
//	homepage            77 of 77   → index.homepage
//	createdDate         77 of 77   → dropped: when the space OBJECT was minted,
//	                                 which a restored space is not
//	lastModifiedDate    77 of 77   → dropped, for the same reason
//	iconOption          74 of 77   → index.icon (a colour)
//	name                75 of 77   → index.name
//	iconImage           56 of 77   → index.icon (an image object in the bundle)
//	description         12 of 77   → index.description
//	featuredRelations   12 of 77   → what the space OBJECT features, which is
//	                                 nothing once the object is not a document
//	iconEmoji            1 of 77   → index.icon
//
// The icon was the reason this could not simply be dropped: a first reading
// of the corpus counted only the members a rendered DOCUMENT still showed and
// concluded four remained. Read from the stored details instead — which is
// what the omission actually sees — three more channels appear, and almost
// every space has one.
//
// So export omits it, and `IndexFromSpaceSettings` is the one place that says
// which detail becomes which index field — so a composer cannot quietly carry
// fewer of them than the omission assumes were carried.

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// spaceSettingsIndexKeys maps the stored detail to the index field it becomes.
// The map is the contract: every member a space document would have carried
// is either here or provably absent by the strips above, and the omission
// predicate checks exactly that rather than trusting the list.
var spaceSettingsIndexKeys = map[string]string{
	bundle.RelationKeyName.String():        "name",
	bundle.RelationKeyDescription.String(): "description",
	"homepage":                             "homepage",
	detailKeyIconEmoji:                     "icon",
	detailKeyIconImage:                     "icon",
	detailKeyIconName:                      "icon",
	detailKeyIconOption:                    "icon",
}

// pageIsEmpty reports an object whose page holds nothing a reader
// would miss — only the header scaffolding every object carries: the root
// block, the header layout, the featured-properties row, and an EMPTY title.
//
// Counting blocks does not answer this: all 77 corpus spaces have an empty
// page, and 17 of them carry four blocks of scaffolding to say so, so a
// `len(blocks) > 1` test keeps 17 documents that hold nothing at all.
//
// Fail-closed, and deliberately narrow: a text block with any text, marks or
// a style other than the two structural ones, and any other block type at
// all, keeps the document.
func pageIsEmpty(base *model.SmartBlockSnapshotBase) bool {
	for _, b := range base.GetBlocks() {
		switch c := b.Content.(type) {
		case *model.BlockContentOfSmartblock, *model.BlockContentOfLayout,
			*model.BlockContentOfFeaturedRelations:
			// the scaffolding the editor puts on every object
		case *model.BlockContentOfText:
			t := c.Text
			if t.GetText() != "" || len(t.GetMarks().GetMarks()) > 0 {
				return false
			}
			if t.GetStyle() != model.BlockContentText_Title &&
				t.GetStyle() != model.BlockContentText_Description {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// spaceIcon chooses the space's icon through the ONE precedence this format
// has (§2b), and reports whether it can be carried WHOLE.
//
// The second return is what makes the omission safe: `iconOf` warns exactly
// where a stored channel cannot be written — an icon name this format cannot
// spell, an image that is not an object id, a list holding more than one. On
// an ordinary object that warning travels with the document. Here there is no
// document to carry it, so anything less than a lossless icon keeps the
// document instead.
func spaceIcon(base *model.SmartBlockSnapshotBase) (icon *Icon, whole bool) {
	det := base.GetDetails().GetFields()
	lossless := true
	ic := iconOf(
		func(k string) *types.Value { return det[k] },
		func(string, string, ...any) { lossless = false },
		nil, // no options here; a space icon is read from the snapshot alone
	)
	return ic, lossless
}

// IndexFromSpaceSettings reads the space's own object into the index fields
// it is the source of (§2c). It is the composer's half of the omission: a
// bundle that drops the document MUST write these, or the space loses its
// name.
//
// It fills only what the object states; an absent detail leaves the index
// field alone, so a composer may set its own name and have the object's not
// overwrite it.
func IndexFromSpaceSettings(idx *Index, base *model.SmartBlockSnapshotBase) {
	if idx == nil || base == nil {
		return
	}
	det := base.GetDetails().GetFields()
	if v := stringDetail(det, bundle.RelationKeyName.String()); v != "" {
		idx.Name = v
	}
	if v := stringDetail(det, bundle.RelationKeyDescription.String()); v != "" {
		idx.Description = v
	}
	if v := stringDetail(det, "homepage"); v != "" {
		// the STORE spells a reserved screen the way core/domain/homepage.go
		// does — a bare `widgets` — while the format spells it `_widgets`,
		// inside the `_` namespace no bundle object may claim (§1). Lifting
		// the stored value verbatim put the wire spelling in the index, and
		// the batch checker then read it as an object id naming nothing:
		// 8 of 77 exported indexes said `"homepage": "widgets"`.
		idx.Homepage = FormatHomepage(v)
	}
	if ic, whole := spaceIcon(base); whole && ic != nil {
		idx.Icon = ic
	}
}

// OmittedSpaceSettings reports a space document a bundle does not write,
// because `index.json` states everything it holds (§2c).
//
// Fail-closed, like the relation omission beside it: a member this package
// cannot account for keeps the document, so a space carrying something
// unforeseen travels rather than vanishing. The accounted-for set is
// everything the strips already remove plus the three index fields — and
// `featuredRelations`, which describes the object rather than the space and
// has nowhere to go once there is no object.
func OmittedSpaceSettings(sbType model.SmartBlockType, base *model.SmartBlockSnapshotBase) bool {
	if sbType != model.SmartBlockType_Workspace || base == nil {
		return false
	}
	if !pageIsEmpty(base) {
		// a space object with real content on its page is not a restatement
		// of anything
		return false
	}
	if _, whole := spaceIcon(base); !whole {
		// the index would carry a lesser icon than the object holds
		return false
	}
	stripped := strippedDetailKeys()
	for k := range base.GetDetails().GetFields() {
		switch {
		case spaceSettingsIndexKeys[k] != "":
			// index.json carries it — IndexFromSpaceSettings is the proof
		case isTransientProperty(k), stripped[k]:
			// already refused or dropped by a rule of its own
		case k == bundle.RelationKeyFeaturedRelations.String():
			// what the space OBJECT features; there is no object to feature
			// anything once the document is gone
		case spaceSettingsConstantKeys[k]:
			// one distinct value across all 77 corpus documents
		case k == bundle.RelationKeyCreatedDate.String(),
			k == bundle.RelationKeyLastModifiedDate.String():
			// when the space OBJECT was minted and last touched. A bundle is
			// not that object: a space restored from one is created when it
			// is restored, so carrying the original timestamps would date the
			// new space to the old one
		default:
			return false // unaccounted: keep the document
		}
	}
	return true
}

// spaceSettingsConstantKeys are the details every space document carries with
// the same value, so a reader learns nothing from them that the kind does not
// already say. Counted across all 77 documents of a 77-space export.
var spaceSettingsConstantKeys = map[string]bool{
	bundle.RelationKeyLayout.String():                 true, // "space", 1 distinct
	bundle.RelationKeyResolvedLayout.String():         true, // "dashboard", 1 distinct
	bundle.RelationKeyIsHidden.String():               true, // true, 1 distinct
	bundle.RelationKeyMigrationObjectContext.String(): true, // 10, 1 distinct
	"migrationBackRelations":                          true, // 1 distinct
	bundle.RelationKeyId.String():                     true, // the envelope carries it
	bundle.RelationKeySpaceId.String():                true, // the bundle IS the space
}

var _ = types.Struct{}
