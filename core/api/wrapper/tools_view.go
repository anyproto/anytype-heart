package wrapper

// tools_view.go — the update_view tool: ONE call that changes how a dataview
// shows its objects — filter, sort order, visible columns — wrapped over the
// server's update_view PATCH op (v2service viewops.go, APIV2.md §8.17).
//
// The three arguments map onto the op's two merge channels and travel in ONE
// single-op PATCH, so the call is atomic by construction: the server
// validates set and columns together against a private copy and refuses the
// whole op on any failure — a bad sort key, a filter that does not parse or
// an unknown column key each change NOTHING, which is the contract the
// receipt text relies on ("filter set" must never be a half-truth).
//
//   - filter → set.filter, the op's compact-string channel: the SAME syntax
//     `find` publishes, parsed server-side into the structured filters. The
//     reuse is the point of the design — the model already speaks it.
//   - sort → set.sorts, parsed wrapper-side from the ORDER BY grammar
//     ("Due date desc, Name") the training data taught every model.
//   - columns → the op's per-column merge channel, computed wrapper-side
//     from a read (see viewColumnPatches for why a read is needed at all).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// maxUpdateViewSorts mirrors the sorts maxItems the op schema advertises
// (schemas_ops.go v2ViewSetPropDef) — enforced wrapper-side so the refusal
// speaks `sort`, not ops[0].set.sorts.
const maxUpdateViewSorts = 10

// maxViewColumnPatches mirrors the columns maxProperties the served op
// schema advertises (v2service maxV2ViewColumns). Checked wrapper-side
// because the wrapper's patch map can be LARGER than the caller's list —
// showing K columns also hides the rest — and the server's over-cap refusal
// would then count patches the caller never sent.
const maxViewColumnPatches = 64

func (r *Runner) runUpdateView(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"), spaceArg(args))
	if err != nil {
		return nil, err
	}
	filter := strArg(args, "filter")
	sortStr := strArg(args, "sort")
	columns := strArg(args, "columns")
	if filter == "" && sortStr == "" && columns == "" {
		return nil, fmt.Errorf(`update_view needs filter, sort or columns — e.g. filter: "Done = false", sort: "Due date desc", columns: "Name,Status"`)
	}
	op := map[string]any{"op": "update_view"}
	blockRef := strArg(args, "block")
	viewRef := strArg(args, "view")
	if blockRef != "" {
		op["block"] = blockRef
	}
	if viewRef != "" {
		op["view"] = viewRef
	}
	set := map[string]any{}
	// parts is the receipt: each given argument earns one clause, so the ok
	// line names exactly what the atomic op changed
	var parts []string
	if filter != "" {
		// the same "@me" convenience find's filter has — one filter syntax,
		// one set of conveniences, on every surface that takes it
		resolved, err := r.resolveFilterMe(ctx, session, space, filter)
		if err != nil {
			return nil, err
		}
		set["filter"] = resolved
		parts = append(parts, "filter set")
	}
	if sortStr != "" {
		sorts, echo, err := parseSortArg(sortStr)
		if err != nil {
			return nil, err
		}
		set["sorts"] = sorts
		parts = append(parts, "sorted by "+echo)
	}
	if columns != "" {
		keys, err := splitColumnKeys(columns)
		if err != nil {
			return nil, err
		}
		patches, err := r.viewColumnPatches(ctx, space, objectId, blockRef, viewRef, keys)
		if err != nil {
			return nil, err
		}
		op["columns"] = patches
		parts = append(parts, "columns: "+strings.Join(keys, ", "))
	}
	if len(set) > 0 {
		op["set"] = set
	}
	// `view` is deliberately NOT a retryable ref field: the §7.4 ambiguity
	// retry rewrites against servedLocalIds, which lists block/row/column
	// ids only — a view label could tail-match a BLOCK id and be rewritten
	// into nonsense
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"block"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: viewSummary(targetLabel(session, strArg(args, "object")), parts, result), JSON: result}, nil
}

// viewSummary renders update_view's receipt in editSummary's frame, naming
// the argument-level changes instead of diff stats — "1 changed" would be
// true of every view edit and say nothing.
func viewSummary(target string, parts []string, result *v2model.EditResult) string {
	line := fmt.Sprintf("ok — %s: %s", target, strings.Join(parts, ", "))
	if result.DryRun {
		line = fmt.Sprintf("dry run — %s: would apply %s", target, strings.Join(parts, ", "))
	}
	for _, w := range result.Warnings {
		line += "\nwarning: " + w.Message
	}
	return line
}

// parseSortArg parses the sort grammar — comma-separated terms, each a
// property with an optional direction: "Due date desc, Name". It is ORDER
// BY's grammar minus the keyword, chosen because every model has read a
// million of them; asc is the default for the same reason. Within a term,
// the LAST whitespace-separated token is the direction when it spells
// asc/desc (case-insensitively) and everything before it — joined with
// single spaces — is the key: property names carry spaces now (D5), so a
// fixed word count cannot parse a term. A term with no trailing direction
// is all key. The parsed terms become the op's set.sorts array; the echo
// string is the normalized receipt form (direction always explicit, so the
// caller sees what the default resolved to). The key itself is NOT
// validated here — the server owns key resolution and the did-you-mean
// refusal, and refuses the whole op on a miss; a mistyped direction
// ("descending") therefore becomes part of the key and earns that refusal,
// the price of multi-word keys.
func parseSortArg(sort string) (sorts []map[string]any, echo string, err error) {
	var echoes []string
	seen := map[string]bool{}
	for _, term := range strings.Split(sort, ",") {
		fields := strings.Fields(term)
		if len(fields) == 0 {
			continue // a doubled or trailing comma — the intent is unambiguous
		}
		dir := "asc"
		if len(fields) > 1 {
			switch last := strings.ToLower(fields[len(fields)-1]); last {
			case "asc", "desc":
				dir = last
				fields = fields[:len(fields)-1]
			}
		}
		key := strings.Join(fields, " ")
		// a key sorted twice is refused, not deduplicated: the second
		// direction would silently lose, and a model that repeats a key
		// believed it named two different ones (the splitBlockRefs rule)
		if seen[key] {
			return nil, "", fmt.Errorf("sort names %q more than once — name each property once; nothing was changed", key)
		}
		seen[key] = true
		sorts = append(sorts, map[string]any{"property": key, "direction": dir})
		echoes = append(echoes, key+" "+dir)
	}
	if len(sorts) == 0 {
		return nil, "", fmt.Errorf(`sort names no property — e.g. "Due date desc"`)
	}
	if len(sorts) > maxUpdateViewSorts {
		return nil, "", fmt.Errorf("sort takes at most %d properties (got %d); nothing was changed", maxUpdateViewSorts, len(sorts))
	}
	return sorts, strings.Join(echoes, ", "), nil
}

// splitColumnKeys parses the comma-separated show-list — the same comma
// contract delete_block's reference list set (splitBlockRefs; a separate
// function because the refusals must speak in columns, not blocks, and the
// helper bakes its texts in).
func splitColumnKeys(list string) ([]string, error) {
	var keys []string
	seen := map[string]bool{}
	for _, part := range strings.Split(list, ",") {
		key := strings.TrimSpace(part)
		if key == "" {
			continue
		}
		if seen[key] {
			return nil, fmt.Errorf("column %q is listed more than once — name each column once; nothing was changed", key)
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf(`columns names no property — give the properties to show, comma-separated ("Name,Status")`)
	}
	if len(keys) > maxViewColumnPatches {
		return nil, fmt.Errorf("columns takes at most %d properties (got %d); nothing was changed", maxViewColumnPatches, len(keys))
	}
	return keys, nil
}

//
// ---- the columns channel ----
//

// servedViewColumn / servedView / servedViewBlock shape the slice of a
// served document the columns channel reads.
type servedViewColumn struct {
	Property string `json:"property"`
	Hidden   bool   `json:"hidden"`
}

type servedView struct {
	Id      string             `json:"id"`
	Name    string             `json:"name"`
	Columns []servedViewColumn `json:"columns"`
}

type servedViewBlock struct {
	Id    string       `json:"id"`
	Type  string       `json:"type"`
	Views []servedView `json:"views"`
}

// viewColumnPatches turns the caller's show-list into the op's per-column
// merge channel: every listed key becomes visible, every OTHER currently
// visible column of the target view is hidden — hidden, not removed, so a
// column's width and config survive being shown again.
//
// This is the one channel that needs a read: the op has no declarative
// "these are the visible columns" form (set.columns is rejected by design —
// one flip must never rewrite the array), so declaring the visible SET means
// knowing the current one. The read-then-patch gap is accepted: a column
// added concurrently between the GET and the PATCH stays visible, which is
// benign; the alternative — merge-only semantics with no read — could not
// express "show only these" at all. The PATCH itself is still one atomic
// op: an invalid key anywhere refuses every patch in it.
func (r *Runner) viewColumnPatches(ctx context.Context, spaceId, objectId, blockRef, viewRef string, keys []string) (map[string]any, error) {
	doc, err := r.client.raw(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/objects/" + seg(objectId),
		// the name vocabulary (D5), like every wrapper document read: the
		// column keys read here are the spellings the patches send back
		query: url.Values{"keys": []string{"name"}},
	})
	if err != nil {
		return nil, fmt.Errorf("read the object to plan column visibility: %w", err)
	}
	var envelope struct {
		Blocks []servedViewBlock `json:"blocks"`
	}
	if err := json.Unmarshal(doc, &envelope); err != nil {
		return nil, fmt.Errorf("decode the served document: %w", err)
	}
	dv, err := pickServedDataview(envelope.Blocks, blockRef)
	if err != nil {
		return nil, err
	}
	view, err := pickServedView(dv, viewRef)
	if err != nil {
		return nil, err
	}

	claimed := make([]bool, len(view.Columns))
	patches := make(map[string]any, len(keys))
	for _, key := range keys {
		spelling := key
		exact := -1
		var folded []int
		for i, col := range view.Columns {
			if col.Property == key {
				exact = i
				break
			}
			if foldColumnKey(col.Property) == foldColumnKey(key) {
				folded = append(folded, i)
			}
		}
		switch {
		case exact >= 0:
			claimed[exact] = true
		case len(folded) == 1:
			// the caller's spelling and the document's differ only in case or
			// punctuation. Send the DOCUMENT spelling: the server
			// canonicalises every key it receives (viewops.go
			// canonicalViewKey), so sending both spellings — the caller's as
			// a show, the document's as a hide — would collapse onto ONE
			// canonical key with whichever patch sorts last winning
			// (applyViewColumns merges in sorted key order): the caller could
			// ask to show a column and have this very call hide it.
			spelling = view.Columns[folded[0]].Property
			claimed[folded[0]] = true
		case len(folded) > 1:
			var candidates []string
			for _, i := range folded {
				candidates = append(candidates, view.Columns[i].Property)
			}
			// the candidates are the DOCUMENT's own spellings — display
			// names under D5 reads — so "column", never "key"
			return nil, fmt.Errorf("column %q matches several columns (%s) — use the exact spelling; nothing was changed",
				key, strings.Join(candidates, ", "))
		}
		// a key matching no current column passes through verbatim: the op
		// appends it as a new visible column, and the SERVER owns validating
		// the key (unknown keys earn the did-you-mean refusal and change
		// nothing)
		patches[spelling] = map[string]any{"hidden": false}
	}
	for i, col := range view.Columns {
		if claimed[i] || col.Hidden || col.Property == "" {
			continue
		}
		if _, shown := patches[col.Property]; shown {
			continue
		}
		patches[col.Property] = map[string]any{"hidden": true}
	}
	if len(patches) > maxViewColumnPatches {
		return nil, fmt.Errorf("showing %d columns means hiding %d others — %d column changes, more than the %d one call carries; nothing was changed",
			len(keys), len(patches)-len(keys), len(patches), maxViewColumnPatches)
	}
	return patches, nil
}

// foldColumnKey folds a property spelling for wrapper-side pairing only —
// the format's own FoldKeyTerm (NFC, casefold, `_`/`-`, whitespace and
// invisibles stripped), so due_date / dueDate / "Due date" meet, and a
// non-Latin name folds to its own class instead of the empty string the old
// ASCII-letters-only fold collapsed every such name into (two Cyrillic
// column names paired with EACH OTHER — a false ambiguity refusal). The
// fold must be at least as wide as the server's resolution (which accepts
// names, resolvePropertyInput chain step 5): a spelling that resolves
// server-side but failed to pair here would produce the show/hide collision
// described above. A spelling the fold does NOT pair still reaches the
// server, which owns the real resolution.
func foldColumnKey(s string) string {
	return anyblockjson.FoldKeyTerm(s)
}

// pickServedDataview resolves the target dataview block among the served
// blocks, under the same rule the server applies (viewops.go
// resolveDataviewBlock): an explicit reference must name a dataview; omitted,
// the object's only dataview is the target, several refuse listing the ids,
// and zero is a not-found. The wrapper duplicates the rule because the
// columns channel needs the resolved view BEFORE the PATCH — and it must
// refuse where it cannot resolve, never fall back to a patch without the
// hide half (a partial application that reports success).
func pickServedDataview(blocks []servedViewBlock, ref string) (*servedViewBlock, error) {
	if ref != "" {
		ids := make([]string, len(blocks))
		for i, b := range blocks {
			ids[i] = b.Id
		}
		idx, matches := matchServedRef(ids, ref)
		switch {
		case matches == 1:
			if blocks[idx].Type != "dataview" {
				return nil, fmt.Errorf("block %q is a %q block, not a dataview — update_view addresses a dataview block", ref, blocks[idx].Type)
			}
			return &blocks[idx], nil
		case matches > 1:
			return nil, fmt.Errorf("block %q matches more than one block — use the full block id; nothing was changed", ref)
		default:
			return nil, fmt.Errorf("block %q not found — run read (mode=outline) to see the blocks", ref)
		}
	}
	var found []*servedViewBlock
	for i := range blocks {
		if blocks[i].Type == "dataview" {
			found = append(found, &blocks[i])
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return nil, fmt.Errorf("this object has no dataview block — views live in dataview blocks; types, sets and collections carry one")
	default:
		ids := make([]string, len(found))
		for i, b := range found {
			ids[i] = b.Id
		}
		return nil, fmt.Errorf("this object has %d dataview blocks — name one with block: %s", len(found), strings.Join(ids, ", "))
	}
}

// pickServedView resolves the target view within one dataview, under the
// server's rule (viewops.go resolveViewIndex/matchViewRef): full id or
// unique suffix; omitted defaults to the only view. Ambiguity refuses with
// the candidates listed — never a guess — and every view refusal carries the
// `id ("name")` listing so the repair needs no second read.
func pickServedView(dv *servedViewBlock, ref string) (*servedView, error) {
	ids := make([]string, len(dv.Views))
	listed := make([]string, len(dv.Views))
	for i, v := range dv.Views {
		ids[i] = v.Id
		listed[i] = fmt.Sprintf("%s (%q)", v.Id, v.Name)
	}
	if ref == "" {
		switch len(dv.Views) {
		case 1:
			return &dv.Views[0], nil
		case 0:
			return nil, fmt.Errorf("this dataview has no views")
		default:
			return nil, fmt.Errorf("this dataview has %d views — name one with view: %s", len(dv.Views), strings.Join(listed, ", "))
		}
	}
	idx, matches := matchServedRef(ids, ref)
	switch {
	case matches == 1:
		return &dv.Views[idx], nil
	case matches > 1:
		var candidates []string
		for i, id := range ids {
			if refNamesServedId(id, ref) {
				candidates = append(candidates, listed[i])
			}
		}
		return nil, fmt.Errorf("view %q matches several views: %s — use the full view id; nothing was changed",
			ref, strings.Join(candidates, ", "))
	default:
		return nil, fmt.Errorf("view %q not found — views: %s", ref, strings.Join(listed, ", "))
	}
}

// matchServedRef resolves a reference against SERVED ids: exact wins,
// otherwise the suffix rule (matchBlockRef's) counts matches.
func matchServedRef(ids []string, ref string) (idx, matches int) {
	for i, id := range ids {
		if id == ref {
			return i, 1
		}
	}
	suffix, count := -1, 0
	for i, id := range ids {
		if refNamesServedId(id, ref) {
			suffix, count = i, count+1
		}
	}
	return suffix, count
}

// mintedRefShapeRe recognises the two machine-minted local-id shapes the
// server relabels on reads (anyblockjson isMintedLocalId): 24-hex block/
// row/column ids and RFC-4122 UUID view ids.
var mintedRefShapeRe = regexp.MustCompile(`^(?:[0-9a-f]{24}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// refNamesServedId reports whether a non-exact reference names a served id.
// Two directions, because the wrapper matches against a DEFAULT read, whose
// minted ids are served as short labels: the usual case is a short ref
// tailing a longer served spelling; the flip is a caller holding the FULL
// minted spelling of an id the read served as its 5-char label — the label
// is the minted id's own tail, so the suffix test reverses. The server
// resolves both spellings on every write channel, so the wrapper must too
// or the two halves of the call would disagree about what exists.
func refNamesServedId(id, ref string) bool {
	if strings.HasSuffix(id, ref) {
		return true
	}
	// len(id) >= 5: served labels are never shorter (compactIdMinLen), and
	// requiring it keeps a degenerate one-char id from tail-matching every
	// minted ref
	return len(ref) > len(id) && len(id) >= 5 && mintedRefShapeRe.MatchString(ref) && strings.HasSuffix(ref, id)
}
