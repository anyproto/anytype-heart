package anyblockjson

// iconcover.go implements §2b: the typed `icon` and `cover` envelope fields.
//
// Nine hidden stored keys — iconEmoji, iconImage, iconName, iconOption,
// coverId, coverType, coverScale, coverX, coverY — used to sit in
// `properties` as nine independent slots, none of which said which of the
// others it excluded. Over 36 966 real objects that produced 22 distinct flat
// key-sets for what is really one choice with eight shapes, and one
// undecodable pair: `"cover_id": "blue"` is a COLOUR under `cover_type: 2`
// and a GRADIENT under `cover_type: 3`, and both occur.
//
// The two fields collapse each family into one object whose `format` member
// selects the variant. They live in the ENVELOPE rather than in `properties`
// for four reasons that are forced rather than aesthetic: `cover` is already
// a stored property key in real data (30 documents, plus 66 spelling it
// `pageCover`), a `properties` member can be rebound by the `property_internal_keys`
// legend to point at an arbitrary relation, `properties` carries
// presence-is-meaningful (§3) while the envelope omits empties (§4), and an
// envelope member has a schema node of its own to annotate.

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
)

// The nine stored detail keys the typed envelope fields carry. Named off the
// bundle so a rename there is a compile error here rather than a silent
// un-lift.
var (
	detailKeyIconEmoji  = bundle.RelationKeyIconEmoji.String()
	detailKeyIconImage  = bundle.RelationKeyIconImage.String()
	detailKeyIconName   = bundle.RelationKeyIconName.String()
	detailKeyIconOption = bundle.RelationKeyIconOption.String()
	detailKeyCoverId    = bundle.RelationKeyCoverId.String()
	detailKeyCoverType  = bundle.RelationKeyCoverType.String()
	detailKeyCoverScale = bundle.RelationKeyCoverScale.String()
	detailKeyCoverX     = bundle.RelationKeyCoverX.String()
	detailKeyCoverY     = bundle.RelationKeyCoverY.String()
)

// liftedDetailKeys is the icon/cover lift list, and it is the single source of
// truth for both directions: export writes these keys nowhere but the typed
// envelope fields, and import refuses them in `properties`
// (deniedPropertyKey reads this same set, §2b, §3). The precedent is
// `type_properties` (§2a); the difference is that this list is ALWAYS on,
// because the fields it feeds are a pure function of the details bag and need
// no resolver.
//
// Deriving the refusal from the list is the point — a restated list is how
// the export and import surfaces drifted apart the last time (see
// strippedDetailKeys).
func liftedDetailKeys() map[string]bool {
	return map[string]bool{
		detailKeyIconEmoji:  true,
		detailKeyIconImage:  true,
		detailKeyIconName:   true,
		detailKeyIconOption: true,
		detailKeyCoverId:    true,
		detailKeyCoverType:  true,
		detailKeyCoverScale: true,
		detailKeyCoverX:     true,
		detailKeyCoverY:     true,
	}
}

// liftedKeyRepair names the envelope field a refused flat spelling belongs in,
// and shows the shape. deniedPropertyKey's other messages say what is wrong;
// this one says what to write instead, because unlike an internal key there
// IS something to write instead.
func liftedKeyRepair(key string) string {
	switch key {
	case detailKeyIconEmoji:
		return `"icon": {"format": "emoji", "emoji": "…"}`
	case detailKeyIconImage:
		return `"icon": {"format": "file", "file": "<object id>"}`
	case detailKeyIconName:
		return `"icon": {"format": "icon", "name": "…"}`
	case detailKeyIconOption:
		return `the "color" member of "icon"`
	case detailKeyCoverId, detailKeyCoverType:
		return `"cover": {"format": "image"|"color"|"gradient", …}`
	case detailKeyCoverScale, detailKeyCoverX, detailKeyCoverY:
		return `the "scale"/"x"/"y" members of an image "cover"`
	}
	return ""
}

//
// ---- values ----
//

// maxObjectRefLen and maxOpaqueNameLen mirror the schema's `objectRef` and
// `opaqueName` bounds. Export admits before it writes, so Marshal never emits
// a value its own Validate rejects (§11, I1) — which is the whole reason the
// 33 leaked filesystem paths in the corpus are dropped rather than carried.
const (
	maxObjectRefLen  = 255
	maxOpaqueNameLen = 64
)

// isObjectRef reports whether a stored value can be written where the schema
// wants an object reference. The deny rule is `^[^/]+$`, not a URL-scheme
// lookahead: the compiler runs Go's RE2, which has no lookahead, and a slash
// is what every unwritable value in 36 966 real objects has in common — 33
// absolute filesystem paths a Notion import left in `coverId`, and nothing
// else. A URL cannot be written either way, which is the layering rule (§2b):
// the format holds only what the store holds, and a URL is resolved to a file
// object id by a layer above before it reaches here.
func isObjectRef(s string) bool {
	return s != "" && utf8.RuneCountInString(s) <= maxObjectRefLen && !strings.Contains(s, "/")
}

// isOpaqueName reports whether a value can be written where the schema wants a
// name from a vocabulary this format does not enumerate — an icon name, a
// cover colour, a gradient.
func isOpaqueName(s string) bool {
	return s != "" && utf8.RuneCountInString(s) <= maxOpaqueNameLen && !strings.Contains(s, "/")
}

// iconColorNames is the palette, in the canonical order the stored
// `iconOption` numbers index: `iconOption: n` is `palette[n-1]`. The mapping
// is total and free — apimodel.IconOptionToColor maps 1..10 onto
// constant.OptionColors() positionally, and §2a already mandates the same
// palette for select options, so the format adopts one colour vocabulary
// rather than minting a second.
func iconColorNames() []string {
	colors := constant.OptionColors()
	out := make([]string, 0, len(colors))
	for _, c := range colors {
		out = append(out, c.String())
	}
	return out
}

// iconColorValue renders a stored `iconOption` as the schema's `iconColor`: a
// palette name, or the raw number for a value the palette has no name for.
// ok is false when there is no colour at all — `iconOption: 0` is the proto
// zero, not the first colour, and treating it as one would invent a grey icon
// on 145 real objects.
//
// The integer escape is not decoration: two generators in this repo disagree
// about the range (`rand.Intn(16)+1` in the pb importer,`rand.Intn(10)+1` in
// the markdown one), so 12/13/15 exist in real data. It is the same device
// §3 already uses for a layout number outside the enum.
func iconColorValue(v *types.Value) (any, bool) {
	if v == nil {
		return nil, false
	}
	n := v.GetNumberValue()
	if n != math.Trunc(n) || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil, false
	}
	i := int64(n)
	if i < 1 {
		return nil, false
	}
	names := iconColorNames()
	if i <= int64(len(names)) {
		return names[i-1], true
	}
	return i, true
}

// iconOptionOf inverts iconColorValue: the stored number a written colour
// stands for. A name outside the palette cannot arrive — the schema refuses
// it — so the fallback is unreachable rather than lenient.
func iconOptionOf(color any) (float64, bool) {
	switch c := color.(type) {
	case string:
		for i, name := range iconColorNames() {
			if name == c {
				return float64(i + 1), true
			}
		}
	case float64:
		if c >= 1 {
			return c, true
		}
	case int64:
		if c >= 1 {
			return float64(c), true
		}
	}
	return 0, false
}

// coverSourceNames maps the stored `coverType` numbers that all mean "an
// image" onto the provenance the typed field carries. The relation's own
// bundled description is the union written as prose: "1-image, 2-color,
// 3-gradient, 4-prebuilt bg image, 5-unsplash image".
//
// One `image` branch with an output-only `source`, rather than three
// branches: a generator that just uploaded an image has no basis to choose
// between them, and choosing `unsplash` writes a permanent false provenance
// claim into cold storage.
const (
	coverTypeNone     = 0
	coverTypeImage    = 1
	coverTypeColor    = 2
	coverTypeGradient = 3
	coverTypePrebuilt = 4
	coverTypeUnsplash = 5

	coverSourceUnsplash = "unsplash"
	coverSourcePrebuilt = "prebuilt"
)

//
// ---- export ----
//

// iconField and coverField are the memoized typed envelope fields. They are
// built once and read twice — the id census (buildLabelPlan) needs the object
// ids they write, and buildDoc needs the objects themselves — so building on
// demand would report every warning twice under compaction.
func (e *exporter) iconField() *omap {
	if !e.iconBuilt {
		e.icon = e.buildIcon()
		e.iconBuilt = true
	}
	return e.icon
}

func (e *exporter) coverField() *omap {
	if !e.coverBuilt {
		e.cover = e.buildCover()
		e.coverBuilt = true
	}
	return e.cover
}

// buildIcon renders the four icon channels as one typed field (§2b), or nil
// when the object has no icon.
//
// The precedence is `iconName` → `iconEmoji` → `iconImage`, which is
// core/api/service/icon.go's rule and the only precedence implementation in
// heart — everywhere else (the dot, graphjson and publish converters) emits
// every channel and lets the consumer decide. `iconOption` is NOT a fourth
// step in that chain: it is orthogonal, and attaches as `color` to whichever
// channel won, standing alone only when none did (87 real objects attach a
// colour to something other than a named icon, and 29 carry a colour with no
// source at all).
//
// A source whose stored value is EMPTY is not a source. That is the one place
// this field overrides §3's presence-is-meaningful rule, and the carve-out is
// principled rather than convenient: all nine relations are `hidden: true`,
// so there is no property row for presence to be meaningful to. It is what
// deletes the format's largest class of fake ambiguity — 883 real objects
// carry both `iconEmoji` and `iconImage`, and in not one of them are both
// non-empty.
func (e *exporter) buildIcon() *omap {
	if e.snapshot == nil || e.snapshot.Details == nil {
		return nil
	}
	return iconOmap(iconOf(e.detail, e.warn))
}

// iconOf chooses the icon from the four stored channels (§2b), or returns nil
// when the object has no icon. It is the ONE implementation of the precedence
// described above: the object surface renders what it returns, and so does a
// bundle index (§2c), which is what keeps the space icon and the object icon
// from being two conventions for one concept.
//
// It reports through `warn` exactly where a stored value cannot be carried,
// and a caller that cannot afford to lose one — the index, which omits the
// document it read the icon from — treats any warning as a refusal.
func iconOf(detail func(string) *types.Value, warn func(path, format string, args ...any)) *Icon {
	color, hasColor := iconColorValue(detail(detailKeyIconOption))
	ic := &Icon{}
	if hasColor {
		ic.Color = color
	}

	if name := detail(detailKeyIconName).GetStringValue(); name != "" {
		if !isOpaqueName(name) {
			warn("/icon", "icon name %q cannot be written in this format and is dropped", name)
		} else {
			ic.Format = "icon"
			ic.Name = name
			// the conflict carry-over: 200 real objects — every one a bundled
			// type mid-migration from an emoji to a named icon — hold BOTH.
			// `format` has already answered which icon wins, so the emoji is
			// baggage rather than ambiguity, and a cold-storage backup format
			// that silently deletes a non-empty stored value on export is
			// disqualifying. It is annotated x-output-only: a document that
			// supplies it is not choosing an icon.
			if emoji := detail(detailKeyIconEmoji).GetStringValue(); emoji != "" {
				ic.Emoji = emoji
				warn("/icon", "this object holds both a named icon (%q) and an emoji (%q); "+
					"the name wins and the emoji is carried as output-only baggage", name, emoji)
			}
			return ic
		}
	}
	if emoji := detail(detailKeyIconEmoji).GetStringValue(); emoji != "" {
		ic.Format = "emoji"
		ic.Emoji = emoji
		return ic
	}
	if images := valueStringList(detail(detailKeyIconImage)); len(images) > 0 {
		if len(images) > 1 {
			warn("/icon", "the icon image list holds %d entries; only the first is an icon", len(images))
		}
		if !isObjectRef(images[0]) {
			// there is no way to write it: the schema's objectRef refuses a
			// URL and a filesystem path, so carrying it would make Marshal
			// emit what its own Validate rejects (§11, I1)
			warn("/icon", "icon image %q is not an object id and is dropped — "+
				"this format holds a reference to an image object, never a URL or a path", images[0])
		} else {
			ic.Format = "file"
			ic.File = images[0]
			return ic
		}
	}
	if hasColor {
		// a colour with no source: the letter-avatar background. 29 real
		// objects carry one, and the API reports every one of them as having
		// no icon at all.
		ic.Format = "color"
		return ic
	}
	return nil
}

// iconOmap renders a chosen icon as the format's typed `icon` field. It is
// the one renderer, shared by the object surface and the bundle index, so the
// two cannot drift into different spellings of the same icon.
//
// `format` is the discriminator and comes first; the colour attaches to
// whichever channel won; the named-icon variant carries its emoji last, as
// output-only baggage.
func iconOmap(ic *Icon) *omap {
	if ic == nil {
		return nil
	}
	format := ic.Format
	if format == "" {
		// a caller that filled the channel without naming the variant
		switch {
		case ic.Name != "":
			format = "icon"
		case ic.Emoji != "":
			format = "emoji"
		case ic.File != "":
			format = "file"
		case ic.Color != nil:
			format = "color"
		default:
			return nil
		}
	}
	m := &omap{}
	m.set("format", format)
	switch format {
	case "icon":
		m.set("name", ic.Name)
	case "emoji":
		m.set("emoji", ic.Emoji)
	case "file":
		m.set("file", ic.File)
	}
	if ic.Color != nil {
		m.set("color", ic.Color)
	}
	if format == "icon" && ic.Emoji != "" {
		m.set("emoji", ic.Emoji)
	}
	return m
}

// buildCover renders the five cover channels as one typed field (§2b), or nil
// when the object has no cover.
//
// `coverType` is the discriminator and it is provably load-bearing: `"blue"`
// occurs in the corpus as `cover_type: 2` (a colour) AND as `cover_type: 3`
// (a gradient). `cover_id` alone is undecodable, so this pair cannot be
// simplified any other way — only typed.
func (e *exporter) buildCover() *omap {
	if e.snapshot == nil || e.snapshot.Details == nil {
		return nil
	}
	id := e.detail(detailKeyCoverId).GetStringValue()
	raw := e.detail(detailKeyCoverType).GetNumberValue()
	t := int(raw)
	if raw != math.Trunc(raw) || t < coverTypeNone || t > coverTypeUnsplash {
		e.warn("/cover", "cover type %v is not one of 0..5 (1-image, 2-color, 3-gradient, "+
			"4-prebuilt, 5-unsplash); the cover is dropped", raw)
		return nil
	}
	if t == coverTypeNone {
		if id != "" {
			e.warn("/cover", "cover %q has no cover type, so nothing says how to read it; it is dropped", id)
		}
		return nil
	}
	if id == "" {
		e.warn("/cover", "cover type %d has no cover id to go with it; the cover is dropped", t)
		return nil
	}

	m := &omap{}
	switch t {
	case coverTypeColor, coverTypeGradient:
		if !isOpaqueName(id) {
			e.warn("/cover", "cover %q cannot be written as a name in this format and is dropped", id)
			return nil
		}
		if t == coverTypeColor {
			m.set("format", "color")
			m.set("color", id)
		} else {
			m.set("format", "gradient")
			m.set("gradient", id)
		}
		return m
	}

	if !isObjectRef(id) {
		// 33 real objects reach here, every one an absolute path into a
		// long-gone temp directory that core/block/import/notion wrote into
		// coverId as if it were a file reference. The value is already dead;
		// dropping it turns permanent silent corruption into a named event.
		e.warn("/cover", "cover image %q is not an object id and is dropped — "+
			"this format holds a reference to an image object, never a URL or a path", id)
		return nil
	}
	m.set("format", "image")
	m.set("file", id)
	switch t {
	case coverTypeUnsplash:
		m.set("source", coverSourceUnsplash)
	case coverTypePrebuilt:
		m.set("source", coverSourcePrebuilt)
	}
	// framing belongs to an image and to nothing else: in 36 966 real objects
	// these three are non-zero only under cover types 1 and 5, though they
	// are PRESENT and zero on colours, gradients and cleared covers alike
	m.setNonEmpty("scale", e.detail(detailKeyCoverScale).GetNumberValue())
	m.setNonEmpty("x", e.detail(detailKeyCoverX).GetNumberValue())
	m.setNonEmpty("y", e.detail(detailKeyCoverY).GetNumberValue())
	return m
}

// calloutIcon renders a callout block's two icon attributes as the same typed
// shape the envelope field uses, restricted to `emoji` and `file` (§5.2).
//
// Emoji beats image, the object rule's order minus the two channels a block
// has no room for. In 36 966 real objects there are 650 callouts — 256 with
// an emoji, 2 with an image, and NONE with both — so the precedence is
// unexercised, and the warning below says so if that ever changes.
func (e *exporter) calloutIcon(t *model.BlockContentText) *omap {
	m := &omap{}
	if t.IconEmoji != "" {
		if t.IconImage != "" {
			e.warn("/blocks", "callout %s holds both an emoji and an image icon; the emoji wins "+
				"and the image is dropped", t.IconImage)
		}
		m.set("format", "emoji")
		m.set("emoji", t.IconEmoji)
		return m
	}
	if t.IconImage == "" {
		return nil
	}
	if !isObjectRef(t.IconImage) {
		e.warn("/blocks", "callout icon %q is not an object id and is dropped — this format holds "+
			"a reference to an image object, never a URL or a path", t.IconImage)
		return nil
	}
	m.set("format", "file")
	m.set("file", t.IconImage)
	return m
}

// calloutIconFrom inverts calloutIcon.
func calloutIconFrom(ic *Icon, t *model.BlockContentText) {
	if ic == nil {
		return
	}
	switch ic.Format {
	case "emoji":
		t.IconEmoji = ic.Emoji
	case "file":
		t.IconImage = ic.File
	}
}

// liftedObjectIds is every OBJECT id the typed envelope fields write. The id
// census needs it explicitly: those ids used to reach the avoid-set through
// the ordinary property walk (`iconImage` is a `file` relation), and the lift
// takes them out of it — while `coverId` is a `longtext` relation and was
// never in it at all, so a compact block label could always have collided
// with a file-backed cover id (§9a).
func (e *exporter) liftedObjectIds() []string {
	var out []string
	if m := e.iconField(); m != nil {
		if id, ok := omapString(m, "file"); ok {
			out = append(out, id)
		}
	}
	if m := e.coverField(); m != nil {
		if id, ok := omapString(m, "file"); ok {
			out = append(out, id)
		}
	}
	return out
}

// omapString reads a string member out of a built field.
func omapString(m *omap, key string) (string, bool) {
	for i, k := range m.keys {
		if k == key {
			s, ok := m.vals[i].(string)
			return s, ok
		}
	}
	return "", false
}

//
// ---- import ----
//

// Icon and Cover are the two typed fields (§2b), exported because they are
// the shape a caller writing a document or a bundle index has to build — and
// because the API layer needs the same union and should adopt this one rather
// than mint a second.
//
// `Color` is `any` because it is the union the schema states: one of the ten
// palette names, or a raw number for a stored value the palette has no name
// for. Everything else is typed. Exactly one variant's members are populated
// at a time; `Format` says which.
type Icon struct {
	Format string `json:"format"`
	Emoji  string `json:"emoji"`
	File   string `json:"file"`
	Name   string `json:"name"`
	Color  any    `json:"color"`
}

type Cover struct {
	Format   string  `json:"format"`
	File     string  `json:"file"`
	Source   string  `json:"source"`
	Color    string  `json:"color"`
	Gradient string  `json:"gradient"`
	Scale    float64 `json:"scale"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
}

// applyIcon writes the stored keys the typed `icon` field stands for (§2b).
// Every emitted variant inverts to exactly the details that produced it,
// which is what makes Export ∘ Import a fixpoint over this field.
//
// It cannot fail: the schema has already refused every shape that is not one
// of the four variants, so there is no case left for the reader to judge.
func (imp *importer) applyIcon(details *types.Struct) {
	ic := imp.doc.Icon
	if ic == nil {
		return
	}
	setNum := func(key string, n float64) {
		details.Fields[key] = &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
	}
	setStr := func(key, s string) {
		details.Fields[key] = &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
	}
	switch ic.Format {
	case "emoji":
		setStr(detailKeyIconEmoji, ic.Emoji)
	case "file":
		// `iconImage` is a `file` relation, so its stored shape is a list —
		// the same shape the ordinary property path writes (wrapToList), and
		// the shape all 12 011 populated cases in the corpus hold
		details.Fields[detailKeyIconImage] = &types.Value{Kind: &types.Value_ListValue{
			ListValue: &types.ListValue{Values: []*types.Value{
				{Kind: &types.Value_StringValue{StringValue: ic.File}},
			}},
		}}
	case "icon":
		setStr(detailKeyIconName, ic.Name)
		if ic.Emoji != "" {
			setStr(detailKeyIconEmoji, ic.Emoji)
		}
	}
	if n, ok := iconOptionOf(ic.Color); ok {
		setNum(detailKeyIconOption, n)
	}
}

// applyCover writes the stored keys the typed `cover` field stands for (§2b).
func (imp *importer) applyCover(details *types.Struct) {
	cv := imp.doc.Cover
	if cv == nil {
		return
	}
	setNum := func(key string, n float64) {
		details.Fields[key] = &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
	}
	setStr := func(key, s string) {
		details.Fields[key] = &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
	}
	switch cv.Format {
	case "color":
		setStr(detailKeyCoverId, cv.Color)
		setNum(detailKeyCoverType, coverTypeColor)
		return
	case "gradient":
		setStr(detailKeyCoverId, cv.Gradient)
		setNum(detailKeyCoverType, coverTypeGradient)
		return
	}
	setStr(detailKeyCoverId, cv.File)
	switch cv.Source {
	case coverSourceUnsplash:
		setNum(detailKeyCoverType, coverTypeUnsplash)
	case coverSourcePrebuilt:
		setNum(detailKeyCoverType, coverTypePrebuilt)
	default:
		setNum(detailKeyCoverType, coverTypeImage)
	}
	if cv.Scale != 0 {
		setNum(detailKeyCoverScale, cv.Scale)
	}
	if cv.X != 0 {
		setNum(detailKeyCoverX, cv.X)
	}
	if cv.Y != 0 {
		setNum(detailKeyCoverY, cv.Y)
	}
}

// LiftedPropertyKeys reports the stored property keys the typed envelope
// fields carry instead of `properties` (§2b). Exported for the same reason
// InternalPropertyKeys is: a round-trip checker comparing a snapshot with its
// re-import has to know which keys moved, or it reports a faithful export as
// data loss.
func LiftedPropertyKeys() map[string]bool {
	return liftedDetailKeys()
}

// DroppedEmptyIconCover reports a stored icon/cover value the typed fields
// treat as NO SOURCE at all — an empty string, an empty list, a zero number,
// a null (§2b, N(S)). Such a key does not survive a round trip, and that is
// the one place the typed fields override §3's presence-is-meaningful rule:
// all nine relations are `hidden: true`, so there is no property row for
// presence to be meaningful to.
//
// It exists so the round-trip comparator can suppress exactly that step and
// nothing else. In the 36 966-object corpus ~2 300 objects carry at least one
// present-but-empty icon or cover key, which is nearly twice the noise that
// buried a previous sweep (see snapshotdiff's recommendedListKeys comment) —
// and a comparator that suppressed the whole KEY instead would go blind to
// the 33 objects whose cover really is lost.
func DroppedEmptyIconCover(key string, v *types.Value) bool {
	return liftedDetailKeys()[key] && !liftedValueIsSource(v)
}

// liftedValueIsSource is the emptiness rule the export builders apply, in one
// place so the comparator and the builders cannot disagree about which values
// are sources.
func liftedValueIsSource(v *types.Value) bool {
	switch k := v.GetKind().(type) {
	case *types.Value_StringValue:
		return k.StringValue != ""
	case *types.Value_NumberValue:
		return k.NumberValue != 0
	case *types.Value_ListValue:
		return len(valueStringList(v)) > 0
	}
	return false
}

//
// ---- validation ----
//

// iconFormatIssues states the one thing the schema's own verdict cannot: what
// a typed field with no `format` should have said instead. `required:
// ["format"]` reports `missing property 'format'`, which tells an author a
// member is missing but not that it is a CHOICE, or what the choices are —
// and naming the alternatives at the moment the author is wrong is the whole
// reason the field is typed rather than flat.
//
// The alternatives are read out of the published schema rather than restated
// here, so a variant added at the extension seam (§2b) shows up in this
// message for free.
func iconFormatIssues(doc map[string]any, r *keySlotReport) {
	check := func(path, noun, def string, node any) {
		if issue, missing := missingFormatIssue(path, noun, def, node); missing {
			r.rejectValueAt(issue.Path, issue.Message)
		}
	}
	check("/icon", "an icon", "icon", doc["icon"])
	check("/cover", "a cover", "cover", doc["cover"])
	for i, raw := range blocksOf(doc) {
		b, _ := raw.(map[string]any)
		if b == nil {
			continue
		}
		check(fmt.Sprintf("/blocks/%d/icon", i), "a callout icon", "plainIcon", b["icon"])
	}
}

// missingFormatIssue is that verdict for one field. Split out because the
// bundle index carries the same typed icon and validates through a different
// entry point (UnmarshalIndex), and a second wording of one rule is how two
// surfaces of one format start disagreeing.
func missingFormatIssue(path, noun, def string, node any) (Issue, bool) {
	obj, isObject := node.(map[string]any)
	if !isObject {
		return Issue{}, false // the schema types the field; this pass only shapes it
	}
	if _, has := obj["format"]; has {
		return Issue{}, false
	}
	names := schemaFormatEnum(def)
	if len(names) == 0 {
		return Issue{}, false
	}
	return Issue{Path: path, Message: fmt.Sprintf(
		"missing property 'format': %s is one of %s (\u00a72b)", noun, quotedList(names))}, true
}

// quotedList renders the alternatives the way the schema library renders an
// enum, so one document's issues read alike whichever pass produced them.
func quotedList(names []string) string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, "'"+n+"'")
	}
	return strings.Join(out, ", ")
}
