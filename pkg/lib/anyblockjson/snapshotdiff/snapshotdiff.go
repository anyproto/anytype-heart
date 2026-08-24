// Package snapshotdiff compares two smartblock snapshots on the axes the
// AnyBlock JSON format promises to preserve: the object's TYPES, detail
// values (up to the documented normalizations) and the text content of
// non-structural text blocks (as a multiset). It is the state-diff /
// text-multiset comparator behind cmd/anyblockroundtrip and the API v2 eval
// metric (DELEGATE-52 backtranslation). Findings are triage input, not
// proof.
package snapshotdiff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// strippedKeys is the format's own internal-property set, not a copy of it.
// The copy that used to stand here fell out of date the moment the package
// added the importer's provenance keys, and every object carrying one was
// reported as data loss (§3, §11).
var strippedKeys = anyblockjson.InternalPropertyKeys()

// recommendedListKeys are the four role lists a type keeps its recommended
// properties in. typeProperties (§2a) collapses them into one labelled array,
// and import rebuilds all four from it — writing an empty list for a role
// nothing occupies. Most types have no file-role property, so the store
// usually has no recommendedFileRelations key at all and the round trip adds
// an empty one.
//
// That is a difference, and it is normalization rather than drift: an absent
// list and an empty list say the same thing, and the empty list is the only
// way the format can express a role being cleared, since typeProperties
// cannot name a section that exists with no members. Left unrecorded it
// buried the sweep — 1 344 of 1 351 differing objects in a 34 339-object
// account differed by nothing else. Whether the object state itself should
// carry all four consistently is GO-7451, not this comparator's call.
// isDroppedEmptyIconCover is the icon/cover analogue, and it is bigger: §2b
// lifted nine hidden keys into the typed `icon` and `cover` envelope fields,
// and a source whose stored value is EMPTY is not a source — so a key present
// and empty comes back absent. Roughly 2 300 objects in a 36 966-object
// account carry at least one, nearly double the recommended-list noise above,
// and left unrecorded it would bury the sweep the same way.
//
// Scoped to absent-vs-dropped-empty and nothing else: a cover that really was
// lost (33 objects hold an absolute filesystem path a Notion import left in
// coverId, which the typed field cannot write) still reports, because its
// value is not empty. The predicate is the format's own, not a copy.
func isDroppedEmptyIconCover(key string, orig, got *types.Value) bool {
	return got == nil && anyblockjson.DroppedEmptyIconCover(key, orig)
}

// isDroppedEmptySystemProperty is the third normalization of this shape, and
// the narrowest: §15 #12 admits seven system-stamped keys whose EMPTY value
// says nothing a reader could act on (`isHidden` false, `revision` 0,
// `relationMaxCount` 0, …), so export omits them and they come back absent.
// The whitelist is deliberately explicit rather than a rule over
// bundle.SystemRelations — see systemtrim.go for the admission test each key
// had to pass, and for the keys that failed it.
//
// Scoped to absent-vs-dropped-empty like its neighbours: a non-empty value
// on one of those keys still reports if it goes missing, and so does an
// empty one that came back SET. The predicate is the format's own.
func isDroppedEmptySystemProperty(key string, orig, got *types.Value) bool {
	return got == nil && anyblockjson.DroppedEmptySystemProperty(key, orig)
}

var recommendedListKeys = map[string]bool{
	bundle.RelationKeyRecommendedFeaturedRelations.String(): true,
	bundle.RelationKeyRecommendedRelations.String():         true,
	bundle.RelationKeyRecommendedFileRelations.String():     true,
	bundle.RelationKeyRecommendedHiddenRelations.String():   true,
}

// isEmptyRecommendedList reports whether an ADDED detail is one of those four
// arriving empty. A recommended list that arrives with members is a real
// difference and is still reported: this suppresses the absent-to-empty step
// only, never a list that gained content.
func isEmptyRecommendedList(key string, v *types.Value) bool {
	if !recommendedListKeys[key] {
		return false
	}
	list := v.GetListValue()
	return list != nil && len(list.Values) == 0
}

// Compare reports every place where got diverges from orig on a
// format-preserved axis, as human-readable findings. An empty result means
// no detectable drift.
//
// sbType is the snapshot's smartblock type, and it is a parameter rather than
// something read off the pair because since v0.22 it is the only thing that
// says how many type slots the envelope had: `template_for` exists exactly on
// a Template (§2). A snapshot cannot answer that question about itself — a
// template's ObjectTypes need not begin with the template key — so a caller
// that has the type must hand it over, or the diff reports a faithfully
// preserved target type as an invented one.
func Compare(orig, got *model.SmartBlockSnapshotBase, sbType model.SmartBlockType, opts anyblockjson.Options) []string {
	var out []string

	out = append(out, compareObjectTypes(orig, got, sbType)...)

	// the §2f omission: a bundled-identical relation document is not written
	// at all — its key travels in the dictionary's `installed` list and a
	// reader reconstructs it from the bundled table. Across that trip the
	// install artifacts (createdDate, origin, apiObjectKey, …) come back
	// absent, re-stamped by the next install, and a definition member the
	// copy never stored comes back as its explicit empty default. Both skips
	// below are scoped to snapshots the omission predicate itself admits —
	// the predicate is the format's own, not a copy — so on the ordinary
	// document round trip, where every key survives, neither ever fires.
	_, omittable := anyblockjson.OmittedBundledRelation(sbType, orig, opts)

	if orig.Details != nil {
		gotFields := map[string]*types.Value{}
		if got.Details != nil {
			gotFields = got.Details.Fields
		}
		keys := make([]string, 0, len(orig.Details.Fields))
		for k := range orig.Details.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if strippedKeys[k] {
				continue
			}
			if isDroppedEmptyIconCover(k, orig.Details.Fields[k], gotFields[k]) {
				continue
			}
			if isDroppedEmptySystemProperty(k, orig.Details.Fields[k], gotFields[k]) {
				continue
			}
			// a TYPE document does not carry its own install provenance
			// (§2a, v0.32): eight keys export omits there whatever their
			// value — each admitted against the corpus individually — so the
			// comparator learns the rule in the same commit that taught
			// export (the miss that produced 1,344 false failures in one
			// sweep). Scoped to absent-on-the-way-back: a got side that
			// somehow carries the key still reports. The predicate is the
			// format's own, not a copy.
			if gotFields[k] == nil && anyblockjson.DroppedTypeProvenanceKey(sbType, k) {
				continue
			}
			// the five type_settings members follow the §4 omit-empty canon
			// (§2a): a pluralName of "" or a defaultTemplateId of [] comes
			// back absent. Same scoping, same ownership of the predicate.
			if gotFields[k] == nil && anyblockjson.DroppedEmptyTypeSetting(sbType, k, orig.Details.Fields[k]) {
				continue
			}
			// an omitted relation document's install artifacts (§2f): absent
			// on the way back, re-stamped by the next install. Scoped to
			// absent-and-artifact on an omittable snapshot — a definition
			// member that goes missing still reports.
			if gotFields[k] == nil && omittable && anyblockjson.RelationInstallArtifactKey(k) {
				continue
			}
			if !detailEqual(k, orig.Details.Fields[k], gotFields[k], opts) {
				out = append(out, fmt.Sprintf("detail %q changed: %s -> %s",
					k, valuePreview(orig.Details.Fields[k]), valuePreview(gotFields[k])))
			}
		}
	}

	// added details: keys present in got but not orig. The orig-key loop
	// above already flags changed/removed (detailEqual against a nil got
	// value), but never sees keys the round trip introduced.
	if got.Details != nil {
		gotOnly := make([]string, 0)
		for k := range got.Details.Fields {
			if strippedKeys[k] {
				continue
			}
			if orig.Details != nil {
				if _, inOrig := orig.Details.Fields[k]; inOrig {
					continue
				}
			}
			gotOnly = append(gotOnly, k)
		}
		sort.Strings(gotOnly)
		for _, k := range gotOnly {
			if isEmptyRecommendedList(k, got.Details.Fields[k]) {
				continue
			}
			// the reconstruction of an omitted relation document (§2f)
			// states the WHOLE definition, so a member the original copy
			// never stored arrives as its explicit empty default —
			// `isHidden: false`, `object_types: []`. Absent and empty say
			// the same thing for a definition member with a defined
			// default; a NON-empty invented member still reports.
			if omittable && anyblockjson.InstallStampedDefault(k, got.Details.Fields[k]) {
				continue
			}
			out = append(out, fmt.Sprintf("detail %q added: %s", k, valuePreview(got.Details.Fields[k])))
		}
	}

	// Compare is intentionally order-insensitive on text (a multiset): the
	// round-trip verifier tolerates legitimate normalization reordering.
	// Order-sensitive scoring lives in the eval corruption metric via
	// TextSequence, where a backtranslation must restore exact order.
	origTexts := TextInventory(orig)
	gotTexts := TextInventory(got)
	for text, n := range origTexts {
		if gotTexts[text] < n {
			out = append(out, fmt.Sprintf("text block lost (%dx): %q", n-gotTexts[text], preview(text)))
		}
	}
	return out
}

// typeKeyIdPrefix is the "ot-" prefix an ObjectTypes entry carries.
var typeKeyIdPrefix = domain.TypeKey("").URL()

// compareObjectTypes reports divergence in the TYPE namespace — the axis
// Compare used to be structurally blind to. It read only details and text, so
// a 36 808-object production sweep could never have caught a type
// substitution: every claim about type-key correctness rested on synthetic
// tests alone. A rebinding is exactly the loss the `type_keys` legend (§3)
// exists to prevent, and exactly what a sweep must be able to see.
//
// Equality is the wrong predicate here, because export normalizes the list
// before it writes it (§2, export.envelopeTypeTerms) and every step of that is
// by design. Measured, not assumed:
//
//	["ot-page","ot-task"]               -> ["ot-page"]                (truncated)
//	["ot-template","ot-task","ot-page"] -> ["ot-template","ot-task"]  (truncated)
//	["ot-","ot-task"]                   -> ["ot-task"]                (closed ranks)
//	["ot-template","ot-","ot-task"]     -> ["ot-template","ot-task"]  (both)
//
// So the comparison applies the same two normalizations to orig — drop the
// keyless entries, then keep the modelled positions — before demanding
// position-for-position identity. That is detailEqual's shape: normalize both
// sides to what the format preserves, then compare exactly, rather than
// reporting a documented normalization as loss. Identity has to be exact
// because order and duplicates carry meaning (`[0]` is the type, `[1]` the
// template target) and both round-trip today. Anything got carries beyond the
// modelled positions is drift the other way: the round trip invented a type.
func compareObjectTypes(orig, got *model.SmartBlockSnapshotBase, sbType model.SmartBlockType) []string {
	origKeys := typeKeysOf(orig)
	gotKeys := typeKeysOf(got)
	modelled := modelledTypeSlots(origKeys, sbType)

	var out []string
	for i := 0; i < modelled; i++ {
		switch {
		case i >= len(gotKeys):
			out = append(out, fmt.Sprintf("object type [%d] lost: %q", i, origKeys[i]))
		case gotKeys[i] != origKeys[i]:
			out = append(out, fmt.Sprintf("object type [%d] changed: %q -> %q", i, origKeys[i], gotKeys[i]))
		}
	}
	for i := modelled; i < len(gotKeys); i++ {
		out = append(out, fmt.Sprintf("object type [%d] added: %q", i, gotKeys[i]))
	}
	return out
}

// typeKeysOf is export's first normalization: the stored key of each entry,
// with the keyless ones dropped and the survivors closing ranks. Trimming the
// prefix is itself a normalization — a legacy row may hold a bare key, and
// import always writes the prefixed form back. A keyless entry (`ot-`, or "")
// names no type and export drops it with a warning rather than letting it take
// its siblings with it, so it is not loss.
func typeKeysOf(s *model.SmartBlockSnapshotBase) []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.ObjectTypes))
	for _, t := range s.ObjectTypes {
		if key := strings.TrimPrefix(t, typeKeyIdPrefix); key != "" {
			out = append(out, key)
		}
	}
	return out
}

// modelledTypeSlots is how many of the surviving keys the format has a slot
// for — export.modelledTypeKeys' own two conditions (§2):
//
//   - `type` takes the first surviving key, whatever it is;
//   - `template_for` exists only on a TEMPLATE, and takes the second.
//
// The second condition used to be "only when the first key is the template
// key", mirroring what export did. Both were wrong the same way: a template
// whose object types do not begin with the template key — a shape nothing in
// the model forbids — lost its target type on export, and this diff called
// that loss correct. `kind` carries template-ness now, so the question is
// answered by the smartblock type and not by the data.
//
// There is no third slot, so anything further is dropped by design.
func modelledTypeSlots(keys []string, sbType model.SmartBlockType) int {
	if len(keys) == 0 {
		return 0
	}
	if sbType == model.SmartBlockType_Template && len(keys) > 1 {
		return 2
	}
	return 1
}

// detailEqual compares one detail value up to the documented normalizations:
// scalars of list-shaped formats become single-element lists, dates truncate
// to whole seconds.
func detailEqual(key string, a, b *types.Value, opts anyblockjson.Options) bool {
	if b == nil {
		return false
	}
	if recommendedDetailKeys[key] && opts.ResolveProperties != nil {
		return equalStrings(
			normalizeRecommended(stringsOf(a), opts.ResolveProperties),
			normalizeRecommended(stringsOf(b), opts.ResolveProperties))
	}
	// relationFormatObjectTypes round-trips by type KEY (§2d), and legacy
	// data mixes spellings the same way the recommended lists do: 27 corpus
	// relations store a bare type key where the store speaks object ids, and
	// import writes the id back — the same TYPE, in the store's own
	// spelling. So the comparison normalizes both sides to keys through the
	// TypeResolver capability and demands position-for-position identity,
	// exactly as normalizeRecommended does one namespace over: a rebound
	// TARGET still reports, a respelled one does not. Without the
	// capability the translation is verbatim both ways (§2d), so the raw
	// comparison below is already exact.
	if key == relationTargetsDetailKey {
		if tr, ok := opts.ResolveProperties.(anyblockjson.TypeResolver); ok {
			return equalStrings(normalizeTypeRefs(stringsOf(a), tr), normalizeTypeRefs(stringsOf(b), tr))
		}
	}
	format, _ := resolveFormat(key, opts)
	switch format {
	case model.RelationFormat_object, model.RelationFormat_file,
		model.RelationFormat_status, model.RelationFormat_tag:
		// mirror the format's list extraction: scalars wrap, empty strings drop
		return equalStrings(stringsOf(a), stringsOf(b))
	case model.RelationFormat_date:
		return int64(a.GetNumberValue()) == int64(b.GetNumberValue())
	}
	return proto.Equal(a, b)
}

// recommendedDetailKeys are the four lists SPEC §2a lifts into
// typeProperties. They round-trip by property KEY, and legacy data mixes ids
// and bare keys, so comparison normalizes both sides to keys and skips
// entries neither side can resolve (dropped-by-design, like missing-object
// sentinels).
var recommendedDetailKeys = map[string]bool{
	"recommendedFeaturedRelations": true,
	"recommendedRelations":         true,
	"recommendedFileRelations":     true,
	"recommendedHiddenRelations":   true,
}

// relationTargetsDetailKey is the stored key behind relation_settings'
// object_types (§2d) — the type-namespace twin of the four lists above.
// Named off the bundle rather than spelled, the §2b rule: a rename there
// is a compile error here instead of a comparator that silently stops
// normalizing the key it was written for.
var relationTargetsDetailKey = bundle.RelationKeyRelationFormatObjectTypes.String()

// normalizeTypeRefs reduces each target entry to the type KEY it names: a
// bundled url through the table, an object id through the resolver, and
// anything else — a legacy bare key included — verbatim, its own address.
// Applied to BOTH sides, so only a change of the type named survives it.
func normalizeTypeRefs(entries []string, tr anyblockjson.TypeResolver) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if key, err := bundle.TypeKeyFromUrl(entry); err == nil && key != "" {
			out = append(out, string(key))
			continue
		}
		if key, ok := tr.TypeKeyById(entry); ok && key != "" {
			out = append(out, key)
			continue
		}
		out = append(out, entry)
	}
	return out
}

func normalizeRecommended(entries []string, r anyblockjson.PropertyResolver) []string {
	var out []string
	for _, id := range entries {
		if def, ok := r.PropertyById(id); ok {
			out = append(out, string(def.Key))
			continue
		}
		if _, ok := r.PropertyId(anyblockjson.PropertyDefinition{Key: domain.RelationKey(id)}); ok {
			out = append(out, id) // already a key
			continue
		}
		if _, err := bundle.GetRelation(domain.RelationKey(id)); err == nil {
			out = append(out, id) // bundle key without a space object
		}
		// otherwise unresolvable: dropped by design on export, skip
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// missingObjectSentinel marks a dangling object reference in stored details
// (pkg/lib/localstore/addr). Export legitimately drops these unresolvable
// refs, so the comparison must not count them as loss.
const missingObjectSentinel = "_missing_object"

// stringsOf reads a value as the format's string list: single strings wrap,
// empty strings drop (the export-side valueStringList semantics), and
// pre-broken missing-object sentinels are ignored.
func stringsOf(v *types.Value) []string {
	if s := v.GetStringValue(); s != "" && s != missingObjectSentinel {
		return []string{s}
	}
	var out []string
	for _, el := range v.GetListValue().GetValues() {
		if s := el.GetStringValue(); s != "" && s != missingObjectSentinel {
			out = append(out, s)
		}
	}
	return out
}

func resolveFormat(key string, opts anyblockjson.Options) (model.RelationFormat, bool) {
	if f, err := bundle.GetRelationFormat(domain.RelationKey(key)); err == nil {
		return f, true
	}
	if opts.ResolveFormat != nil {
		return opts.ResolveFormat(domain.RelationKey(key))
	}
	return 0, false
}

// TextInventory counts the plain text of text blocks the format preserves —
// the text multiset. Structural styles (title, description) are dropped by
// design; blocks with emoji marks are skipped because emoji materialization
// changes the text lossily by design (SPEC §8).
func TextInventory(s *model.SmartBlockSnapshotBase) map[string]int {
	out := map[string]int{}
	for _, b := range s.Blocks {
		t := b.GetText()
		if t == nil || t.Text == "" {
			continue
		}
		switch t.Style {
		case model.BlockContentText_Title, model.BlockContentText_Description:
			continue
		}
		skip := false
		for _, m := range t.Marks.GetMarks() {
			if m != nil && m.Type == model.BlockContentTextMark_Emoji {
				skip = true
				break
			}
		}
		if !skip {
			out[t.Text]++
		}
	}
	return out
}

// TextSequence is the ordered analog of TextInventory: the preserved text of
// text blocks in snapshot block order, with the same structural/emoji
// filtering. Used to detect pure reordering, which the multiset cannot.
func TextSequence(s *model.SmartBlockSnapshotBase) []string {
	var out []string
	for _, b := range s.Blocks {
		t := b.GetText()
		if t == nil || t.Text == "" {
			continue
		}
		switch t.Style {
		case model.BlockContentText_Title, model.BlockContentText_Description:
			continue
		}
		skip := false
		for _, m := range t.Marks.GetMarks() {
			if m != nil && m.Type == model.BlockContentTextMark_Emoji {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, t.Text)
		}
	}
	return out
}

func preview(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

func valuePreview(v *types.Value) string {
	if v == nil {
		return "<absent>"
	}
	return preview(v.String())
}
