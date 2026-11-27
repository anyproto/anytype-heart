package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

const (
	expectAbsent = "absent"
	lockWait     = 10 * time.Second
	maxPayload   = 16 << 10
)

// verbTimeout bounds a key verb end to end: reading stdin, taking the
// lock and the store's I/O. commitGrace is how long a write that is
// already replacing the item when the deadline passes gets to finish
// before the verb reports it unconfirmed. Variables so tests can shorten
// them.
var (
	verbTimeout = 30 * time.Second
	commitGrace = 5 * time.Second
)

var stampRe = regexp.MustCompile(`^[0-9a-f]{16}@[1-9][0-9]{0,17}$`)

// keyResult is the one JSON line a mutating verb prints. Unconfirmed is a
// write the store took whose confirmation failed: the item may hold the
// new state, so the caller must reconcile with key check before it
// releases or revokes anything. It never comes with exit 7.
type keyResult struct {
	OK          bool   `json:"ok"`
	Unconfirmed bool   `json:"unconfirmed,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Rev         int64  `json:"rev,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Store       string `json:"store,omitempty"`
}

// checkResult is key check's line. Present is null when the store could
// not say (exit 7).
type checkResult struct {
	Present     *bool  `json:"present"`
	Invalid     bool   `json:"invalid,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Rev         int64  `json:"rev,omitempty"`
	Account     string `json:"account,omitempty"`
	Port        int    `json:"port,omitempty"`
	Tier        string `json:"tier,omitempty"`
	Store       string `json:"store"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
}

type keyEnv struct {
	st     store.Store
	stdout io.Writer
	stderr io.Writer

	// the verb's deadline: once expired, the verb's own goroutine prints
	// nothing more and replaces nothing; committing means it reached the
	// point of replacing or removing the item before the deadline
	mu         sync.Mutex
	expired    bool
	committing bool
}

// emit writes the one JSON line and the stderr note. The caller holds mu.
func (e *keyEnv) emit(v any, note string) {
	line, err := json.Marshal(v)
	if err != nil {
		line = []byte(`{"ok":false,"reason":"internal error"}`)
	}
	fmt.Fprintln(e.stdout, string(line))
	if note != "" {
		fmt.Fprintf(e.stderr, "anytype-mcp: %s\n", note)
	}
}

func (e *keyEnv) print(v any) { e.say(v, "") }

// say is how the verb speaks: never after its deadline.
func (e *keyEnv) say(v any, note string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.expired {
		return
	}
	e.emit(v, note)
}

// mayCommit is the store's commit gate: the item is replaced or removed
// only if the deadline has not passed.
func (e *keyEnv) mayCommit() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.expired {
		return false
	}
	e.committing = true
	return true
}

// bounded runs a verb under its deadline. When the deadline passes before
// the verb reached its commit point, nothing was and nothing will be
// written: exit 7. When it passes while the item is being replaced, the
// verb gets a short grace to finish; after that the outcome is reported
// as unconfirmed, never as "nothing changed". The caller's os.Exit ends
// whatever is still stuck, and the lock dies with the process.
func (e *keyEnv) bounded(verb func() int) int {
	done := make(chan int, 1)
	go func() { done <- verb() }()
	timer := time.NewTimer(verbTimeout)
	defer timer.Stop()
	select {
	case code := <-done:
		return code
	case <-timer.C:
	}
	e.mu.Lock()
	if !e.committing {
		e.expired = true
		const reason = "timed out: nothing was changed, try again"
		e.emit(keyResult{Reason: reason, Store: store.ReasonUnreachable}, reason)
		e.mu.Unlock()
		return exitTemporary
	}
	e.mu.Unlock()
	grace := time.NewTimer(commitGrace)
	defer grace.Stop()
	select {
	case code := <-done:
		return code
	case <-grace.C:
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.expired = true
	const reason = "timed out while the item was being changed: check before retrying"
	e.emit(keyResult{Unconfirmed: true, Reason: reason}, reason)
	return exitInternal
}

// fail prints the failure line. Nothing here ever carries the key.
func (e *keyEnv) fail(code int, reason string, storeState string) int {
	e.say(keyResult{Reason: reason, Store: storeState}, reason)
	return code
}

// storeState names a store failure for the JSON line: denied (a folder or
// the item failed a safety rule) or unreachable. Both are exit 7: nothing
// was read or written.
func storeState(err error) (string, int) {
	var t *store.TemporaryError
	switch {
	case err == nil:
		return "ok", exitOK
	case errors.As(err, &t):
		return t.Reason, exitTemporary
	}
	return "error", exitInternal
}

func oneLine(err error) string { return strings.ReplaceAll(err.Error(), "\n", " ") }

// storeFail reports a store failure, with one stderr line that names the
// rule or the error (never a value).
func (e *keyEnv) storeFail(err error) int {
	state, code := storeState(err)
	reason := "the item store failed"
	if code == exitTemporary {
		reason = "the item store is not ready (" + state + "): nothing was changed, try again"
	}
	e.say(keyResult{Reason: reason, Store: state}, oneLine(err)+"\nanytype-mcp: "+reason)
	return code
}

// unconfirmed reports a write or a delete the store took but could not
// confirm.
func (e *keyEnv) unconfirmed(err error) int {
	const reason = "the item was changed but that could not be confirmed: check before retrying"
	e.say(keyResult{Unconfirmed: true, Reason: reason}, reason+" ("+oneLine(err)+")")
	return exitInternal
}

// current reads the item. absent covers both no item and a stored value
// that is not a valid item: a later key set replaces either with a new
// private file.
func current(ctx context.Context, st store.Store, id string) (it store.Item, absent bool, err error) {
	raw, err := st.Read(ctx, id)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrInvalidItem) {
		return store.Item{}, true, nil
	}
	if err != nil {
		return store.Item{}, false, err
	}
	it, err = store.DecodeItem(raw)
	if err != nil {
		return store.Item{}, true, nil
	}
	return it, false, nil
}

// withLock holds the connection's exclusive lock across a verb's read,
// compare, write and read-back. It serializes cooperating Anytype
// profiles; it does not stop other programs.
func (e *keyEnv) withLock(ctx context.Context, id string, fn func() int) int {
	lctx, cancel := context.WithTimeout(ctx, lockWait)
	defer cancel()
	unlock, err := e.st.Lock(lctx, id)
	if err != nil {
		return e.storeFail(err)
	}
	defer unlock()
	return fn()
}

type keyFlags struct {
	connection string
	expect     string
	account    string
	port       int
	tier       string
}

func keyCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	verb := args[0]
	switch verb {
	case "set", "retarget", "delete", "check":
	default:
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	var f keyFlags
	fs := newFlags("key "+verb, stderr)
	fs.StringVar(&f.connection, "connection", "", "connection id")
	if verb != "check" {
		fs.StringVar(&f.expect, "expect", "", "precondition: absent or <fp>@<rev>")
	}
	if verb == "retarget" || verb == "delete" {
		fs.StringVar(&f.account, "account", "", "the owning account id")
	}
	if verb == "retarget" {
		fs.IntVar(&f.port, "port", 0, "the new port")
		fs.StringVar(&f.tier, "tier", "", "the new tier")
	}
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return exitUsage
	}
	if !store.ValidConnectionID(f.connection) {
		return usageError(stderr, "--connection must match ^[a-z0-9-]{1,64}$")
	}
	switch verb {
	case "set":
		if f.expect != expectAbsent && !stampRe.MatchString(f.expect) {
			return usageError(stderr, "--expect must be absent or <fp>@<rev>")
		}
	case "retarget", "delete":
		if !stampRe.MatchString(f.expect) {
			return usageError(stderr, "--expect must be <fp>@<rev>")
		}
		if f.account == "" {
			return usageError(stderr, "--account is required")
		}
	}
	if verb == "retarget" && (f.port < 1 || f.port > 65535 || !store.ValidTier(f.tier)) {
		return usageError(stderr, "--port and --tier are required")
	}

	e := &keyEnv{st: store.Open(namespace), stdout: stdout, stderr: stderr}
	return e.run(verb, f, stdin)
}

// run executes one verb under the verb deadline, which covers reading
// stdin, taking the lock and every store call.
func (e *keyEnv) run(verb string, f keyFlags, stdin io.Reader) int {
	ctx, cancel := context.WithTimeout(context.Background(), verbTimeout)
	defer cancel()
	ctx = store.WithCommitGate(ctx, e.mayCommit)
	return e.bounded(func() int {
		switch verb {
		case "check":
			return e.check(ctx, f)
		case "set":
			// the payload is read and judged before the lock is taken
			payload, code := readPayload(stdin, e)
			if code != exitOK {
				return code
			}
			return e.withLock(ctx, f.connection, func() int { return e.set(ctx, f, payload) })
		case "retarget":
			return e.withLock(ctx, f.connection, func() int { return e.retarget(ctx, f) })
		default:
			return e.withLock(ctx, f.connection, func() int { return e.remove(ctx, f) })
		}
	})
}

type setPayload struct {
	Key     string `json:"key"`
	Account string `json:"account"`
	Port    int    `json:"port"`
	Tier    string `json:"tier"`
}

// readPayload takes exactly one complete JSON object from stdin: no more
// than maxPayload bytes, and nothing but white space after it. A payload
// cut short or followed by anything else is invalid input.
func readPayload(stdin io.Reader, e *keyEnv) (setPayload, int) {
	var p setPayload
	raw, err := io.ReadAll(io.LimitReader(stdin, maxPayload+1))
	if err != nil {
		return p, e.fail(exitInvalidInput, "the payload could not be read", "")
	}
	if len(raw) > maxPayload {
		return p, e.fail(exitInvalidInput, "the payload is too large", "")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, e.fail(exitInvalidInput, "the payload is not {key, account, port, tier}", "")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return p, e.fail(exitInvalidInput, "the payload is followed by something else", "")
	}
	// judged with the largest revision, so what is accepted here can
	// always be stored
	probe := store.Item{V: store.ItemVersion, Rev: 1 << 62, Key: p.Key, Account: p.Account, Port: p.Port, Tier: p.Tier}
	if _, err := probe.Encode(); err != nil {
		return p, e.fail(exitInvalidInput, "the payload is not valid: "+err.Error(), "")
	}
	return p, exitOK
}

// commit writes the item, reads it back and prints the receipt. Exit 7
// means nothing was written. Once the store has taken the write, any
// failure is "unconfirmed" with exit 1, never "try again".
func (e *keyEnv) commit(ctx context.Context, id string, next store.Item) int {
	raw, err := next.Encode()
	if err != nil {
		return e.fail(exitInvalidInput, "the item is not valid: "+err.Error(), "")
	}
	if err := e.st.Write(ctx, id, raw); err != nil {
		var taken *store.UnconfirmedError
		if !errors.As(err, &taken) {
			// the store proved the item was not replaced
			return e.storeFail(err)
		}
		// an error after the item may have been replaced: the item itself
		// says what happened. In place is ok; anything else is
		// unconfirmed, never "nothing changed".
		if back, absent, rerr := current(ctx, e.st, id); rerr != nil || absent || back != next {
			return e.unconfirmed(err)
		}
	}
	back, absent, err := current(ctx, e.st, id)
	if err != nil {
		return e.unconfirmed(err)
	}
	if absent || back != next {
		return e.unconfirmed(errors.New("the item read back differs from what was written"))
	}
	e.print(keyResult{OK: true, Fingerprint: next.Fingerprint(), Rev: next.Rev})
	return exitOK
}

func (e *keyEnv) set(ctx context.Context, f keyFlags, p setPayload) int {
	cur, absent, err := current(ctx, e.st, f.connection)
	if err != nil {
		return e.storeFail(err)
	}
	if (f.expect == expectAbsent) != absent || (!absent && cur.Stamp() != f.expect) {
		return e.fail(exitPrecondition, "precondition failed: the item is not in the expected state", "")
	}
	next := store.Item{V: store.ItemVersion, Rev: cur.Rev + 1, Key: p.Key, Account: p.Account, Port: p.Port, Tier: p.Tier}
	return e.commit(ctx, f.connection, next)
}

// owned checks a retarget's or a delete's precondition: the state and the
// owning account.
func (e *keyEnv) owned(ctx context.Context, f keyFlags) (store.Item, int) {
	cur, absent, err := current(ctx, e.st, f.connection)
	if err != nil {
		return cur, e.storeFail(err)
	}
	if absent {
		return cur, e.fail(exitNoItem, "no item for this connection", "")
	}
	if cur.Stamp() != f.expect || cur.Account != f.account {
		return cur, e.fail(exitPrecondition, "precondition failed: the item is not in the expected state", "")
	}
	return cur, exitOK
}

func (e *keyEnv) retarget(ctx context.Context, f keyFlags) int {
	cur, code := e.owned(ctx, f)
	if code != exitOK {
		return code
	}
	next := cur
	next.Rev, next.Port, next.Tier = cur.Rev+1, f.port, f.tier
	return e.commit(ctx, f.connection, next)
}

func (e *keyEnv) remove(ctx context.Context, f keyFlags) int {
	if _, code := e.owned(ctx, f); code != exitOK {
		return code
	}
	if err := e.st.Delete(ctx, f.connection); err != nil {
		var taken *store.UnconfirmedError
		switch {
		case errors.Is(err, store.ErrNotFound):
			return e.fail(exitNoItem, "no item for this connection", "")
		case errors.As(err, &taken):
			// an error after the item may have been removed: gone is ok,
			// anything else is unconfirmed
			if _, absent, rerr := current(ctx, e.st, f.connection); rerr != nil || !absent {
				return e.unconfirmed(err)
			}
		default:
			return e.storeFail(err)
		}
	}
	e.print(keyResult{OK: true})
	return exitOK
}

// check prints the item's public half, or the store's state when there is
// no item. It never prints the key.
func (e *keyEnv) check(ctx context.Context, f keyFlags) int {
	out := checkResult{Kind: e.st.Kind(), Namespace: store.NormalizeNamespace(namespace)}
	yes, no := true, false
	raw, err := e.st.Read(ctx, f.connection)
	var it store.Item
	if err == nil {
		it, err = store.DecodeItem(raw)
	}
	switch {
	case err == nil:
		out.Store, out.Present = "ok", &yes
		out.Fingerprint, out.Rev, out.Account, out.Port, out.Tier = it.Fingerprint(), it.Rev, it.Account, it.Port, it.Tier
		e.print(out)
		return exitOK
	case errors.Is(err, store.ErrInvalidItem):
		out.Store, out.Present, out.Invalid = "ok", &no, true
		e.print(out)
		return exitNoItem
	case errors.Is(err, store.ErrNotFound):
		// no item: say whether the store could take one
		state, code := storeState(e.st.Probe(ctx))
		out.Store = state
		if code == exitOK {
			out.Present = &no
			e.print(out)
			return exitNoItem
		}
		e.print(out)
		return code
	}
	state, code := storeState(err)
	out.Store = state
	e.say(out, oneLine(err))
	return code
}
