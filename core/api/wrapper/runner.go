package wrapper

// runner.go — tool execution: strict argument validation against the same
// Arg table the manifest serves, handle/label resolution (the reference
// channel), the idempotency machinery, and the §7.4 ambiguity retry. One
// Runner serves both deliveries: the CLI constructs it per invocation with
// a FileStore; a long-lived host constructs it once with a MemoryStore.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// Result is one tool call's outcome: Text is the agent/human-readable form
// (the CLI's default output); JSON is the machine shape (CLI --json, MCP).
type Result struct {
	Text string
	JSON any
}

// Runner executes tools against the local API.
type Runner struct {
	client *Client
	store  Store

	// DryRun sends ?dry_run=true on every mutation (CLI --dry-run).
	DryRun bool
	// IfMatch adds an If-Match precondition on mutations (CLI advanced
	// flag; the task tools themselves never set it — C7 advisory mode).
	IfMatch string
	// AllowNewOptions skips the wrapper's option-name pre-validation (the
	// A2 guard), letting the REST create-missing semantics (R9) through.
	AllowNewOptions bool

	// mu serializes Run: the long-lived delivery shares one Runner across
	// concurrent tool calls, and a tool call is a session read-modify-write.
	mu sync.Mutex

	now func() time.Time
}

// NewRunner builds a runner over a client and a session store.
func NewRunner(client *Client, store Store) *Runner {
	return &Runner{client: client, store: store, now: time.Now}
}

// executors maps tool names to implementations. A test asserts this map and
// Tools() agree exactly — the one-definition contract.
var executors = map[string]func(*Runner, context.Context, *Session, map[string]any) (*Result, error){
	"spaces":         (*Runner).runSpaces,
	"find":           (*Runner).runFind,
	"read":           (*Runner).runRead,
	"describe":       (*Runner).runDescribe,
	"create":         (*Runner).runCreate,
	"set_properties": (*Runner).runSetProperties,
	"check_item":     (*Runner).runCheckItem,
	"add_blocks":     (*Runner).runAddBlocks,
	"edit_text":      (*Runner).runEditText,
	"set_cell":       (*Runner).runSetCell,
	"move_block":     (*Runner).runMoveBlock,
	"delete_block":   (*Runner).runDeleteBlock,
}

// Run executes one tool call. Arguments are validated strictly against the
// tool's Arg table (unknown args, missing required args and wrong types are
// wrapper-side errors with the same steering style the server uses).
func (r *Runner) Run(ctx context.Context, tool string, args map[string]any) (*Result, error) {
	def, ok := ToolByName(tool)
	if !ok {
		return nil, fmt.Errorf("unknown tool %q — tools: %s", tool, strings.Join(ToolNames(), ", "))
	}
	exec := executors[tool]
	if exec == nil {
		return nil, fmt.Errorf("tool %q has no executor", tool)
	}
	if err := validateArgs(def, args); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	session, err := r.store.Load()
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	result, err := exec(r, ctx, session, args)
	// the session is saved on BOTH paths: a failed mutation has already
	// minted its Idempotency-Key (Session.LastWrite), and dropping it is
	// exactly the double-apply the reuse window exists to prevent — the
	// harness re-run after a failure must carry the SAME key
	if saveErr := r.store.Save(session); saveErr != nil && err == nil {
		err = fmt.Errorf("save session: %w", saveErr)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// validateArgs enforces the Arg table: unknown keys, required presence,
// scalar types and enums.
func validateArgs(def Tool, args map[string]any) error {
	for name := range args {
		if _, ok := def.arg(name); !ok {
			known := make([]string, 0, len(def.Args))
			for _, a := range def.Args {
				known = append(known, a.Name)
			}
			return fmt.Errorf("%s does not take %q — arguments: %s", def.Name, name, strings.Join(known, ", "))
		}
	}
	for _, a := range def.Args {
		v, present := args[a.Name]
		if !present {
			if a.Required {
				return fmt.Errorf("%s needs %q%s", def.Name, a.Name, argHint(a))
			}
			continue
		}
		switch a.Type {
		case ArgString:
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s: %q must be a string", def.Name, a.Name)
			}
			// empty is distinct from missing: the argument WAS supplied, so
			// the error must not claim otherwise (that shape produces a
			// re-send-the-same-call repair loop)
			if a.Required && s == "" && !a.AllowEmpty {
				return fmt.Errorf("%s: %q must not be empty%s", def.Name, a.Name, argHint(a))
			}
			if len(a.Enum) > 0 && s != "" && !containsStr(a.Enum, s) {
				return fmt.Errorf("%s: %q must be one of %s", def.Name, a.Name, strings.Join(a.Enum, ", "))
			}
		case ArgBoolean:
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s: %q must be true or false", def.Name, a.Name)
			}
		case ArgInteger:
			if _, err := intArg(v); err != nil {
				return fmt.Errorf("%s: %q must be an integer", def.Name, a.Name)
			}
		case ArgObject:
			if _, ok := v.(map[string]any); !ok {
				return fmt.Errorf("%s: %q must be an object of key → value", def.Name, a.Name)
			}
		}
	}
	return nil
}

// argHint renders " — <description>" or nothing — never a dangling dash.
func argHint(a Arg) string {
	if a.Description == "" {
		return ""
	}
	return " — " + a.Description
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// intArg accepts the JSON and CLI encodings of an integer.
func intArg(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("not an integer")
		}
		return int(n), nil
	case string:
		return strconv.Atoi(n)
	default:
		return 0, fmt.Errorf("not an integer")
	}
}

func strArg(args map[string]any, name string) string {
	s, _ := args[name].(string)
	return s
}

func boolArg(args map[string]any, name string) bool {
	b, _ := args[name].(bool)
	return b
}

func objArg(args map[string]any, name string) map[string]any {
	m, _ := args[name].(map[string]any)
	return m
}

//
// ---- the reference channel: handles + labels ----
//

var handleRe = regexp.MustCompile(`^[0-9]{1,3}$`)

// errNoSession steers a handle user to find.
func errNoSession(what string) error {
	return fmt.Errorf("no working session for %s — run find first: it numbers the results (1, 2, …) and sets the working space", what)
}

// resolveObject maps the `object` argument to (spaceId, objectId): a handle
// number resolves through the last find; anything else is an object id in
// the session's working space.
func (r *Runner) resolveObject(session *Session, ref string) (string, string, error) {
	if handleRe.MatchString(ref) {
		n, _ := strconv.Atoi(ref)
		h, ok := session.handle(n)
		if !ok {
			if len(session.Handles) == 0 {
				return "", "", errNoSession(fmt.Sprintf("handle %d", n))
			}
			return "", "", fmt.Errorf("no handle %d — the last find returned %d results; run find again to renumber", n, len(session.Handles))
		}
		return session.Space, h.Id, nil
	}
	if session.Space == "" {
		return "", "", errNoSession(fmt.Sprintf("object %q", ref))
	}
	return session.Space, ref, nil
}

// resolveBlockRef maps a block label to its retained full id when the
// session has one; otherwise the ref passes through (unique-suffix
// resolution is server-side, C4).
func resolveBlockRef(session *Session, objectId, ref string) string {
	if full, ok := session.Labels[objectId][ref]; ok {
		return full
	}
	return ref
}

//
// ---- full-read relabeling ----
//

var fullBlockIdRe = regexp.MustCompile(`^[0-9a-f]{24}$`)

// labelLen is the starting label length (the C4 outline label shape).
const labelLen = 5

// relabelDoc replaces every full 24-hex block id in a document — block ids
// AND table row/column ids, which are the same bson-hex shape and the ids
// set_cell takes — with a short unique-suffix label, returning the
// rewritten document plus the label → full-id map. Uniqueness is computed
// over the WHOLE pool, so a row label can never collide with a block label.
// The replacement is textual over the exact quoted ids, so the document's
// canonical key order survives (derived cell ids like "row-col" are left
// alone: the quoted-exact match cannot touch them).
func relabelDoc(doc []byte) ([]byte, map[string]string) {
	type idHolder struct {
		Id string `json:"id"`
	}
	var envelope struct {
		Blocks []struct {
			Id      string     `json:"id"`
			Columns []idHolder `json:"columns"`
			Rows    []idHolder `json:"rows"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(doc, &envelope); err != nil {
		return doc, nil
	}
	var ids []string
	addId := func(id string) {
		if fullBlockIdRe.MatchString(id) {
			ids = append(ids, id)
		}
	}
	for _, b := range envelope.Blocks {
		addId(b.Id)
		for _, c := range b.Columns {
			addId(c.Id)
		}
		for _, row := range b.Rows {
			addId(row.Id)
		}
	}
	if len(ids) == 0 {
		return doc, nil
	}
	labels := suffixLabels(ids)
	out := doc
	byLabel := make(map[string]string, len(labels))
	for id, label := range labels {
		out = []byte(strings.ReplaceAll(string(out), `"`+id+`"`, `"`+label+`"`))
		byLabel[label] = id
	}
	return out, byLabel
}

// suffixLabels assigns each id its shortest unique suffix of at least
// labelLen characters (unique = no OTHER id ends with it, the server's
// matchBlockRef rule).
func suffixLabels(ids []string) map[string]string {
	labels := make(map[string]string, len(ids))
	for _, id := range ids {
		for n := labelLen; n <= len(id); n++ {
			suffix := id[len(id)-n:]
			unique := true
			for _, other := range ids {
				if other != id && strings.HasSuffix(other, suffix) {
					unique = false
					break
				}
			}
			if unique {
				labels[id] = suffix
				break
			}
		}
		if _, ok := labels[id]; !ok {
			labels[id] = id
		}
	}
	return labels
}

//
// ---- mutation machinery ----
//

// mutationKey returns the Idempotency-Key for a resolved mutation request:
// reused from LastWrite when the identical request repeats within the reuse
// window (a retry — C8 replays it), fresh otherwise (an intentional repeat
// applies). hash comes from requestHash.
func (r *Runner) mutationKey(session *Session, hash string) string {
	if lw := session.LastWrite; lw != nil && lw.Hash == hash && r.now().Sub(lw.At) < idempotencyReuseWindow {
		lw.At = r.now()
		return lw.Key
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("random idempotency key: %w", err))
	}
	key := hex.EncodeToString(b[:])
	session.LastWrite = &LastWrite{Hash: hash, Key: key, At: r.now()}
	return key
}

// requestHash digests the RESOLVED request the wrapper is about to send —
// method, path, encoded query and marshalled body: the same identity the
// server's C8 middleware hashes. Hashing the raw tool args instead (the
// original sketch) was wrong in both directions: a dry run and its real
// twin, or one tool call re-addressed by a re-find, shared a key (the C8
// store answers 409 idempotency_conflict — the write silently does not
// happen), while a genuine retry whose relative dates resolve identically
// must and now does share one.
func requestHash(method, path string, query url.Values, body any) string {
	h := sha256.New()
	for _, part := range []string{method, path, query.Encode()} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	if body != nil {
		payload, _ := json.Marshal(body)
		h.Write(payload)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// mutationQuery renders the shared mutation query params.
func (r *Runner) mutationQuery() url.Values {
	if !r.DryRun {
		return nil
	}
	return url.Values{"dry_run": []string{"true"}}
}

// patchOps sends a single-op PATCH (the wrapper never batches — one tool
// call, one intent) with the idempotency machinery and the §7.4 ambiguity
// retry: on 400 ambiguous_input the runner re-reads the object, resolves
// the named block refs to retained full ids, and retries once.
func (r *Runner) patchOps(ctx context.Context, session *Session, spaceId, objectId string, op map[string]any, refFields []string) (*v2model.EditResult, error) {
	path := "/v2/spaces/" + seg(spaceId) + "/objects/" + seg(objectId)
	query := r.mutationQuery()
	body := map[string]any{"ops": []any{op}}
	key := r.mutationKey(session, requestHash("PATCH", path, query, body))
	send := func() (*v2model.EditResult, error) {
		var result v2model.EditResult
		err := r.client.decode(ctx, apiRequest{
			method:         "PATCH",
			path:           path,
			query:          query,
			body:           body,
			idempotencyKey: key,
			ifMatch:        r.IfMatch,
		}, &result)
		if err != nil {
			return nil, translateOpsError(err)
		}
		return &result, nil
	}
	result, err := send()
	if err == nil || !isAmbiguous(err) || len(refFields) == 0 {
		return result, err
	}
	// ambiguity retry: re-read the object and retry ONCE when the refs now
	// resolve (see retryAmbiguous — this self-heals the concurrent-
	// modification race, not a genuinely ambiguous label)
	retried, retryErr := r.retryAmbiguous(ctx, session, spaceId, objectId, op, refFields)
	if retryErr != nil || !retried {
		return nil, err // surface the original ambiguity, it names the ref
	}
	// the rewrite changed the body, so re-stamp the SAME key onto the new
	// request identity: the C8 store never cached the failed 400, and a
	// later identical call that resolves client-side (labels were retained
	// by the re-read) must replay, not re-apply
	if lw := session.LastWrite; lw != nil && lw.Key == key {
		lw.Hash = requestHash("PATCH", path, query, body)
		lw.At = r.now()
	}
	return send()
}

// retryAmbiguous re-reads the object and rewrites op's ref fields to full
// ids where the suffix now resolves uniquely, reporting whether anything
// changed. Scope, stated honestly: a ref RETAINED in the session never gets
// here — resolveBlockRef resolved it to a full id before the send (§7.4's
// retained-id promise, honored up front). So the 400 means the ref was not
// retained (an outline label, a pruned session), and re-resolving it by
// suffix over the CURRENT document self-heals exactly one case: the
// concurrent-modification race, where the reference was ambiguous when the
// PATCH ran but is unique against the re-read document (background sync
// removed the collision). A ref that is STILL ambiguous on re-read is
// unresolvable in principle — the wrapper cannot know which block the model
// meant — and the server's error surfaces untouched. The re-read's labels
// are retained either way, so the model's next read/edit starts resolved.
func (r *Runner) retryAmbiguous(ctx context.Context, session *Session, spaceId, objectId string, op map[string]any, refFields []string) (bool, error) {
	doc, err := r.client.raw(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/objects/" + seg(objectId),
	})
	if err != nil {
		return false, err
	}
	_, labels := relabelDoc(doc)
	if labels != nil {
		session.setLabels(objectId, labels)
	}
	fullIds := make([]string, 0, len(labels))
	for _, id := range labels {
		fullIds = append(fullIds, id)
	}
	changed := false
	for _, field := range refFields {
		ref, _ := op[field].(string)
		if ref == "" || fullBlockIdRe.MatchString(ref) {
			continue
		}
		var match string
		count := 0
		for _, id := range fullIds {
			if strings.HasSuffix(id, ref) {
				match = id
				count++
			}
		}
		if count == 1 && match != ref {
			op[field] = match
			changed = true
		}
	}
	return changed, nil
}

// opsVocab maps the op-path vocabulary of server error texts back onto the
// tool-argument vocabulary the model actually speaks: the wrapper renames
// `inside`→`under`, `id`→`block`, `tableId`→`table` and strips the ops[0]
// prefix (the wrapper never batches), and the REST-route repair hints become
// tool repairs. Without this, a model that follows the server's own hint
// (`retry with inside`) earns the wrapper's rejection — two dead retries on
// the most common anchoring mistake. Order matters: longest keys first.
var opsVocab = []struct{ from, to string }{
	{"ops[0].inside", "under"},
	{"ops[0].tableId", "table"},
	{"ops[0].id", "block"},
	{"ops[0].", ""},
	{"GET the object with ?outline=true to list block ids", "run read with mode=outline to list the block labels"},
}

// translateOpsError rewrites a *ToolError's text and issue paths from the op
// vocabulary to the tool vocabulary. Non-ToolErrors pass through.
func translateOpsError(err error) error {
	var te *ToolError
	if !errors.As(err, &te) {
		return err
	}
	for _, sub := range opsVocab {
		te.Text = strings.ReplaceAll(te.Text, sub.from, sub.to)
	}
	for i := range te.Issues {
		for _, sub := range opsVocab {
			te.Issues[i].Path = strings.ReplaceAll(te.Issues[i].Path, sub.from, sub.to)
			te.Issues[i].Message = strings.ReplaceAll(te.Issues[i].Message, sub.from, sub.to)
		}
	}
	return te
}

// targetLabel names the resolved mutation target for the receipt: the
// handle's object NAME when the reference was a handle, the raw reference
// otherwise. Every mutation receipt must identify what was written — a find
// between composing and running a call silently renumbers the handles, and
// an anonymous "ok" would hide the mis-address from both the model and a
// human reading the transcript.
func targetLabel(session *Session, ref string) string {
	if handleRe.MatchString(ref) {
		n, _ := strconv.Atoi(ref)
		if h, ok := session.handle(n); ok {
			if h.Name != "" {
				return fmt.Sprintf("%q", h.Name)
			}
			return h.Id
		}
	}
	return ref
}

// editSummary renders an edit result as the compact receipt text, naming
// the object that was written.
func editSummary(target string, result *v2model.EditResult) string {
	var parts []string
	stats := result.DiffStats
	add := func(n int, what string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, what))
		}
	}
	add(stats.BlocksAdded, "added")
	add(stats.BlocksRemoved, "removed")
	add(stats.BlocksChanged, "changed")
	add(stats.BlocksMoved, "moved")
	add(stats.PropertiesChanged, "properties changed")
	if len(parts) == 0 {
		parts = append(parts, "no changes")
	}
	line := fmt.Sprintf("ok — %s: %s", target, strings.Join(parts, ", "))
	if result.DryRun {
		line = fmt.Sprintf("dry run — %s: would apply %s", target, strings.Join(parts, ", "))
	}
	for _, w := range result.Warnings {
		line += "\nwarning: " + w.Message
	}
	return line
}
