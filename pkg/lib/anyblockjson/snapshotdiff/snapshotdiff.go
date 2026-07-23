// Package snapshotdiff compares two smartblock snapshots on the axes the
// AnyBlock JSON format promises to preserve: detail values (up to the
// documented normalizations) and the text content of non-structural text
// blocks (as a multiset). It is the state-diff / text-multiset comparator
// behind cmd/anyblockroundtrip and the API v2 eval harness's corruption
// metric (DELEGATE-52 backtranslation). Findings are triage input, not
// proof.
package snapshotdiff

import (
	"fmt"
	"sort"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// strippedKeys mirrors the export-side strip set (SPEC §3): LocalAndDerived
// minus the keys the importer meaningfully preserves. Differences on these
// keys are never loss.
var strippedKeys = func() map[string]bool {
	kept := map[string]bool{
		"createdDate": true, "lastModifiedDate": true, "creator": true,
		"isFavorite": true, "isArchived": true, "resolvedLayout": true,
	}
	out := map[string]bool{"id": true, "type": true}
	for _, k := range bundle.LocalAndDerivedRelationKeys {
		if !kept[string(k)] {
			out[string(k)] = true
		}
	}
	return out
}()

// Compare reports every place where got diverges from orig on a
// format-preserved axis, as human-readable findings. An empty result means
// no detectable drift.
func Compare(orig, got *model.SmartBlockSnapshotBase, opts anyblockjson.Options) []string {
	var out []string

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
