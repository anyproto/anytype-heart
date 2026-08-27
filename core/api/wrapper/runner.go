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
	// AllowNewOptions is the caller's consent to MINT select options for
	// names a property does not hold yet. It does two things, and needs to
	// do both to mean anything: it skips the wrapper's own option-name
	// pre-validation (the A2 guard), and it sends ?create_missing_options=true so
	// the server permits the mint. Without the second half the flag would
	// only move the refusal from the client to the server.
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
	err = r.steerError(ctx, def, session, args, err)
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
			switch {
			case len(session.Handles) > 0:
				return "", "", fmt.Errorf("no handle %d — the last find returned %d results; run find again to renumber", n, len(session.Handles))
			case session.Space != "":
				// a working space with no handles: the last find numbered
				// nothing — either it matched nothing, or it named no criteria
				// and listed the space (§8.33). errNoSession's "run find first"
				// would be false here (find HAS run) and reads as a repair
				// already tried
				return "", "", fmt.Errorf("no handle %d — the last find numbered nothing: a find with only a space lists without numbering, and a search with no matches numbers nothing. Run find with query, type or filter", n)
			default:
				return "", "", errNoSession(fmt.Sprintf("handle %d", n))
			}
		}
		return session.Space, h.Id, nil
	}
	if session.Space == "" {
		return "", "", errNoSession(fmt.Sprintf("object %q", ref))
	}
	return session.Space, ref, nil
}

//
// ---- served ids ----
//

// fullBlockIdRe recognises a full editor-minted block/row/column id (bson
// hex). The server's relabeler now uses the same minted-shape predicate —
// the wrapper had this rule first, and its client-side relabeling retired
// when the server started serving labels itself: block refs pass through
// verbatim, and the server resolves a label by exact id or unique suffix
// (C4). The retired label map was worse than dead weight: a stale map from
// a previous document version rewrote a just-read label into an outdated
// full id, which the server accepted as an exact match.
var fullBlockIdRe = regexp.MustCompile(`^[0-9a-f]{24}$`)

// servedLocalIds walks a served document and returns its local ids exactly
// as served — block ids plus table column and row ids (the ids set_cell
// takes). On a default read those are the server's compact labels for
// minted ids and the full spelling for everything else; either spelling
// resolves server-side.
func servedLocalIds(doc []byte) []string {
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
		return nil
	}
	var ids []string
	addId := func(id string) {
		if id != "" {
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
	return ids
}

//
// ---- mutation machinery ----
//

// withinReuseWindow reports whether a LastWrite stamped `at` may be reused
// at `now`: strictly younger than the window AND not from the future. The
// record is persisted in the CLI session file, so a backwards clock step
// (NTP, a session file moved between hosts) can put `at` ahead of `now` —
// without the floor, any negative age passed the `< window` check and
// revived an arbitrarily old key (and its recorded rewrite) until the clock
// caught back up.
func withinReuseWindow(now, at time.Time) bool {
	age := now.Sub(at)
	return age >= 0 && age < idempotencyReuseWindow
}

// mutationKey returns the Idempotency-Key for a resolved mutation request:
// reused from LastWrite when the identical request repeats within the reuse
// window (a retry — C8 replays it), fresh otherwise (an intentional repeat
// applies). hash comes from requestHash; now is the caller's ONE clock
// reading for the whole call, so every window judgment in the call agrees.
func (r *Runner) mutationKey(session *Session, hash string, now time.Time) string {
	if lw := session.LastWrite; lw != nil && lw.Hash == hash && withinReuseWindow(now, lw.At) {
		lw.At = now
		return lw.Key
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("random idempotency key: %w", err))
	}
	key := hex.EncodeToString(b[:])
	session.LastWrite = &LastWrite{Hash: hash, Key: key, At: now}
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
	q := url.Values{}
	if r.DryRun {
		q.Set("dry_run", "true")
	}
	if r.AllowNewOptions {
		q.Set("create_missing_options", "true")
	}
	if len(q) == 0 {
		return nil
	}
	return q
}

// patchOps sends a single-op PATCH (the wrapper never batches — one tool
// call, one intent) with the idempotency machinery and the §7.4 ambiguity
// retry: on 400 ambiguous_input the runner re-reads the object, rewrites the
// named block refs to the served id they uniquely tail-match, and retries
// once.
func (r *Runner) patchOps(ctx context.Context, session *Session, spaceId, objectId string, op map[string]any, refFields []string) (*v2model.EditResult, error) {
	path := "/v2/spaces/" + seg(spaceId) + "/objects/" + seg(objectId)
	query := r.mutationQuery()
	body := map[string]any{"ops": []any{op}}
	// ONE clock reading serves the whole call: the rewrite-replay gate below
	// and mutationKey used to read the clock separately, and ticking across
	// the window boundary between the two applied the recorded rewrite and
	// then minted a FRESH key for it — a rewritten body under a new identity.
	now := r.now()
	// an identical re-run of a call whose refs were rewritten mid-flight
	// (the ambiguity retry below) computes the PRE-rewrite hash; the server
	// bound the key to the REWRITTEN body, so the re-run must reproduce the
	// rewrite to replay instead of 409ing or re-applying
	if lw := session.LastWrite; lw != nil && len(lw.Rewrites) > 0 &&
		lw.PriorHash == requestHash("PATCH", path, query, body) && withinReuseWindow(now, lw.At) {
		for field, resolved := range lw.Rewrites {
			op[field] = resolved // op backs body, so the hash below sees this
		}
	}
	key := r.mutationKey(session, requestHash("PATCH", path, query, body), now)
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
	priorHash := session.LastWrite.Hash
	result, err := send()
	if err == nil || !isAmbiguous(err) || len(refFields) == 0 {
		return result, err
	}
	// ambiguity retry: re-read the object and retry ONCE when the refs now
	// resolve (see retryAmbiguous — this self-heals the concurrent-
	// modification race, not a genuinely ambiguous label)
	rewrites, retryErr := r.retryAmbiguous(ctx, spaceId, objectId, op, refFields)
	if retryErr != nil || len(rewrites) == 0 {
		return nil, err // surface the original ambiguity, it names the ref
	}
	// the rewrite changed the body, so re-stamp the SAME key onto the new
	// request identity — the C8 store never cached the failed 400 — and
	// record the rewrite so a later identical call replays, not re-applies
	if lw := session.LastWrite; lw != nil && lw.Key == key {
		// PriorHash stays the FIRST pre-rewrite hash: on a chained second
		// rewrite (the replayed body went ambiguous again) the current
		// lw.Hash is itself a rewritten hash, and capturing IT orphaned the
		// original request identity — a third identical run re-applied under
		// a fresh key instead of replaying (reproduced double-apply)
		if lw.PriorHash == "" {
			lw.PriorHash = priorHash
		}
		// merged, not replaced: a chained retry may rewrite a subset of the
		// ref fields, and a later run must reproduce the WHOLE chain
		if lw.Rewrites == nil {
			lw.Rewrites = map[string]string{}
		}
		for field, resolved := range rewrites {
			lw.Rewrites[field] = resolved
		}
		lw.Hash = requestHash("PATCH", path, query, body)
		lw.At = r.now()
	}
	return send()
}

// retryAmbiguous re-reads the object and rewrites op's ref fields to the
// served id they now uniquely tail-match, returning the rewrites it made.
// The pool is the re-read document's own served ids (servedLocalIds) —
// never a session-retained map, which could be stale against the current
// document. This self-heals exactly one case: the concurrent-modification
// race, where the reference was ambiguous when the PATCH ran but is unique
// against the re-read document (background sync removed the collision). A
// ref that is STILL ambiguous on re-read is unresolvable in principle — the
// wrapper cannot know which block the model meant — and the server's error
// surfaces untouched.
func (r *Runner) retryAmbiguous(ctx context.Context, spaceId, objectId string, op map[string]any, refFields []string) (map[string]string, error) {
	doc, err := r.client.raw(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/objects/" + seg(objectId),
	})
	if err != nil {
		return nil, err
	}
	servedIds := servedLocalIds(doc)
	var rewrites map[string]string
	for _, field := range refFields {
		ref, _ := op[field].(string)
		if ref == "" || fullBlockIdRe.MatchString(ref) {
			continue
		}
		var match string
		count := 0
		for _, id := range servedIds {
			if strings.HasSuffix(id, ref) {
				match = id
				count++
			}
		}
		if count == 1 && match != ref {
			op[field] = match
			if rewrites == nil {
				rewrites = map[string]string{}
			}
			rewrites[field] = match
		}
	}
	return rewrites, nil
}

// opsVocab maps the op-path vocabulary of server error texts back onto the
// tool-argument vocabulary the model actually speaks: the wrapper renames
// `inside`→`under`, `id`→`block`, `table_id`→`table` and strips the ops[0]
// prefix (the wrapper never batches). Without this, a model that follows the
// server's own hint (`retry with inside`) earns the wrapper's rejection — two
// dead retries on the most common anchoring mistake. Order matters: longest
// keys first. The REST-route repair hints are NOT here: they arrive on every
// tool, not just the ops path, and steer.go re-spells them in one place for
// all of them (§8.34).
var opsVocab = []struct{ from, to string }{
	{"ops[0].inside", "under"},
	{"ops[0].table_id", "table"},
	{"ops[0].id", "block"},
	{"ops[0].", ""},
	// edit_text deliberately has no replace_all (§8.6) — the server's escape
	// hint would steer the model into an argument the tool rejects
	{`, or set "replace_all": true`, ""},
	// the find-as-locator refusals (§8.43) name the op's id slot; the tool's
	// slot is `block` — same §8.21-measured refusal, re-spelled
	{"retry with id naming", "retry with block naming"},
	{"or give the block id", "or pass block"},
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
			// Hint too — deRest already rewrites it, and the locator refusals
			// (§8.43) put the repair in the Hint: without this line the
			// "or give the block id" row can never fire, and a model
			// following the un-rewritten hint emits a schema-invalid call
			// (edit_text's slot is `block`; wrapper schemas are
			// additionalProperties:false).
			te.Issues[i].Hint = strings.ReplaceAll(te.Issues[i].Hint, sub.from, sub.to)
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
