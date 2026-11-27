package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

const (
	expectAbsent  = "absent"
	lockWait      = 10 * time.Second
	verbTimeout   = 30 * time.Second
	promptTimeout = 3 * time.Minute
	maxPayload    = 16 << 10
)

var stampRe = regexp.MustCompile(`^[0-9a-f]{16}@[1-9][0-9]{0,17}$`)

// keyResult is the one JSON line a mutating verb prints.
type keyResult struct {
	OK          bool   `json:"ok"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Rev         int64  `json:"rev,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Store       string `json:"store,omitempty"`
}

// checkResult is key check's line. Present is null when the store could
// not say.
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
}

func (e *keyEnv) print(v any) {
	line, err := json.Marshal(v)
	if err != nil {
		line = []byte(`{"ok":false,"reason":"internal error"}`)
	}
	fmt.Fprintln(e.stdout, string(line))
}

// fail prints the failure line and maps a store error to its exit code.
// Nothing here ever carries the key.
func (e *keyEnv) fail(code int, reason string, storeState string) int {
	e.print(keyResult{Reason: reason, Store: storeState})
	fmt.Fprintf(e.stderr, "anytype-mcp: %s\n", reason)
	return code
}

// storeState names a store failure for the JSON line: none, locked,
// denied, unreachable, not-ready.
func storeState(err error) (string, int) {
	var t *store.TemporaryError
	switch {
	case err == nil:
		return "ok", exitOK
	case errors.Is(err, store.ErrNoStore):
		return "none", exitNoStore
	case errors.As(err, &t):
		return t.Reason, exitTemporary
	}
	return "error", exitInternal
}

func (e *keyEnv) storeFail(err error) int {
	state, code := storeState(err)
	reason := "the key store failed"
	switch code {
	case exitNoStore:
		reason = "this system has no key store"
	case exitTemporary:
		reason = "the key store is not ready (" + state + "): try again"
	}
	return e.fail(code, reason, state)
}

// current reads the item. absent covers both no item and a stored value
// that is not a valid item: a later key set replaces either.
func current(ctx context.Context, st store.Store, id string, interactive bool) (it store.Item, absent bool, err error) {
	raw, err := st.Read(ctx, id, interactive)
	if errors.Is(err, store.ErrNotFound) {
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

func lockDir(over overrides) (string, error) {
	if over.lockDir != "" {
		return over.lockDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".anytype", "ai-connect"), nil
}

// withLock holds the connection's exclusive lock across a verb's read,
// compare, write and read-back. It serializes cooperating Anytype
// profiles; it does not stop other programs.
func withLock(ctx context.Context, over overrides, id string, fn func() int, busy func() int) int {
	dir, err := lockDir(over)
	if err == nil {
		err = os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return busy()
	}
	lock := flock.New(filepath.Join(dir, id+".lock"))
	lctx, cancel := context.WithTimeout(ctx, lockWait)
	defer cancel()
	ok, err := lock.TryLockContext(lctx, 25*time.Millisecond)
	if err != nil || !ok {
		return busy()
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			// the lock dies with the process either way
			return
		}
	}()
	return fn()
}

type keyFlags struct {
	connection  string
	expect      string
	account     string
	port        int
	tier        string
	interactive bool
	over        overrides
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
	if verb == "set" {
		fs.BoolVar(&f.interactive, "interactive", false, "allow the OS to show an unlock prompt")
	}
	f.over.register(fs, false)
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

	timeout := verbTimeout
	if f.interactive {
		timeout = promptTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	e := &keyEnv{st: store.Open(namespace, f.over.storeDir), stdout: stdout, stderr: stderr}
	busy := func() int { return e.fail(exitTemporary, "another Anytype is changing this connection: try again", "") }
	switch verb {
	case "check":
		return e.check(ctx, f)
	case "set":
		// the payload is read before the lock is taken: one stdin line
		payload, code := readPayload(stdin, e)
		if code != exitOK {
			return code
		}
		return withLock(ctx, f.over, f.connection, func() int { return e.set(ctx, f, payload) }, busy)
	case "retarget":
		return withLock(ctx, f.over, f.connection, func() int { return e.retarget(ctx, f) }, busy)
	default:
		return withLock(ctx, f.over, f.connection, func() int { return e.remove(ctx, f) }, busy)
	}
}

type setPayload struct {
	Key     string `json:"key"`
	Account string `json:"account"`
	Port    int    `json:"port"`
	Tier    string `json:"tier"`
}

func readPayload(stdin io.Reader, e *keyEnv) (setPayload, int) {
	var p setPayload
	line, err := bufio.NewReader(io.LimitReader(stdin, maxPayload)).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return p, e.fail(exitInvalidInput, "the payload could not be read", "")
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, e.fail(exitInvalidInput, "the payload is not {key, account, port, tier}", "")
	}
	probe := store.Item{V: store.ItemVersion, Rev: 1, Key: p.Key, Account: p.Account, Port: p.Port, Tier: p.Tier}
	if err := probe.Validate(); err != nil {
		return p, e.fail(exitInvalidInput, "the payload is not valid: "+err.Error(), "")
	}
	return p, exitOK
}

// commit writes the item, reads it back and prints the receipt.
func (e *keyEnv) commit(ctx context.Context, id string, next store.Item, interactive bool) int {
	raw, err := next.Encode()
	if err != nil {
		return e.fail(exitInvalidInput, "the item is not valid: "+err.Error(), "")
	}
	if err := e.st.Write(ctx, id, raw, interactive); err != nil {
		return e.storeFail(err)
	}
	back, absent, err := current(ctx, e.st, id, false)
	if err != nil {
		return e.storeFail(err)
	}
	if absent || back != next {
		return e.fail(exitInternal, "the item read back differs from what was written", "")
	}
	e.print(keyResult{OK: true, Fingerprint: next.Fingerprint(), Rev: next.Rev})
	return exitOK
}

func (e *keyEnv) set(ctx context.Context, f keyFlags, p setPayload) int {
	cur, absent, err := current(ctx, e.st, f.connection, f.interactive)
	if err != nil {
		return e.storeFail(err)
	}
	if (f.expect == expectAbsent) != absent || (!absent && cur.Stamp() != f.expect) {
		return e.fail(exitPrecondition, "precondition failed: the item is not in the expected state", "")
	}
	next := store.Item{V: store.ItemVersion, Rev: cur.Rev + 1, Key: p.Key, Account: p.Account, Port: p.Port, Tier: p.Tier}
	return e.commit(ctx, f.connection, next, f.interactive)
}

// owned checks a retarget's or a delete's precondition: the state and the
// owning account.
func (e *keyEnv) owned(ctx context.Context, f keyFlags) (store.Item, int) {
	cur, absent, err := current(ctx, e.st, f.connection, false)
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
	return e.commit(ctx, f.connection, next, false)
}

func (e *keyEnv) remove(ctx context.Context, f keyFlags) int {
	if _, code := e.owned(ctx, f); code != exitOK {
		return code
	}
	if err := e.st.Delete(ctx, f.connection); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return e.fail(exitNoItem, "no item for this connection", "")
		}
		return e.storeFail(err)
	}
	e.print(keyResult{OK: true})
	return exitOK
}

// check prints the item's public half, or the store's state when there is
// no item. It never prompts and never prints the key.
func (e *keyEnv) check(ctx context.Context, f keyFlags) int {
	out := checkResult{Kind: e.st.Kind(), Namespace: store.NormalizeNamespace(namespace)}
	yes, no := true, false
	raw, err := e.st.Read(ctx, f.connection, false)
	if err == nil {
		out.Store = "ok"
		it, derr := store.DecodeItem(raw)
		if derr != nil {
			out.Present, out.Invalid = &no, true
			e.print(out)
			return exitNoItem
		}
		out.Present = &yes
		out.Fingerprint, out.Rev, out.Account, out.Port, out.Tier = it.Fingerprint(), it.Rev, it.Account, it.Port, it.Tier
		e.print(out)
		return exitOK
	}
	if errors.Is(err, store.ErrNotFound) {
		// no item: say whether the store could take one
		out.Present = &no
		state, code := storeState(e.st.Probe(ctx))
		out.Store = state
		e.print(out)
		if code == exitOK {
			return exitNoItem
		}
		return code
	}
	state, code := storeState(err)
	out.Store = state
	e.print(out)
	if code == exitInternal {
		fmt.Fprintln(e.stderr, "anytype-mcp: the key store failed: "+strings.ReplaceAll(err.Error(), "\n", " "))
	}
	return code
}
