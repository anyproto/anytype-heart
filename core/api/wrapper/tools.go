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

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
)

// defaultFindLimit is find's page size when no limit is given.
const defaultFindLimit = 10

// findResult is find's machine shape.
type findResult struct {
	Handles []Handle `json:"handles"`
	Total   int      `json:"total"`
	HasMore bool     `json:"has_more"`
}

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
	var resp apimodel.V2ListResponse[apimodel.V2ObjectRow]
	err := r.client.decode(ctx, apiRequest{
		method: "POST",
		path:   "/v2/spaces/" + seg(space) + "/search",
		query:  url.Values{"limit": []string{strconv.Itoa(limit)}},
		body:   body,
	}, &resp)
	if err != nil {
		return nil, err
	}

	handles := make([]Handle, 0, len(resp.Data))
	for i, row := range resp.Data {
		handles = append(handles, Handle{N: i + 1, Id: row.Id, Name: row.Name, Type: row.Type})
	}
	// each find renumbers (§7.4); labels survive only for objects still
	// referenced, so the session stays bounded
	session.Space = space
	session.Handles = handles
	if session.Labels != nil {
		kept := map[string]map[string]string{}
		for _, h := range handles {
			if m, ok := session.Labels[h.Id]; ok {
				kept[h.Id] = m
			}
		}
		session.Labels = kept
	}

	var b strings.Builder
	for _, h := range handles {
		name := h.Name
		if name == "" {
			name = "(unnamed)"
		}
		fmt.Fprintf(&b, "%d. %s (%s)\n", h.N, name, h.Type)
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
	if mode != "outline" {
		// the reference channel: relabel full block ids to short unique
		// suffixes and retain the map for the editing tools (§7.1/§7.4)
		relabeled, labels := relabelDoc(doc)
		if labels != nil {
			session.setLabels(objectId, labels)
		}
		doc = relabeled
	}
	return &Result{Text: string(doc), JSON: rawJSONResult(doc)}, nil
}

func (r *Runner) runCreate(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space := strArg(args, "space")
	body := map[string]any{
		"type": strArg(args, "type"),
		"name": strArg(args, "name"),
	}
	if props := objArg(args, "properties"); len(props) > 0 {
		resolved, err := r.prepareValues(ctx, session, space, props, true)
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
	key := r.mutationKey(session, requestHash("POST", path, query, body))
	var result apimodel.V2CreateResult
	err := r.client.decode(ctx, apiRequest{
		method:         "POST",
		path:           path,
		query:          query,
		body:           body,
		idempotencyKey: key,
	}, &result)
	if err != nil {
		return nil, err
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
	op := map[string]any{"op": "setProperties"}
	provided := false
	for _, field := range []string{"set", "add"} {
		if m := objArg(args, field); len(m) > 0 {
			resolved, err := r.prepareValues(ctx, session, space, m, true)
			if err != nil {
				return nil, err
			}
			op[field] = resolved
			provided = true
		}
	}
	if m := objArg(args, "remove"); len(m) > 0 {
		// remove never creates anything server-side — no option guard, but
		// @me / relative dates still resolve so entries match
		resolved, err := r.prepareValues(ctx, session, space, m, false)
		if err != nil {
			return nil, err
		}
		op["remove"] = resolved
		provided = true
	}
	if !provided {
		return nil, fmt.Errorf("set_properties needs set, add or remove — e.g. set: {\"status\": \"Done\"}")
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, nil)
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(result), JSON: result}, nil
}

func (r *Runner) runCheckItem(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op":  "updateBlock",
		"id":  resolveBlockRef(session, objectId, strArg(args, "block")),
		"set": map[string]any{"checked": boolArg(args, "checked")},
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"id"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(result), JSON: result}, nil
}

// anchorTarget maps the wrapper's after/under vocabulary onto the op
// targeting (under → inside; both omitted → root-append).
func anchorTarget(session *Session, objectId string, args map[string]any, op map[string]any) error {
	after := strArg(args, "after")
	under := strArg(args, "under")
	if after != "" && under != "" {
		return fmt.Errorf("give after or under, not both — after inserts next to the block, under inserts into it")
	}
	if after != "" {
		op["after"] = resolveBlockRef(session, objectId, after)
	}
	if under != "" {
		op["inside"] = resolveBlockRef(session, objectId, under)
	}
	return nil
}

func (r *Runner) runAddBlocks(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op":       "insertBlocks",
		"markdown": strArg(args, "markdown"),
	}
	if err := anchorTarget(session, objectId, args, op); err != nil {
		return nil, err
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"after", "inside"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(result), JSON: result}, nil
}

func (r *Runner) runEditText(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op":      "replaceText",
		"id":      resolveBlockRef(session, objectId, strArg(args, "block")),
		"find":    strArg(args, "find"),
		"replace": strArg(args, "replace"),
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"id"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(result), JSON: result}, nil
}

func (r *Runner) runSetCell(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op":      "setCell",
		"tableId": resolveBlockRef(session, objectId, strArg(args, "table")),
		"row":     strArg(args, "row"),
		"col":     strArg(args, "col"),
		"value":   strArg(args, "value"),
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"tableId"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(result), JSON: result}, nil
}

func (r *Runner) runMoveBlock(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op": "moveBlock",
		"id": resolveBlockRef(session, objectId, strArg(args, "block")),
	}
	if err := anchorTarget(session, objectId, args, op); err != nil {
		return nil, err
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"id", "after", "inside"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(result), JSON: result}, nil
}

func (r *Runner) runDeleteBlock(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	op := map[string]any{
		"op": "deleteBlock",
		"id": resolveBlockRef(session, objectId, strArg(args, "block")),
	}
	if boolArg(args, "recursive") {
		op["recursive"] = true
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"id"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(result), JSON: result}, nil
}

// rawJSONResult wraps a raw document for the JSON output channel.
func rawJSONResult(doc []byte) any {
	return jsonRaw(doc)
}

type jsonRaw []byte

func (r jsonRaw) MarshalJSON() ([]byte, error) { return r, nil }
