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
//	name                75 of 77   → index.name
//	description         12 of 77   → index.description
//	featuredRelations   12 of 77   → what the space OBJECT features, which is
//	                                 nothing once the object is not a document
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
		idx.Homepage = v
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
	if len(base.GetBlocks()) > 1 {
		// a space object with real content on its page is not a restatement
		// of anything; every one of the 77 in the corpus has none
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
