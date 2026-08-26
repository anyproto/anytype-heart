package anyblockjson

// index.go implements §2c: the bundle-level index.json. Every other document
// in this format describes one object; index.json describes the set — the
// space's name, what opens on entry, and what the sidebar shows.
//
// It exists because none of that is expressible per-object. The wiring splits
// it across two outputs, because the installer takes them from two places: a
// `profile` file at the archive root (pb.Profile, read by util/builtinobjects)
// carries spaceDashboardId, and the sidebar travels as a Widget snapshot the
// wiring BUILDS from index.widgets (WidgetsSnapshot) — a bundle itself
// carries no widget document, the way it carries no space document. See §2c.
// A bundle without an index imports as an undifferentiated object list.

import (
	"bytes"
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema/index.schema.json
var indexSchemaJSON []byte

// IndexFileName is the name a bundle's index must have, at the bundle root.
const IndexFileName = "index.json"

// PlatformPrefix opens the platform's own address space. A value beginning
// with it — `_otpage`, `_brdue_date`, `_missing_object`, `_participant_…` —
// names something the platform provides, never something a document or a
// bundle mints (§1). The format borrows the same namespace for the built-in
// screens and listings an index.json may name, which is what makes the two
// sets provably disjoint: **no bundle-local object id may begin with `_`**.
//
// That disjointness is the whole point, and it is not decorative. The pb
// importer resolves a widget's target through the bundle's own id map FIRST
// (common.UpdateLinksToObjects) and only then asks
// widget.IsPredefinedWidgetTargetId. While the reserved listings were bare
// words — `set`, `favorite` — a bundle shipping an object with id `set`
// silently captured the widget that meant *the Sets listing*, and
// Index.EntryPoint, which skips reserved targets, silently disagreed with
// EffectiveEntryPoint about what the bundle even opens. A prefix rule needs
// only "no minted id STARTS with `_`", which is permanently true, where a
// per-word reservation needs a new ban every time a listing is added.
const PlatformPrefix = "_"

// IsPlatformId reports whether id lies in the reserved `_` namespace, and so
// addresses the platform rather than anything a bundle ships.
func IsPlatformId(id string) bool {
	return strings.HasPrefix(id, PlatformPrefix)
}

// Reserved homepage values. Anything else is an object id.
//
// These are the format's spellings, not the wire's: core/domain/homepage.go
// carries them as the bare `widgets` / `graph`, and builtinobjects switches on
// those names BEFORE looking an id up — the opposite precedence from widget
// targets, and the reason a bundle object with id `graph` can never be a
// homepage. Both directions close once the reserved spellings live in the
// platform namespace. The `_` is translated away at the wire boundary, by
// WireHomepage.
const (
	HomepageWidgets = "_widgets"
	HomepageGraph   = "_graph"
)

// wireHomepages is the format spelling of each reserved homepage and the
// spelling core/domain/homepage.go uses for the same screen. WireHomepage
// walks it one way and FormatHomepage the other: the STORE holds the wire
// spelling, so a value lifted out of a space object has to be translated
// before it becomes an index field, exactly as one written to a profile has
// to be translated on the way out.
var wireHomepages = map[string]string{
	HomepageWidgets: "widgets",
	HomepageGraph:   "graph",
}

// reservedWidgetTargets are the widget targets that name a built-in listing
// rather than an object in the bundle (core/block/editor/widget).
var reservedWidgetTargets = map[string]struct{}{
	"_favorite": {}, "_recent": {}, "_set": {}, "_collection": {},
	"_all_objects": {}, "_recent_open": {}, "_chat": {}, "_bin": {},
}

// importableWidgetTargets are the reserved targets the *importer* recognises:
// exactly widget.IsPredefinedWidgetTargetId, which is what
// common.handleLinkBlock consults before deciding a link target it cannot
// resolve is broken. The wire spellings are the live space's own — bare and
// sometimes camelCase (`favorite`, `allObjects`) — and the wiring translates
// them at the boundary, both ways (WireWidgetTarget / FormatWidgetTarget).
//
// The inventory is the full set of listings live spaces actually hold:
// measured over a 77-space account, 33 of 218 widget links name one, and the
// population is chat 11 · bin 10 · allObjects 8 · recent 1 · set 1 (plus two
// strays no client defines, which stay inexpressible). For one revision the
// importer knew only four of these and this map said so — a bundle naming
// `_all_objects` was refused up front, because the importer would have
// rewritten the link to addr.MissingObject and WidgetObject.Init would have
// stripped it, losing the widget with no error. The importer knows all eight
// now, so all eight travel.
//
// The map is the translation table as well as the membership test, so the two
// cannot drift: adding a listing without giving it a wire spelling is not
// expressible.
var importableWidgetTargets = map[string]string{
	"_favorite": "favorite", "_recent": "recent", "_set": "set", "_collection": "collection",
	"_all_objects": "allObjects", "_recent_open": "recentOpen", "_chat": "chat", "_bin": "bin",
}

// wireWidgetTargetsByWire inverts importableWidgetTargets, so a stored link
// target can be lifted into the format's spelling by lookup rather than by a
// second table that would drift.
var wireWidgetTargetsByWire = func() map[string]string {
	out := make(map[string]string, len(importableWidgetTargets))
	for format, wire := range importableWidgetTargets {
		out[wire] = format
	}
	return out
}()

// IsReservedWidgetTarget reports whether target names a built-in listing, in
// which case it does not name an object in the bundle.
func IsReservedWidgetTarget(target string) bool {
	_, ok := reservedWidgetTargets[target]
	return ok
}

// IsImportableWidgetTarget reports whether a reserved target survives import.
// Every listing in today's inventory does — the importer knows all eight —
// but the two questions stay separate, because they can come apart again the
// day a listing is added here before the importer learns it: a reserved
// target the importer does not know is the one case where a widget is
// dropped silently, so callers must reject it rather than emit it.
func IsImportableWidgetTarget(target string) bool {
	_, ok := importableWidgetTargets[target]
	return ok
}

// WireWidgetTarget returns the id to write into a link block's target for a
// widget target: the bare listing name for a reserved one, the id itself for
// anything else.
//
// The translation is not cosmetic and is the reason the rename is safe.
// common.handleLinkBlock leaves a target alone only when
// widget.IsPredefinedWidgetTargetId knows it, and that function knows the four
// bare words and nothing else — write `_set` and the link is rewritten to
// addr.MissingObject and then stripped along with its wrapper, losing the
// widget with no error. A reserved-but-not-importable target has no wire
// spelling at all, and is returned unchanged so it fails loudly rather than
// impersonating an id; CheckIndexTargets refuses those before conversion.
func WireWidgetTarget(target string) string {
	if wire, ok := importableWidgetTargets[target]; ok {
		return wire
	}
	return target
}

// FormatWidgetTarget is WireWidgetTarget's inverse: the format's `_`-prefixed
// spelling for a wire listing name, the id itself for anything else. It is
// what a lift out of a stored widget object applies to every target-shaped
// value — the link targets and the auto-widget list alike — so the stored
// `bin` becomes the `_bin` no bundle object may claim (§1).
func FormatWidgetTarget(wire string) string {
	if format, ok := wireWidgetTargetsByWire[wire]; ok {
		return format
	}
	return wire
}

// WireHomepage returns the value pb.Profile.SpaceDashboardId should carry for
// a homepage: the bare screen name for a reserved one, the object id
// otherwise. builtinobjects.setWorkspaceSettings matches those bare names
// before it tries to resolve an id, so an untranslated `_graph` would be
// looked up as an object, fail, and fall back to the widgets screen.
func FormatHomepage(wire string) string {
	for format, w := range wireHomepages {
		if w == wire {
			return format
		}
	}
	return wire
}

func WireHomepage(homepage string) string {
	if wire, ok := wireHomepages[homepage]; ok {
		return wire
	}
	return homepage
}

// IsReservedBundleId reports whether id is one no object in a bundle may
// claim.
//
// Two populations, and the second is the one that is easy to miss. The `_`
// namespace is the format's own guarantee (§1). But the importer's spellings
// are BARE — WireWidgetTarget translates `_set` to `set` on the way out — and
// common.handleLinkBlock resolves a link target through the bundle's id map
// before it asks widget.IsPredefinedWidgetTargetId. So an object with id `set`
// captures the translated widget exactly the way it captured the untranslated
// one: renaming the format's spelling moves the collision downstream rather
// than removing it, unless the wire spellings are unmintable too.
//
// The homepages are the same story with the precedence reversed:
// setWorkspaceSettings matches `graph` before resolving an id, so a bundle
// object with that id is simply unreachable as a homepage.
func IsReservedBundleId(id string) bool {
	if IsPlatformId(id) {
		return true
	}
	for _, wire := range importableWidgetTargets {
		if id == wire {
			return true
		}
	}
	for _, wire := range wireHomepages {
		if id == wire {
			return true
		}
	}
	return false
}

// ReservedWidgetTargets lists every reserved listing, sorted, so a diagnostic
// can name the inventory instead of asking the author to guess it.
func ReservedWidgetTargets() []string {
	out := make([]string, 0, len(reservedWidgetTargets))
	for t := range reservedWidgetTargets {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// IsReservedHomepage reports whether homepage names a built-in screen rather
// than an object in the bundle.
func IsReservedHomepage(homepage string) bool {
	_, ok := wireHomepages[homepage]
	return ok
}

// Widget is one sidebar widget (§2c) — flat, though the wire carries it as
// two blocks. A stored sidebar is a widget WRAPPER block with an indented
// LINK child naming the target, and measured over 77 real spaces the pairing
// is perfectly regular: 218 wrapper blocks, 218 link children, nothing else.
// The pair carries no information beyond its members, so the format does not
// ask an author to build block scaffolding to get a sidebar.
//
// The members are the two blocks' §5 members, verbatim: `layout`, `limit`,
// `view_id` and `auto_added` are the wrapper's, and `card_style`,
// `icon_size`, `description` and `properties` are the link's own display
// members, with the link's `object_id` renamed `target` because here it may
// also name a reserved listing.
type Widget struct {
	Target    string `json:"target"`
	Layout    string `json:"layout"`
	Limit     int32  `json:"limit"`
	ViewId    string `json:"view_id"`
	AutoAdded bool   `json:"auto_added"`
	// the link child's display members (§5): how the widget's row renders.
	CardStyle   string `json:"card_style"`
	IconSize    string `json:"icon_size"`
	Description string `json:"description"`
	// Properties are the property keys shown on the widget's card, held as
	// STORED keys the way the manifest holds type keys: the index has no
	// per-document legend, so the spelling must be a pure function of the
	// key, and the dictionary's own spelling pair is that function (§2f).
	Properties []string `json:"properties"`
}

// Manifest says where to find what a reader must resolve by key or id
// rather than by walking (§2c): the format defines no folder layout, and an
// object names its type by spelling alone, so without one a reader resolves
// a type by scanning every document for a matching key. Types are keyed by
// STORED type key and options by option object id — the two spellings that
// survive a rename — and Properties points at the dictionary (§2f), which
// answers for stored property keys the same way. Paths are relative to the
// index file.
type Manifest struct {
	Types      map[string]string `json:"types"`
	Options    map[string]string `json:"options"`
	Properties string            `json:"properties"`
}

// empty reports whether the manifest locates nothing — the shape setNonEmpty
// cannot judge for a struct.
func (m *Manifest) empty() bool {
	return m == nil || (len(m.Types) == 0 && len(m.Options) == 0 && m.Properties == "")
}

// Index is a bundle's index.json (§2c).
type Index struct {
	Schema      string `json:"$schema"`
	Version     int    `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Icon is the space's icon in the typed shape every icon in this format
	// has (§2b), restricted to the two kinds a bundle can hold: an emoji, or
	// the object id of an image IN THE BUNDLE. Two flat keys used to stand
	// here, with no rule for which one wins and with `icon_image` spelled as
	// a scalar while the object surface spelled it as a list — one concept,
	// two conventions, in one format.
	//
	// The installer resolves the space icon by image *name*
	// (builtinobjects.getNewAvatarId queries name + image layout), so the
	// wiring looks the name up from this id; that asymmetry is the wire
	// format's, not the author's. An image needs the image object and its
	// file in the archive, which is why a generated bundle uses an emoji.
	Icon *Icon `json:"icon"`
	// Entrypoint is the object opened once, right after the space is created
	// — the first thing a user ever sees. Distinct from Homepage, which is
	// what opens on every later entry, and deliberately not the widget order:
	// the wire format carries the entry point as widgets[0]
	// (builtinobjects.inject), but making authors express it by sorting a list
	// means reordering the sidebar silently changes what opens.
	Entrypoint string   `json:"entrypoint"`
	Homepage   string   `json:"homepage"`
	Widgets    []Widget `json:"widgets"`
	// AutoWidgetTargets are the targets the client has already auto-added a
	// widget for — its ledger for not re-adding one the user then deleted.
	// Space state, not widget state: an entry usually names a widget that is
	// NOT in the sidebar any more, which is the whole point of the ledger.
	// Spelled like widget targets (a reserved listing or an object id),
	// because that is what the entries are. 21 of 77 real spaces carry one.
	AutoWidgetTargets []string `json:"auto_widget_targets"`
	// AutoWidgetDisabled records that the user turned automatic widgets off
	// for this space entirely. 2 of 77 real spaces.
	AutoWidgetDisabled bool `json:"auto_widget_disabled"`
	// Manifest locates types, options and the property dictionary without a
	// folder convention (§2c). Optional: a bundle without one is walked the
	// way every bundle was before it existed.
	Manifest *Manifest `json:"manifest"`
}

// EntryPoint returns the entry point the bundle *declares*: the entrypoint
// field, or for a bundle written before it existed, the first widget naming an
// object.
//
// TEMPORARY: this is intent, not behaviour. pb.Profile has no field for an
// entry point — builtinobjects.inject opens widgets[0].targetObjectId — so
// until the profile handling grows one, what actually opens is
// EffectiveEntryPoint. The two differ exactly when a bundle declares an
// entrypoint that is not its first widget, which is worth reporting.
func (i *Index) EntryPoint() string {
	if i.Entrypoint != "" {
		return i.Entrypoint
	}
	for _, w := range i.Widgets {
		if !IsReservedWidgetTarget(w.Target) {
			return w.Target
		}
	}
	return ""
}

// EffectiveEntryPoint returns what the installer opens *today*: the first
// widget naming an object, which is all pb.Profile can express. Compare with
// EntryPoint to detect a declared entry point that will not be honoured yet.
func (i *Index) EffectiveEntryPoint() string {
	for _, w := range i.Widgets {
		if !IsReservedWidgetTarget(w.Target) {
			return w.Target
		}
	}
	return ""
}

// SpaceHomepage returns what opens on entering the space: the declared
// homepage, else the entry point. Only an explicit reserved value gives up a
// real page — omitting homepage does not.
func (i *Index) SpaceHomepage() string {
	if i.Homepage != "" {
		return i.Homepage
	}
	return i.EntryPoint()
}

var compileIndexSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(indexSchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("decode embedded index schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	// the object schema is added alongside, because the index's `icon` is a
	// $ref into it (§2b): one definition of the icon shape for both surfaces,
	// rather than a copy in each that drifts. Both files are published at
	// these URLs, so an external validator resolves the same reference.
	objectDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("decode embedded schema: %w", err)
	}
	if err := c.AddResource(SchemaURL, objectDoc); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	if err := c.AddResource(IndexSchemaURL, doc); err != nil {
		return nil, fmt.Errorf("add index schema resource: %w", err)
	}
	sch, err := c.Compile(IndexSchemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile index schema: %w", err)
	}
	return sch, nil
})

// UnmarshalIndex validates data against the index schema and decodes it
// (§2c). Errors wrap *ValidationError with path-addressed issues, like
// Unmarshal.
//
// Whether the ids it names exist is a cross-document question the wiring
// answers, not this package: an index is valid on its own terms while
// pointing at an object no document defines.
func UnmarshalIndex(data []byte) (*Index, error) {
	raw, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, &ValidationError{Issues: []Issue{{Message: fmt.Sprintf("invalid JSON: %v", err)}}}
	}
	doc, ok := raw.(map[string]any)
	if !ok {
		return nil, &ValidationError{Issues: []Issue{{Message: "index must be a JSON object"}}}
	}
	// An index shares the format version and its rules with object documents
	// (§10): gate on it here, before the schema can turn a newer version into
	// a generic "value must be 1" that says nothing about why.
	if err := checkVersion(doc); err != nil {
		return nil, err
	}
	if issues := misroutedIssues(data, KindIndex); len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}
	// The `_` namespace is checked here, ahead of the schema, for the same
	// reason the version is: the schema states the rule machine-readably (a
	// pattern and an enum, so a generator reading the schema obeys it), but
	// its failure is an anonymous "does not match ^[^_]" that tells an author
	// nothing about which namespace they walked into or what to do about it.
	if issues := platformNameIssues(doc); len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}
	// the typed icon's discriminator, worded the same way it is on an object
	// document: `required` can say `format` is missing but not that it is a
	// CHOICE, and naming the alternatives is the whole point of typing it
	if issue, missing := missingFormatIssue("/icon", "a space icon", "plainIcon", doc["icon"]); missing {
		return nil, &ValidationError{Issues: []Issue{issue}}
	}
	// MIGRATION SEAM: an older version is migrated forward here, between the
	// version gate and schema validation. The schema pins the version to a
	// const, so it doubles as the assertion that migration ran (§10).
	sch, err := compileIndexSchema()
	if err != nil {
		return nil, fmt.Errorf("embedded index schema: %w", err)
	}
	if err := sch.Validate(raw); err != nil {
		return nil, &ValidationError{Issues: schemaIssues(err, keySlotReport{})}
	}

	var idx Index
	if err := jsonUnmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	// the manifest's type keys arrive in the format's spelling and are held
	// as STORED keys, the way the dictionary holds its property keys: the
	// wire says `chat_derived`, the codec says `chatDerived`, and a caller
	// looking a type up by stored key finds it (§2c).
	if idx.Manifest != nil {
		idx.Manifest.Types = reKeyed(idx.Manifest.Types, StoredTypeKey)
	}
	// a widget's shown properties follow the same rule: spelled on the wire,
	// held as STORED keys, resolved by the ladder every key slot uses. An
	// ambiguous spelling stays verbatim — the index is display state, and
	// which property a fold means is the tooling's cross-document question.
	for i := range idx.Widgets {
		idx.Widgets[i].Properties = mapStrings(idx.Widgets[i].Properties, func(s string) string {
			stored, _ := dictionaryStoredKey(s)
			return stored
		})
	}
	return &idx, nil
}

// platformNameIssues reports index members that walk into the reserved `_`
// namespace: an entry point or homepage that is not a screen the platform
// provides, and a widget target spelled like a reserved listing that is not
// one of the six.
//
// Without it a typo in the namespace lands in the worst possible place. A
// widget target the reader treats as an object id resolves to nothing, and an
// unresolvable link target becomes addr.MissingObject, after which
// WidgetObject.Init strips the link and its wrapper — the widget is gone with
// no error. Saying "unknown reserved listing, here is the inventory" points at
// the repair; "no object with that id in the bundle" points away from it.
func platformNameIssues(doc map[string]any) []Issue {
	var issues []Issue
	str := func(key string) string {
		v, _ := doc[key].(string)
		return v
	}
	if e := str("entrypoint"); IsPlatformId(e) {
		issues = append(issues, Issue{
			Path: "/entrypoint",
			Message: fmt.Sprintf("%q begins with %q, which is the platform's own address space (§1): "+
				"an entry point must be an object id from this bundle, and no built-in screen can be one",
				e, PlatformPrefix),
		})
	}
	if h := str("homepage"); IsPlatformId(h) && !IsReservedHomepage(h) {
		issues = append(issues, Issue{
			Path: "/homepage",
			Message: fmt.Sprintf("%q begins with %q, which is the platform's own address space (§1): "+
				"the only reserved homepages are %q and %q, and an object id from this bundle may not begin with %q",
				h, PlatformPrefix, HomepageWidgets, HomepageGraph, PlatformPrefix),
		})
	}
	widgets, _ := doc["widgets"].([]any)
	for i, raw := range widgets {
		w, _ := raw.(map[string]any)
		target, _ := w["target"].(string)
		if !IsPlatformId(target) || IsReservedWidgetTarget(target) {
			continue
		}
		issues = append(issues, Issue{
			Path: fmt.Sprintf("/widgets/%d/target", i),
			Message: fmt.Sprintf("%q is not a reserved listing; the whole inventory is %s. "+
				"An object id from this bundle may not begin with %q, so this target names nothing "+
				"— and a widget target that resolves to nothing is dropped on install without an error",
				target, strings.Join(ReservedWidgetTargets(), ", "), PlatformPrefix),
		})
	}
	// the auto-widget ledger's entries are target-shaped and get the same
	// diagnostic: a `_`-typo there would otherwise read as an object id that
	// names nothing
	autos, _ := doc["auto_widget_targets"].([]any)
	for i, raw := range autos {
		target, _ := raw.(string)
		if !IsPlatformId(target) || IsReservedWidgetTarget(target) {
			continue
		}
		issues = append(issues, Issue{
			Path: fmt.Sprintf("/auto_widget_targets/%d", i),
			Message: fmt.Sprintf("%q is not a reserved listing; the whole inventory is %s. "+
				"An object id from this bundle may not begin with %q, so this entry names nothing",
				target, strings.Join(ReservedWidgetTargets(), ", "), PlatformPrefix),
		})
	}
	return issues
}

// indexIconOmap renders a bundle index's icon in canonical form (§2c).
//
// It is `iconOmap`, the object surface's own renderer, and deliberately not a
// second one: the index used to render its own two variants, and a space
// whose icon is a bare COLOUR would have had it silently dropped on the way
// into the index. Measured over 77 real spaces: 55 resolve to an image, and
// their colour rides along on the file variant, which the narrow shape did
// allow — but 20 have the colour and nothing else, and those are the letter
// avatars the client actually draws. The index now admits the full `icon`
// shape, which is also what its schema $refs.
func indexIconOmap(ic *Icon) *omap {
	return iconOmap(ic)
}

// IconImageId is the object id of an index's image icon, or "" when the index
// has no icon or an emoji one. The bundle wiring resolves that id to an image
// NAME, which is what the installer actually takes.
func (i *Index) IconImageId() string {
	if i == nil || i.Icon == nil || i.Icon.Format != "file" {
		return ""
	}
	return i.Icon.File
}

// reKeyed re-spells a manifest map's keys, keeping its values.
func reKeyed(in map[string]string, spell func(string) string) map[string]string {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[spell(k)] = v
	}
	return out
}

// MarshalIndex renders an index in the canonical byte form (§4).
func MarshalIndex(idx *Index) ([]byte, error) {
	if idx == nil {
		return nil, fmt.Errorf("nil index")
	}
	doc := &omap{}
	doc.set("$schema", IndexSchemaURL)
	doc.set("version", FormatVersion)
	doc.setNonEmpty("name", idx.Name)
	doc.setNonEmpty("description", idx.Description)
	doc.setNonEmpty("icon", indexIconOmap(idx.Icon))
	doc.setNonEmpty("entrypoint", idx.Entrypoint)
	doc.setNonEmpty("homepage", idx.Homepage)

	var widgets []any
	for _, w := range idx.Widgets {
		if w.Target == "" {
			continue
		}
		wm := &omap{}
		wm.set("target", w.Target)
		// the §4 omit-empty canon, member by member: `link` is the wrapper's
		// default layout, and the three link display defaults are the same
		// ones the link BLOCK omits (§5) — `text`, `none`, `none`
		if w.Layout != "" && w.Layout != "link" {
			wm.set("layout", w.Layout)
		}
		wm.setNonEmpty("limit", w.Limit)
		wm.setNonEmpty("view_id", w.ViewId)
		wm.setNonEmpty("auto_added", w.AutoAdded)
		if w.CardStyle != "" && w.CardStyle != "text" {
			wm.set("card_style", w.CardStyle)
		}
		if w.IconSize != "" && w.IconSize != "none" {
			wm.set("icon_size", w.IconSize)
		}
		if w.Description != "" && w.Description != "none" {
			wm.set("description", w.Description)
		}
		// spelled the way the dictionary spells a stored key (§2f): the
		// index has no legend, so the spelling is a pure function of the key
		wm.setNonEmpty("properties", stringsToAny(mapStrings(w.Properties, dictionaryKeySpelling)))
		widgets = append(widgets, wm)
	}
	doc.setNonEmpty("widgets", widgets)
	doc.setNonEmpty("auto_widget_targets", stringsToAny(idx.AutoWidgetTargets))
	doc.setNonEmpty("auto_widget_disabled", idx.AutoWidgetDisabled)
	if !idx.Manifest.empty() {
		m := &omap{}
		// the manifest keys types the way the dictionary keys properties and
		// the way a type document spells a target type: one spelling per
		// concept (§2c, §2f). It carried `chatDerived`, `objectType`,
		// `relationOption` and `spaceView` — 308 camelCase keys across 77
		// bundles — while the documents beside it said `chat_derived`.
		m.setNonEmpty("types", sortedStringOmap(reKeyed(idx.Manifest.Types, TypeKeySpelling)))
		m.setNonEmpty("options", sortedStringOmap(idx.Manifest.Options))
		m.setNonEmpty("properties", idx.Manifest.Properties)
		doc.setNonEmpty("manifest", m)
	}
	return marshalCanonical(doc)
}

// sortedStringOmap renders a string map with sorted keys — the canonical
// order for the manifest's two lookup tables (§4), or nil when there is
// nothing to render.
func sortedStringOmap(m map[string]string) *omap {
	if len(m) == 0 {
		return nil
	}
	out := &omap{}
	for _, k := range sortedStringKeys(m) {
		out.set(k, m[k])
	}
	return out
}
