package wrapper

// tools.go — the per-tool executors: each maps one task tool onto its
// backing /v2 primitive (§7.2) through the wrapper channels — enumerated
// handles in, markdown in, anchors in; short labels out.

import (
	"context"
	"fmt"
	"net/url"
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
		fmt.Fprintf(&b, "%s — %s\n", name, row.Id)
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
	space := strArg(args, "space")
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
			query:  url.Values{"limit": []string{strconv.Itoa(limit)}},
			body:   body,
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
			Text: listingText(rows, resp.Total, resp.HasMore, resp.Warnings),
			JSON: findResult{Total: resp.Total, HasMore: resp.HasMore, Listing: true, Rows: rows},
		}, nil
	}
	session.Handles = handles

	var b strings.Builder
	for _, h := range handles {
		fmt.Fprintf(&b, "%d. %s (%s)\n", h.N, displayName(h), h.Type)
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
func listingText(rows []Handle, total int, hasMore bool, warnings []v2model.Issue) string {
	var b strings.Builder
	b.WriteString("nothing was searched for: find with only a space has no criterion to match on, so this is a listing of what the space holds — not results, and not numbered.\n")
	for _, h := range rows {
		fmt.Fprintf(&b, "  %s (%s)\n", displayName(h), h.Type)
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
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	mode := strArg(args, "mode")
	req := apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(space) + "/objects/" + seg(objectId),
	}
	if mode == "outline" {
		req.query = url.Values{"outline": []string{"true"}}
	}
	doc, err := r.client.raw(ctx, req)
	if err != nil {
		return nil, err
	}
	// the served document already carries the reference vocabulary: the
	// server labels minted ids itself (Wave 0.2) and resolves either
	// spelling on every write channel (C4), so the wrapper serves the body
	// verbatim — client-side relabeling retired with the session label map
	return &Result{Text: string(doc), JSON: rawJSONResult(doc)}, nil
}

func (r *Runner) runCreate(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space := strArg(args, "space")
	body := map[string]any{
		"type": strArg(args, "type"),
		"name": strArg(args, "name"),
	}
	if props := objArg(args, "properties"); len(props) > 0 {
		formats, err := r.propertyFormats(ctx, space)
		if err != nil {
			return nil, err
		}
		resolved, err := r.prepareValues(ctx, session, space, formats, props, true)
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
	text := fmt.Sprintf("created %s (%s)", result.Id, result.Type)
	if result.DryRun {
		text = fmt.Sprintf("dry run — a %s object would be created", result.Type)
	}
	for _, w := range result.Warnings {
		text += "\nwarning: " + w.Message
	}
	return &Result{Text: text, JSON: result}, nil
}

func (r *Runner) runSetProperties(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	setM, addM, removeM := objArg(args, "set"), objArg(args, "add"), objArg(args, "remove")
	if len(setM)+len(addM)+len(removeM) == 0 {
		return nil, fmt.Errorf("set_properties needs set, add or remove — e.g. set: {\"status\": \"Done\"}")
	}
	// one property-index fetch serves set, add AND remove
	formats, err := r.propertyFormats(ctx, space)
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
		resolved, err := r.prepareValues(ctx, session, space, formats, field.m, true)
		if err != nil {
			return nil, err
		}
		op[field.name] = resolved
	}
	if len(removeM) > 0 {
		// remove never creates anything server-side — no option guard, but
		// @me / relative dates still resolve so entries match
		resolved, err := r.prepareValues(ctx, session, space, formats, removeM, false)
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
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
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
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
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
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
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
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
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
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
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

func (r *Runner) runDeleteBlock(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op": "delete_block",
		"id": strArg(args, "block"),
	}
	if boolArg(args, "recursive") {
		op["recursive"] = true
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"id"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

// rawJSONResult wraps a raw document for the JSON output channel.
func rawJSONResult(doc []byte) any {
	return jsonRaw(doc)
}

type jsonRaw []byte

func (r jsonRaw) MarshalJSON() ([]byte, error) { return r, nil }
