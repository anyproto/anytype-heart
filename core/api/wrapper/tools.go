package wrapper

// tools.go — the per-tool executors: each maps one task tool onto its
// backing /v2 primitive (§7.2) through the wrapper channels — enumerated
// handles in, markdown in, anchors in; short labels out.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

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
		key := r.mutationKey(session, requestHash("POST", path, query, body))
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
	op := map[string]any{"op": "setProperties"}
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
		"op":  "updateBlock",
		"id":  resolveBlockRef(session, objectId, strArg(args, "block")),
		"set": map[string]any{"checked": boolArg(args, "checked")},
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"id"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
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
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

func (r *Runner) runEditText(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space, objectId, err := r.resolveObject(session, strArg(args, "object"))
	if err != nil {
		return nil, err
	}
	find := strArg(args, "find")
	blockRef := strArg(args, "block")
	if blockRef == "" {
		// block omitted (§8.21): locate it from the snippet — only when the
		// snippet identifies exactly ONE block; anything else refuses with a
		// repair tip, never guesses
		blockRef, err = r.locateBlock(ctx, session, space, objectId, find)
		if err != nil {
			return nil, err
		}
	} else {
		blockRef = resolveBlockRef(session, objectId, blockRef)
	}
	op := map[string]any{
		"op":      "replaceText",
		"id":      blockRef,
		"find":    find,
		"replace": strArg(args, "replace"),
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"id"})
	if err != nil {
		return nil, err
	}
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

// maxSnippetCandidates bounds how many candidate blocks an ambiguity
// refusal lists.
const maxSnippetCandidates = 8

// locateBlock resolves edit_text's block from the find snippet when the
// model omitted it (§8.21: a required block id is unknowable on turn one,
// so small models routed around the tool and called read instead). The
// ambiguity rule is the point of the feature: the snippet must identify
// exactly ONE block — zero matches steer to read mode=outline, several
// matching blocks list the candidates with context so the retry can pass
// block explicitly, and several occurrences within the one block get the
// same more-context refusal the explicit-block path earns from the server.
// A silent wrong edit is far worse than any of these refusals.
func (r *Runner) locateBlock(ctx context.Context, session *Session, spaceId, objectId, find string) (string, error) {
	doc, err := r.client.raw(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/objects/" + seg(objectId),
	})
	if err != nil {
		return "", err
	}
	// retain the labels either way — the model's next call starts resolved,
	// and the refusals below name blocks by the labels read would mint
	_, labels := relabelDoc(doc)
	if labels != nil {
		session.setLabels(objectId, labels)
	}
	var envelope struct {
		Blocks []struct {
			Id   string `json:"id"`
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(doc, &envelope); err != nil {
		return "", fmt.Errorf("decode document: %w", err)
	}
	byId := make(map[string]string, len(labels))
	for label, id := range labels {
		byId[id] = label
	}
	label := func(id string) string {
		if l, ok := byId[id]; ok {
			return l
		}
		return id
	}
	type candidate struct {
		id, typ, text string
		count         int
	}
	var candidates []candidate
	for _, b := range envelope.Blocks {
		if n := strings.Count(b.Text, find); n > 0 {
			candidates = append(candidates, candidate{id: b.Id, typ: b.Type, text: b.Text, count: n})
		}
	}
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("no block contains %q — run read with mode=outline to see the blocks, and copy the find text exactly as read shows it (text is markdown source: ** [ ] etc. count)", find)
	case 1:
		if c := candidates[0]; c.count > 1 {
			return "", fmt.Errorf("found %d matches for %q in block %s — provide more context to make the match unique", c.count, find, label(c.id))
		}
		return candidates[0].id, nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%q appears in %d blocks — retry with block naming one of:", find, len(candidates))
		for i, c := range candidates {
			if i == maxSnippetCandidates {
				fmt.Fprintf(&b, "\n  … and %d more", len(candidates)-maxSnippetCandidates)
				break
			}
			fmt.Fprintf(&b, "\n  block %s (%s): %q", label(c.id), c.typ, snippetContext(c.text, find))
		}
		return "", fmt.Errorf("%s", b.String())
	}
}

// snippetContextWindow is how much surrounding text an ambiguity candidate
// carries on each side of the snippet.
const snippetContextWindow = 30

// snippetContext excerpts the text around the snippet's first occurrence —
// enough context to tell candidate blocks apart without dumping whole
// blocks into the refusal.
func snippetContext(text, find string) string {
	idx := strings.Index(text, find)
	start := idx - snippetContextWindow
	prefix := "…"
	if start <= 0 {
		start, prefix = 0, ""
	}
	end := idx + len(find) + snippetContextWindow
	suffix := "…"
	if end >= len(text) {
		end, suffix = len(text), ""
	}
	// never slice mid-rune: move both cuts forward to rune boundaries
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	return prefix + text[start:end] + suffix
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
		"op":      "setCell",
		"tableId": resolveBlockRef(session, objectId, strArg(args, "table")),
		"row":     resolveBlockRef(session, objectId, strArg(args, "row")),
		"col":     resolveBlockRef(session, objectId, strArg(args, "col")),
		"value":   value,
	}
	result, err := r.patchOps(ctx, session, space, objectId, op, []string{"tableId", "row", "col"})
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
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
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
	return &Result{Text: editSummary(targetLabel(session, strArg(args, "object")), result), JSON: result}, nil
}

// rawJSONResult wraps a raw document for the JSON output channel.
func rawJSONResult(doc []byte) any {
	return jsonRaw(doc)
}

type jsonRaw []byte

func (r jsonRaw) MarshalJSON() ([]byte, error) { return r, nil }
