package wrapper

// tools.go — the per-tool executors: each maps one task tool onto its
// backing /v2 primitive (§7.2) through the wrapper channels — enumerated
// handles in, markdown in, anchors in; short labels out.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// defaultFindLimit is find's page size when no limit is given.
const defaultFindLimit = 10

// defaultSpacesLimit is spaces' page size when no limit is given.
const defaultSpacesLimit = 25

// spacesResult is spaces' machine shape.
type spacesResult struct {
	Spaces  []v2model.SpaceRow `json:"spaces"`
	Total   int                `json:"total"`
	HasMore bool               `json:"has_more"`
}

// runSpaces lists the user's spaces — the bootstrap tool: every trace needs
// a space id, and before this tool nothing in the set could produce one.
// spaceRowSeparator joins a space's name to its id in the `spaces` listing.
// The renderer and the reader below share it: what a tool PRINTS has to be
// accepted as what a tool TAKES, or the surface teaches a format it then
// refuses.
const spaceRowSeparator = " — "

// spaceArg reads the `space` argument, tolerating the shape `spaces` serves:
// "Name — id". A small model that hands a listing line straight back is not
// confused about which space it means — it is quoting the surface. Measured:
// gemma-4-e2b sent the whole rendered row on 19 of its 28 failed calls and
// scored 1/18 almost entirely because of it, while every other model
// extracted the id and passed.
func spaceArg(args map[string]any) string {
	v := strArg(args, "space")
	if _, id, ok := strings.Cut(v, spaceRowSeparator); ok {
		return strings.TrimSpace(id)
	}
	return v
}

func (r *Runner) runSpaces(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	limit := defaultSpacesLimit
	if v, ok := args["limit"]; ok {
		limit, _ = intArg(v)
		if limit < 1 {
			limit = 1
		}
		if limit > 100 {
			limit = 100
		}
	}
	var resp v2model.ListResponse[v2model.SpaceRow]
	err := r.client.decode(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces",
		query:  url.Values{"limit": []string{strconv.Itoa(limit)}},
	}, &resp)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for _, row := range resp.Data {
		name := row.Name
		if name == "" {
			name = "(unnamed)"
		}
		fmt.Fprintf(&b, "%s%s%s\n", name, spaceRowSeparator, row.Id)
	}
	switch {
	case resp.Total == 0:
		b.WriteString("no spaces")
	case resp.HasMore:
		fmt.Fprintf(&b, "%d spaces — showing %d; raise limit for the rest", resp.Total, len(resp.Data))
	default:
		fmt.Fprintf(&b, "%d spaces", resp.Total)
	}
	b.WriteString("\npass the id after the dash as space to find, describe and create")
	return &Result{
		Text: b.String(),
		JSON: spacesResult{Spaces: resp.Data, Total: resp.Total, HasMore: resp.HasMore},
	}, nil
}

// findResult is find's machine shape. Listing marks the no-criteria call
// (§8.33): its Rows carry no handle numbers, because nothing was matched.
type findResult struct {
	Handles []Handle `json:"handles"`
	Total   int      `json:"total"`
	HasMore bool     `json:"has_more"`
	Listing bool     `json:"listing,omitempty"`
	Rows    []Handle `json:"rows,omitempty"`
}

// runFind searches a space — or, when the call names no criterion at all,
// LISTS it (§8.33). The two are different acts and the difference is the
// whole point: a search ranks matches, so handle 1 is the object the caller
// asked for; a bare space matches nothing, so handle 1 is whichever object
// the index happened to return first. Both used to render as "N matches"
// with the same numbered handles, and a small model that dropped its
// `query` read that as a match, addressed handle 1 and wrote three blocks
// into an unrelated object. The listing therefore assigns NO handles: the
// intent is still served (you can see what a space holds), but nothing it
// returns can be passed as `object`, so that write is unreachable rather
// than discouraged.
func (r *Runner) runFind(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space := spaceArg(args)
	limit := defaultFindLimit
	if v, ok := args["limit"]; ok {
		limit, _ = intArg(v)
		if limit < 1 {
			limit = 1
		}
		if limit > 50 {
			limit = 50
		}
	}
	body := map[string]any{}
	for _, k := range []string{"query", "type"} {
		if v := strArg(args, k); v != "" {
			body[k] = v
		}
	}
	if filter := strArg(args, "filter"); filter != "" {
		resolved, err := r.resolveFilterMe(ctx, session, space, filter)
		if err != nil {
			return nil, err
		}
		body["filter"] = resolved
	}
	// no query, no type, no filter: nothing was matched, so this is a
	// listing and not a search (see the doc comment)
	listing := len(body) == 0
	var resp v2model.ListResponse[v2model.ObjectRow]
	search := func() error {
		return r.client.decode(ctx, apiRequest{
			method: "POST",
			path:   "/v2/spaces/" + seg(space) + "/search",
			// keys=name: a refused filter or type is refused in the name
			// vocabulary this surface teaches (§4.3) — the rows themselves
			// are unaffected (their `type` stays the key)
			query: url.Values{"limit": []string{strconv.Itoa(limit)}, "keys": []string{"name"}},
			body:  body,
		}, &resp)
	}
	if err := search(); err != nil {
		// the §8.21 case fold: retry once with the unique case variant
		folded, ok, foldErr := r.foldTypeArg(ctx, space, strArg(args, "type"), err)
		if foldErr != nil {
			return nil, foldErr
		}
		if !ok {
			return nil, err
		}
		body["type"] = folded
		if err := search(); err != nil {
			return nil, err
		}
	}

	handles := make([]Handle, 0, len(resp.Data))
	for i, row := range resp.Data {
		handles = append(handles, Handle{N: i + 1, Id: row.Id, Name: row.Name, Type: row.Type})
	}
	// the TEXT channel spells each row's type by display NAME — find is
	// where the model learns the type vocabulary it hands back to describe
	// and create, and the search row carries the internal key ("set" for
	// the type users call "Query"). Best-effort and text-only: the session
	// handle and the JSON channel keep the key (the machine identity), and
	// a failed listing degrades to the key rather than failing the find.
	var typeNames map[string]string
	if len(resp.Data) > 0 {
		typeNames = r.typeNameIndex(ctx, space)
	}
	// each find renumbers (§7.4) — and a listing numbers nothing, which
	// clears whatever the previous find left behind: stale handles surviving
	// a call that matched nothing would be the same mis-address one document
	// further away
	session.Space = space
	if listing {
		session.Handles = nil
		// the rows shed their numbers too: the JSON channel must not carry an
		// addressable-looking handle the text deliberately withheld
		rows := make([]Handle, 0, len(handles))
		for _, h := range handles {
			rows = append(rows, Handle{Id: h.Id, Name: h.Name, Type: h.Type})
		}
		return &Result{
			Text: listingText(rows, typeNames, resp.Total, resp.HasMore, resp.Warnings),
			JSON: findResult{Total: resp.Total, HasMore: resp.HasMore, Listing: true, Rows: rows},
		}, nil
	}
	session.Handles = handles

	var b strings.Builder
	for _, h := range handles {
		fmt.Fprintf(&b, "%d. %s (%s)\n", h.N, displayName(h), typeLabel(typeNames, h.Type))
	}
	switch {
	case resp.Total == 0:
		b.WriteString("no matches")
	case resp.HasMore:
		fmt.Fprintf(&b, "%d matches — showing %d; narrow with filter or query, or raise limit", resp.Total, len(handles))
	default:
		fmt.Fprintf(&b, "%d matches", resp.Total)
	}
	for _, w := range resp.Warnings {
		b.WriteString("\nwarning: " + w.Message)
	}
	return &Result{
		Text: b.String(),
		JSON: findResult{Handles: handles, Total: resp.Total, HasMore: resp.HasMore},
	}, nil
}

// displayName renders a row's name, naming the empty case rather than
// serving a blank.
func displayName(h Handle) string {
	if h.Name == "" {
		return "(unnamed)"
	}
	return h.Name
}

// listingText renders the no-criteria find. The frame comes FIRST: a small
// model reads top-down, and the fact that nothing was matched has to reach
// it before the names do, or the names read as results. Nothing is
// numbered, and the closing line names the one repair that produces
// handles.
func listingText(rows []Handle, typeNames map[string]string, total int, hasMore bool, warnings []v2model.Issue) string {
	var b strings.Builder
	b.WriteString("nothing was searched for: find with only a space has no criterion to match on, so this is a listing of what the space holds — not results, and not numbered.\n")
	for _, h := range rows {
		fmt.Fprintf(&b, "  %s (%s)\n", displayName(h), typeLabel(typeNames, h.Type))
	}
	switch {
	case total == 0:
		b.WriteString("the space is empty")
	case hasMore:
		fmt.Fprintf(&b, "%d objects — showing %d", total, len(rows))
	default:
		fmt.Fprintf(&b, "%d objects", total)
	}
	b.WriteString("\nto address one, run find again with query (words from its name), type or filter — a search numbers its results 1, 2, …, and those numbers are what `object` takes.")
	for _, w := range warnings {
		b.WriteString("\nwarning: " + w.Message)
	}
	return b.String()
}

func (r *Runner) runRead(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"), spaceArg(args))
	if err != nil {
		return nil, err
	}
	mode := strArg(args, "mode")
	// every body-serving GET asks for the name vocabulary (D5): the wrapper
	// teaches display names, so the document the model reads must spell them
	query := url.Values{"keys": []string{"name"}}
	if mode == "outline" {
		query.Set("outline", "true")
	}
	req := apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(space) + "/objects/" + seg(objectId),
		query:  query,
	}
	doc, err := r.client.raw(ctx, req)
	if err != nil {
		return nil, err
	}
	// the served document already carries the reference vocabulary: the
	// server labels minted ids itself and resolves either
	// spelling on every write channel (C4), so the wrapper serves the body
	// verbatim — client-side relabeling retired with the session label map
	return &Result{Text: string(doc), JSON: rawJSONResult(doc)}, nil
}

func (r *Runner) runCreate(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space := spaceArg(args)
	body := map[string]any{
		"type": strArg(args, "type"),
		"name": strArg(args, "name"),
	}
	if props := objArg(args, "properties"); len(props) > 0 {
		idx, err := r.propertyIndexFor(ctx, space)
		if err != nil {
			return nil, err
		}
		resolved, err := r.prepareValues(ctx, session, space, idx, props, true)
		if err != nil {
			return nil, err
		}
		body["properties"] = resolved
	}
	if md := strArg(args, "markdown"); md != "" {
		body["markdown"] = md
	}
	path := "/v2/spaces/" + seg(space) + "/objects"
	query := r.mutationQuery()
	var result v2model.CreateResult
	attempt := func() error {
		// the key re-derives per attempt: a folded type is a different
		// resolved request, so it must not reuse the failed body's key
		key := r.mutationKey(session, requestHash("POST", path, query, body), r.now())
		return r.client.decode(ctx, apiRequest{
			method:         "POST",
			path:           path,
			query:          query,
			body:           body,
			idempotencyKey: key,
		}, &result)
	}
	if err := attempt(); err != nil {
		// the §8.21 case fold: a 400 validated nothing into existence, so
		// retrying with the unique case variant is a fresh, correct create
		folded, ok, foldErr := r.foldTypeArg(ctx, space, strArg(args, "type"), err)
		if foldErr != nil {
			return nil, foldErr
		}
		if !ok {
			return nil, err
		}
		body["type"] = folded
		if err := attempt(); err != nil {
			return nil, err
		}
	}
	// the receipt spells the type by display NAME (find's rule: the text
	// channel teaches names, the machine channels keep the key) — best
	// effort, so a failed type listing serves the resolved key instead
	label := typeLabel(r.typeNameIndex(ctx, space), result.Type)
	text := fmt.Sprintf("created %s (%s)", result.Id, label)
	if !result.DryRun {
		// create used to dead-end: it returned a full id, registered no
		// handle and left the working space unset (only find set it), so a
		// caller had to SEARCH for the object it had just made — racing the
		// full-text index to address its own creation. The handle closes
		// that loop, and is named in the text because a number the caller
		// cannot see is a number it cannot pass.
		n := session.registerHandle(space, Handle{Id: result.Id, Name: strArg(args, "name"), Type: result.Type})
		text = fmt.Sprintf("created %s (%s) — handle %d", result.Id, label, n)
	}
	if result.DryRun {
		text = fmt.Sprintf("dry run — a %s object would be created", label)
	}
	for _, w := range result.Warnings {
		text += "\nwarning: " + w.Message
	}
	return &Result{Text: text, JSON: result}, nil
}

func (r *Runner) runSetProperties(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"), spaceArg(args))
	if err != nil {
		return nil, err
	}
	setM, addM, removeM := objArg(args, "set"), objArg(args, "add"), objArg(args, "remove")
	if len(setM)+len(addM)+len(removeM) == 0 {
		return nil, fmt.Errorf("set_properties needs set, add or remove — e.g. set: {\"Status\": \"Done\"}")
	}
	// one property-index fetch serves set, add AND remove
	idx, err := r.propertyIndexFor(ctx, space)
	if err != nil {
		return nil, err
	}
	op := map[string]any{"op": "set_properties"}
	for _, field := range []struct {
		name string
		m    map[string]any
	}{{"set", setM}, {"add", addM}} {
		if len(field.m) == 0 {
			continue
		}
		resolved, err := r.prepareValues(ctx, session, space, idx, field.m, true)
		if err != nil {
			return nil, err
		}
		op[field.name] = resolved
	}
	if len(removeM) > 0 {
		// remove never creates anything server-side — no option guard, but
		// @me / relative dates still resolve so entries match
		resolved, err := r.prepareValues(ctx, session, space, idx, removeM, false)
		if err != nil {
			return nil, err
		}
		op["remove"] = resolved
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, nil)
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

func (r *Runner) runCheckItem(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"), spaceArg(args))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op":  "update_block",
		"id":  strArg(args, "block"),
		"set": map[string]any{"checked": boolArg(args, "checked")},
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"id"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

// anchorTarget maps the wrapper's after/under vocabulary onto the op
// targeting (under → inside; both omitted → root-append). Refs pass through
// verbatim — resolution is server-side (C4).
func anchorTarget(args map[string]any, op map[string]any) error {
	after := strArg(args, "after")
	under := strArg(args, "under")
	if after != "" && under != "" {
		return fmt.Errorf("give after or under, not both — after inserts next to the block, under inserts into it")
	}
	if after != "" {
		op["after"] = after
	}
	if under != "" {
		op["inside"] = under
	}
	return nil
}

func (r *Runner) runAddBlocks(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"), spaceArg(args))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op":       "insert_blocks",
		"markdown": strArg(args, "markdown"),
	}
	if err := anchorTarget(args, op); err != nil {
		return nil, err
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"after", "inside"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

func (r *Runner) runEditText(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"), spaceArg(args))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op":      "replace_text",
		"find":    strArg(args, "find"),
		"replace": strArg(args, "replace"),
	}
	// block omitted (§8.21): the op's id is omitted too, and the SERVER
	// locates the block from the find snippet under the object lock
	// (APIV2.md §8.43) — same one-match-or-refuse rule this function used
	// to implement with its own GET, minus the read-then-patch TOCTOU (the
	// document could move between the locate read and the PATCH). The
	// refusals arrive server-shaped and steer.go/opsVocab re-spell them in
	// the tool vocabulary. No ref field is passed when id is absent: the
	// ambiguity retry has nothing to rewrite, so a locator ambiguity
	// surfaces directly instead of costing a wasted re-read.
	var refFields []string
	if blockRef := strArg(args, "block"); blockRef != "" {
		op["id"] = blockRef
		refFields = []string{"id"}
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, refFields)
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

func (r *Runner) runSetCell(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"), spaceArg(args))
	if err != nil {
		return nil, err
	}
	// an empty value means "clear the cell" — the op documents null as clear
	var value any = strArg(args, "value")
	if value == "" {
		value = nil
	}
	op := map[string]any{
		"op":       "set_cell",
		"table_id": strArg(args, "table"),
		"row":      strArg(args, "row"),
		"col":      strArg(args, "col"),
		"value":    value,
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"table_id", "row", "col"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

func (r *Runner) runMoveBlock(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"), spaceArg(args))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op": "move_block",
		"id": strArg(args, "block"),
	}
	if err := anchorTarget(args, op); err != nil {
		return nil, err
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"id", "after", "inside"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

// maxDeleteBlocks bounds one delete_block call's reference list. The bound
// is wrapper-side so an over-long list earns a refusal in the tool's own
// vocabulary — the server's 512-op cap answers in PATCH terms ("split the
// edit across several PATCH requests") a model driving this tool never sees.
const maxDeleteBlocks = 64

// runDeleteBlock deletes one or SEVERAL blocks in one call. The batch form
// exists because of a measured chain-length cliff (manifest.go records the
// numbers): models that had to chain delete×3 dropped the third call, and
// batching turns the 4-call dependent chain into 2. All references travel
// in ONE PATCH — the server applies the batch to a child state committed
// with a single Apply, so either every named block is deleted or none is;
// a reference that fails to resolve refuses the whole call before anything
// is removed. Each ref still rides the existing block-reference path (the
// op's id field, server-resolved per C4, ambiguity-retried per §7.4). No
// wrapper-side pre-resolution is needed for order safety: block ids are
// stable state ids — deleting one block never renames another — and a
// suffix label can only get LESS ambiguous as earlier ops shrink the pool.
func (r *Runner) runDeleteBlock(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"), spaceArg(args))
	if err != nil {
		return nil, err
	}
	refs, err := splitBlockRefs(strArg(args, "block"))
	if err != nil {
		return nil, err
	}
	ops := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		op := map[string]any{
			"op": "delete_block",
			"id": ref,
		}
		if boolArg(args, "recursive") {
			op["recursive"] = true
		}
		ops = append(ops, op)
	}
	result, err := r.patchOpsBatch(ctx, session, space, objectId, ops, []string{"id"})
	if err != nil {
		return nil, deleteBatchError(err, refs)
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

// splitBlockRefs parses delete_block's comma-separated reference list.
// Whitespace around a ref and empty segments (a trailing comma, a doubled
// one) are tolerated — the intent is unambiguous — but a reference listed
// twice is refused, not deduplicated: the second op would fail as not-found
// after the first delete and refuse the whole batch with a message about a
// block the caller DID name correctly, which is the confusing shape; and
// silently deleting once could mask a model that believed it named two
// different blocks.
func splitBlockRefs(list string) ([]string, error) {
	var refs []string
	seen := map[string]bool{}
	for _, part := range strings.Split(list, ",") {
		ref := strings.TrimSpace(part)
		if ref == "" {
			continue
		}
		if seen[ref] {
			return nil, fmt.Errorf("block %q is listed more than once — name each block once; nothing was deleted", ref)
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		// validateArgs already refused an empty required string; this catches
		// a list of only separators (",,")
		return nil, fmt.Errorf("block names no block — give %s, or several separated by commas", blockArgDescription)
	}
	if len(refs) > maxDeleteBlocks {
		return nil, fmt.Errorf("delete_block takes at most %d blocks in one call (got %d) — split the list; nothing was deleted", maxDeleteBlocks, len(refs))
	}
	return refs, nil
}

// opsIndexPathRe matches the server's ops[i]-scoped path vocabulary for the
// delete batch's one ref field.
var opsIndexPathRe = regexp.MustCompile(`ops\[\d+\]\.id`)

// opsIndexPrefixRe strips any remaining ops[i]. prefix.
var opsIndexPrefixRe = regexp.MustCompile(`ops\[\d+\]\.`)

// deleteBatchError re-frames a failed multi-reference delete. Two facts the
// model MUST get, in this order: nothing was deleted (a partial delete that
// reads as success is the worst outcome — the batch is atomic server-side,
// so the wrapper can state this truthfully), and WHICH reference failed
// (the server names it verbatim; the position in the caller's list is
// appended when the text identifies one). Server paths speak ops[i], a
// vocabulary this tool's caller never sent — re-spelled to `block`, the way
// opsVocab does for the single-op tools. A single-reference call keeps its
// error untouched: it is the shape every other block tool reports.
func deleteBatchError(err error, refs []string) error {
	if len(refs) < 2 {
		return err
	}
	var te *ToolError
	if !errors.As(err, &te) {
		return fmt.Errorf("deleted nothing — the %d deletes apply together or not at all: %w", len(refs), err)
	}
	pos := ""
	for i, ref := range refs {
		if strings.Contains(te.Text, strconv.Quote(ref)) {
			pos = fmt.Sprintf(" (block %d of the %d given)", i+1, len(refs))
			break
		}
	}
	rewrite := func(s string) string {
		s = opsIndexPathRe.ReplaceAllString(s, "block")
		return opsIndexPrefixRe.ReplaceAllString(s, "")
	}
	te.Text = fmt.Sprintf("deleted nothing — the %d deletes apply together or not at all: %s%s", len(refs), rewrite(te.Text), pos)
	for i := range te.Issues {
		te.Issues[i].Path = rewrite(te.Issues[i].Path)
		te.Issues[i].Message = rewrite(te.Issues[i].Message)
		te.Issues[i].Hint = rewrite(te.Issues[i].Hint)
	}
	return te
}

// rawJSONResult wraps a raw document for the JSON output channel.
func rawJSONResult(doc []byte) any {
	return jsonRaw(doc)
}

type jsonRaw []byte

func (r jsonRaw) MarshalJSON() ([]byte, error) { return r, nil }
