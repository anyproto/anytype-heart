package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

const canary = "CANARY-cli-key-0123456789abcdefghijklmnopq"

var (
	buildOnce sync.Once
	buildDir  string
	buildErr  error
	// goEnv is what the go tool needs once HOME points at a scratch folder
	goEnv []string
)

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// TestMain is the scratch guard: before any test runs, HOME and
// USERPROFILE point at a fresh temp folder, so a test that forgets to give
// a child its own home still can't reach the developer's. The go tool's
// own folders are pinned first, because they default to the home.
func TestMain(m *testing.M) {
	os.Exit(guarded(m))
}

func guarded(m *testing.M) int {
	if goBin, err := exec.LookPath("go"); err == nil {
		for _, name := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOROOT", "GOFLAGS", "GOPROXY", "GOPRIVATE", "GOTOOLCHAIN"} {
			out, err := exec.Command(goBin, "env", name).Output()
			if err == nil {
				value := strings.TrimSpace(string(out))
				goEnv = append(goEnv, name+"="+value)
				_ = os.Setenv(name, value)
			}
		}
	}
	scratch, err := os.MkdirTemp("", "anytype-mcp-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "scratch home:", err)
		return 1
	}
	defer os.RemoveAll(scratch)
	_ = os.Setenv("HOME", scratch)
	_ = os.Setenv("USERPROFILE", scratch)
	code := m.Run()
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
	return code
}

func TestScratchGuard(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Contains(t, home, "anytype-mcp-home-", "the tests' own home is a scratch folder")
	assert.Contains(t, store.Open("dev").Dir(), "anytype-mcp-home-")
}

// binaries builds the two shipped variants, once: the default build and
// the release-namespace build. There is no test build.
func binaries(t *testing.T) (dev, release string) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH")
	}
	buildOnce.Do(func() {
		buildDir, buildErr = os.MkdirTemp("", "anytype-mcp-test-")
		if buildErr != nil {
			return
		}
		for _, b := range [][]string{
			{"dev"},
			{"release", "-ldflags", "-X main.linkNamespace=anytype-mcp-namespace:release -X main.version=1.2.3"},
		} {
			args := append([]string{"build", "-o", filepath.Join(buildDir, exe(b[0]))}, b[1:]...)
			if out, err := exec.Command("go", append(args, ".")...).CombinedOutput(); err != nil {
				buildErr = fmt.Errorf("go build %v: %v\n%s", b, err, out)
				return
			}
		}
	})
	require.NoError(t, buildErr)
	return filepath.Join(buildDir, exe("dev")), filepath.Join(buildDir, exe("release"))
}

type result struct {
	code   int
	stdout string
	stderr string
}

func (r result) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(r.stdout)), &m), "stdout: %q", r.stdout)
	return m
}

// env is one scratch home with its own item store.
type env struct {
	t    *testing.T
	bin  string
	home string
}

func newEnv(t *testing.T) *env {
	dev, _ := binaries(t)
	return &env{t: t, bin: dev, home: t.TempDir()}
}

// childEnv is the whole environment of a child: built from scratch, never
// from os.Environ.
func childEnv(home string) []string {
	return []string{"HOME=" + home, "USERPROFILE=" + home}
}

func (e *env) items() string { return filepath.Join(e.home, ".anytype", "ai-connect", "items-dev") }

func (e *env) run(stdin string, args ...string) result {
	e.t.Helper()
	return runIn(e.t, e.home, e.bin, stdin, args...)
}

func runIn(t *testing.T, home, bin, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = childEnv(home)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		require.True(t, ok, "run %v: %v", cmd.Args, err)
		code = ee.ExitCode()
	}
	for _, a := range cmd.Args {
		require.NotContains(t, a, canary, "the key must never be an argument")
	}
	for _, kv := range cmd.Env {
		require.NotContains(t, kv, canary, "the key must never be in the environment")
	}
	return result{code: code, stdout: out.String(), stderr: errb.String()}
}

func payload(key, account string, port int, tier string) string {
	b, _ := json.Marshal(map[string]any{"key": key, "account": account, "port": port, "tier": tier})
	return string(b) + "\n"
}

func (e *env) set(expect, key, account string, port int, tier string) result {
	return e.run(payload(key, account, port, tier), "key", "set", "--connection", "conn-a", "--expect", expect)
}

func (e *env) check() result { return e.run("", "key", "check", "--connection", "conn-a") }

func (e *env) retarget(expect, account, port, tier string) result {
	return e.run("", "key", "retarget", "--connection", "conn-a", "--expect", expect, "--account", account, "--port", port, "--tier", tier)
}

func (e *env) del(expect, account string) result {
	return e.run("", "key", "delete", "--connection", "conn-a", "--expect", expect, "--account", account)
}

func stamp(r map[string]any) string {
	return fmt.Sprintf("%s@%d", r["fingerprint"], int64(r["rev"].(float64)))
}

func TestVersionAndNamespaces(t *testing.T) {
	dev, release := binaries(t)
	home := t.TempDir()
	for _, tc := range []struct {
		bin, namespace, version string
	}{{dev, "dev", "dev"}, {release, "release", "1.2.3"}} {
		r := runIn(t, home, tc.bin, "", "version", "--json")
		require.Equal(t, 0, r.code)
		m := r.json(t)
		assert.Equal(t, tc.namespace, m["namespace"])
		assert.Equal(t, tc.version, m["version"])
		assert.Equal(t, "go", m["impl"])
		assert.EqualValues(t, Rev, m["rev"])
		assert.Len(t, m, 5, "version, commit, rev, namespace, impl and nothing else")
	}
	assert.Contains(t, runIn(t, home, dev, "", "version").stdout, "dev namespace")

	t.Run("dev and release write to different folders and can't see each other's item", func(t *testing.T) {
		set := func(bin, key string) result {
			return runIn(t, home, bin, payload(key, "A", 1, "small"), "key", "set", "--connection", "conn-a", "--expect", "absent")
		}
		check := func(bin string) result { return runIn(t, home, bin, "", "key", "check", "--connection", "conn-a") }
		require.Equal(t, exitOK, set(dev, canary).code)
		assert.Equal(t, exitNoItem, check(release).code)
		require.Equal(t, exitOK, set(release, canary+"r").code)
		assert.Equal(t, "release", check(release).json(t)["namespace"])
		assert.NotEqual(t, check(dev).json(t)["fingerprint"], check(release).json(t)["fingerprint"])
		base := filepath.Join(home, ".anytype", "ai-connect")
		for _, p := range []string{filepath.Join(base, "items-dev", "conn-a.item"), filepath.Join(base, "items", "conn-a.item")} {
			_, err := os.Stat(p)
			require.NoError(t, err, p)
		}
	})

	t.Run("there are no override flags and no interactive mode", func(t *testing.T) {
		for _, bin := range []string{dev, release} {
			for _, flag := range []string{"--port", "--tier", "--store-dir", "--lock-dir"} {
				assert.Equal(t, exitUsage, runIn(t, home, bin, "", "serve", "--connection", "x", flag, "1").code, flag)
				assert.Equal(t, exitUsage, runIn(t, home, bin, "", "key", "check", "--connection", "x", flag, "1").code, flag)
			}
			r := runIn(t, home, bin, payload("k", "A", 1, "small"), "key", "set", "--connection", "x", "--expect", "absent", "--interactive")
			assert.Equal(t, exitUsage, r.code)
		}
	})
}

func TestKeyVerbs(t *testing.T) {
	t.Run("set, check, retarget, delete", func(t *testing.T) {
		e := newEnv(t)
		r := e.check()
		assert.Equal(t, exitNoItem, r.code)
		assert.Equal(t, false, r.json(t)["present"])
		assert.Equal(t, "ok", r.json(t)["store"])
		assert.Equal(t, "file", r.json(t)["kind"])

		r = e.set("absent", canary, "AccountOne", 31009, "full")
		require.Equal(t, exitOK, r.code, r.stderr)
		first := r.json(t)
		assert.Equal(t, true, first["ok"])
		assert.Equal(t, store.Fingerprint(canary), first["fingerprint"])
		assert.EqualValues(t, 1, first["rev"])

		r = e.check()
		require.Equal(t, exitOK, r.code)
		c := r.json(t)
		assert.Equal(t, true, c["present"])
		assert.Equal(t, "AccountOne", c["account"])
		assert.EqualValues(t, 31009, c["port"])
		assert.Equal(t, "full", c["tier"])
		assert.Equal(t, "dev", c["namespace"])
		assert.Equal(t, stamp(first), stamp(c))

		r = e.retarget(stamp(first), "AccountOne", "40000", "full")
		require.Equal(t, exitOK, r.code, r.stderr)
		second := r.json(t)
		assert.EqualValues(t, 2, second["rev"])
		assert.Equal(t, first["fingerprint"], second["fingerprint"], "a retarget keeps the key")
		assert.EqualValues(t, 40000, e.check().json(t)["port"])

		// the same retarget again: its precondition is stale
		assert.Equal(t, exitPrecondition, e.retarget(stamp(first), "AccountOne", "40000", "full").code)

		// rotation
		r = e.set(stamp(second), canary+"2", "AccountOne", 40000, "full")
		require.Equal(t, exitOK, r.code, r.stderr)
		third := r.json(t)
		assert.EqualValues(t, 3, third["rev"])
		assert.NotEqual(t, second["fingerprint"], third["fingerprint"])

		assert.Equal(t, exitPrecondition, e.del(stamp(second), "AccountOne").code, "stale state")
		assert.Equal(t, exitPrecondition, e.del(stamp(third), "AccountTwo").code, "not the owner")
		assert.Equal(t, exitOK, e.check().code, "a refused delete changes nothing")
		r = e.del(stamp(third), "AccountOne")
		assert.Equal(t, exitOK, r.code)
		assert.Equal(t, true, r.json(t)["ok"])
		assert.Equal(t, exitNoItem, e.check().code)
		assert.Equal(t, exitNoItem, e.del(stamp(third), "AccountOne").code, "an absent item is exit 3")
	})

	t.Run("preconditions", func(t *testing.T) {
		e := newEnv(t)
		assert.Equal(t, exitPrecondition, e.set(store.Fingerprint(canary)+"@1", canary, "A", 1, "small").code, "expected an item, found none")
		require.Equal(t, exitOK, e.set("absent", canary, "A", 1, "small").code)
		assert.Equal(t, exitPrecondition, e.set("absent", canary, "A", 1, "small").code, "expected none, found one")
		assert.Equal(t, exitPrecondition, e.set(store.Fingerprint(canary)+"@2", canary, "A", 1, "small").code, "wrong rev")
		assert.Equal(t, exitPrecondition, e.set(store.Fingerprint("other")+"@1", canary, "A", 1, "small").code, "wrong fingerprint")
		assert.Equal(t, exitPrecondition, e.retarget(store.Fingerprint(canary)+"@1", "B", "2", "small").code, "not the owner")
		assert.EqualValues(t, 1, e.check().json(t)["rev"], "nothing changed")
	})

	t.Run("usage and invalid input", func(t *testing.T) {
		e := newEnv(t)
		for _, args := range [][]string{
			{},
			{"nope"},
			{"key"},
			{"key", "frobnicate"},
			{"key", "check"},
			{"key", "check", "--connection", "Bad_ID"},
			{"key", "check", "--connection", "../x"},
			{"key", "set", "--connection", "conn-a"},
			{"key", "set", "--connection", "conn-a", "--expect", "whatever"},
			{"key", "retarget", "--connection", "conn-a", "--expect", "absent", "--account", "A", "--port", "1", "--tier", "small"},
			{"key", "retarget", "--connection", "conn-a", "--expect", "0123456789abcdef@1", "--account", "A", "--port", "1", "--tier", "huge"},
			{"key", "delete", "--connection", "conn-a", "--expect", "0123456789abcdef@1"},
			{"key", "check", "--connection", "conn-a", "--key", "x"},
			{"key", "header", "--connection", "conn-a"},
			{"serve"},
			{"serve", "--connection", "conn-a", "extra"},
			{"version", "--nope"},
		} {
			assert.Equal(t, exitUsage, e.run("", args...).code, "%v", args)
		}
		good := strings.TrimSuffix(payload(canary, "A", 1, "small"), "\n")
		for name, in := range map[string]string{
			"empty":                 "",
			"not JSON":              "hello\n",
			"unknown field":         `{"key":"k","account":"A","port":1,"tier":"small","extra":1}` + "\n",
			"no key":                payload("", "A", 1, "small"),
			"bad tier":              payload("k", "A", 1, "medium"),
			"bad port":              payload("k", "A", 0, "small"),
			"no account":            payload("k", "", 1, "small"),
			"key with a newline":    `{"key":"a\nb","account":"A","port":1,"tier":"small"}` + "\n",
			"followed by x":         good + "x\n",
			"followed by an object": good + "\n" + good + "\n",
			"one byte too long":     good + strings.Repeat(" ", maxPayload+1-len(good)),
			"cut at the limit":      `{"key":"` + strings.Repeat("k", maxPayload),
		} {
			r := e.run(in, "key", "set", "--connection", "conn-a", "--expect", "absent")
			assert.Equal(t, exitInvalidInput, r.code, name)
			assert.NotContains(t, r.stdout+r.stderr, canary, name)
		}
		assert.Equal(t, exitNoItem, e.check().code, "nothing was written")
		_, err := os.Stat(filepath.Join(e.home, ".anytype"))
		assert.ErrorIs(t, err, os.ErrNotExist, "a refused payload creates nothing")
		r := e.run(good+"  \n\n", "key", "set", "--connection", "conn-a", "--expect", "absent")
		assert.Equal(t, exitOK, r.code, "trailing white space is fine")
	})

	t.Run("an invalid stored value counts as absent and is replaced by key set", func(t *testing.T) {
		e := newEnv(t)
		require.Equal(t, exitOK, e.run(payload("k", "A", 1, "small"), "key", "set", "--connection", "other", "--expect", "absent").code)
		good := strings.TrimSuffix(payload(canary, "A", 1, "small"), "\n")
		for _, bad := range []string{"garbage", `{"v":1,"rev":1,"key":"k","account":"A","port":1,"tier":"small"}x`, good, strings.Repeat("x", 5000)} {
			require.NoError(t, os.WriteFile(filepath.Join(e.items(), "conn-a.item"), []byte(bad), 0o600))
			r := e.check()
			assert.Equal(t, exitNoItem, r.code)
			assert.Equal(t, true, r.json(t)["invalid"])
			assert.Equal(t, false, r.json(t)["present"])
			r = e.set("absent", canary, "A", 1, "small")
			require.Equal(t, exitOK, r.code, r.stderr)
			assert.EqualValues(t, 1, r.json(t)["rev"])
		}
	})

	t.Run("the key is in no output and in no file but its own item", func(t *testing.T) {
		e := newEnv(t)
		outputs := []result{
			e.set("absent", canary, "A", 1, "small"),
			e.check(),
			e.set("absent", canary, "A", 1, "small"),
			e.run(`{"key":"`+canary+`","oops":1}`+"\n", "key", "set", "--connection", "conn-a", "--expect", "absent"),
			e.retarget(store.Fingerprint(canary)+"@1", "A", "2", "small"),
		}
		for _, r := range outputs {
			assert.NotContains(t, r.stdout, canary)
			assert.NotContains(t, r.stderr, canary)
		}
		var holders []string
		require.NoError(t, filepath.WalkDir(e.home, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			raw, err := os.ReadFile(path)
			if err == nil && bytes.Contains(raw, []byte(canary)) {
				holders = append(holders, path)
			}
			return nil
		}))
		assert.Equal(t, []string{filepath.Join(e.items(), "conn-a.item")}, holders)
	})
}

// TestStoreRulesThroughTheVerbs: a folder or an item that fails a safety
// rule is exit 7 for every verb, changes nothing, and names the rule on
// stderr; serve starts not connected.
func TestStoreRulesThroughTheVerbs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modes are a Unix rule; the Windows descriptors are tested in the store package")
	}
	all := func(e *env, st string) []result {
		return []result{
			e.check(),
			e.set(st, canary+"x", "A", 1, "small"),
			e.retarget(st, "A", "2", "small"),
			e.del(st, "A"),
		}
	}
	seeded := func(t *testing.T) (*env, string, []byte) {
		e := newEnv(t)
		require.Equal(t, exitOK, e.set("absent", canary, "A", 1, "small").code)
		raw, err := os.ReadFile(filepath.Join(e.items(), "conn-a.item"))
		require.NoError(t, err)
		return e, stamp(e.check().json(t)), raw
	}
	untouched := func(t *testing.T, e *env, before []byte) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(e.items(), "conn-a.item"))
		require.NoError(t, err)
		assert.Equal(t, before, raw, "the item is untouched")
	}
	serveNotConnected := func(t *testing.T, e *env) {
		t.Helper()
		r := e.run(initLine+"\n", "serve", "--connection", "conn-a")
		assert.Equal(t, exitOK, r.code)
		assert.Contains(t, r.stdout, `"code":-32001`)
	}

	for name, breakIt := range map[string]func(t *testing.T, e *env) (restore func()){
		"the items folder at 0755": func(t *testing.T, e *env) func() {
			require.NoError(t, os.Chmod(e.items(), 0o755))
			return func() { _ = os.Chmod(e.items(), 0o700) }
		},
		"a group-writable ancestor": func(t *testing.T, e *env) func() {
			dot := filepath.Join(e.home, ".anytype")
			require.NoError(t, os.Chmod(dot, 0o775))
			return func() { _ = os.Chmod(dot, 0o700) }
		},
		"the item at 0644": func(t *testing.T, e *env) func() {
			item := filepath.Join(e.items(), "conn-a.item")
			require.NoError(t, os.Chmod(item, 0o644))
			return func() { _ = os.Chmod(item, 0o600) }
		},
	} {
		t.Run(name+": denied", func(t *testing.T) {
			e, st, before := seeded(t)
			restore := breakIt(t, e)
			for i, r := range all(e, st) {
				assert.Equal(t, exitTemporary, r.code, "verb %d", i)
				assert.Equal(t, "denied", r.json(t)["store"], "verb %d", i)
				assert.Contains(t, r.stderr, "denied", "the rule is named on stderr")
				assert.NotContains(t, r.stdout+r.stderr, canary)
			}
			assert.Nil(t, e.check().json(t)["present"], "present is null: the store could not say")
			serveNotConnected(t, e)
			restore()
			untouched(t, e, before)
			assert.Equal(t, st, stamp(e.check().json(t)))
		})
	}

	t.Run("the items folder at 000: unreachable", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads everything")
		}
		e, st, before := seeded(t)
		// 000 fails the "exactly 0700" rule before anything is opened
		require.NoError(t, os.Chmod(e.items(), 0o000))
		for i, r := range all(e, st) {
			assert.Equal(t, exitTemporary, r.code, "verb %d", i)
			assert.Contains(t, []any{"denied", "unreachable"}, r.json(t)["store"], "verb %d", i)
		}
		serveNotConnected(t, e)
		require.NoError(t, os.Chmod(e.items(), 0o700))
		untouched(t, e, before)
	})

	t.Run("an item symlink is refused and its target is not read", func(t *testing.T) {
		e, st, _ := seeded(t)
		item := filepath.Join(e.items(), "conn-a.item")
		target := filepath.Join(e.home, "elsewhere.item")
		require.NoError(t, os.Rename(item, target))
		require.NoError(t, os.Symlink(target, item))
		for i, r := range all(e, st) {
			assert.Equal(t, exitTemporary, r.code, "verb %d", i)
		}
		raw, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Contains(t, string(raw), canary, "the target is untouched")
		serveNotConnected(t, e)
	})
}

// readBackFails is a store whose reads fail once a write has gone through.
type readBackFails struct {
	*store.Mem
	wrote bool
}

func (s *readBackFails) Write(ctx context.Context, id string, value []byte) error {
	s.wrote = true
	return s.Mem.Write(ctx, id, value)
}

func (s *readBackFails) Read(ctx context.Context, id string) ([]byte, error) {
	if s.wrote {
		return nil, store.Temporary(store.ReasonUnreachable, errors.New("read-back failed"))
	}
	return s.Mem.Read(ctx, id)
}

// takenThenFails is a store that reports a write it took as unconfirmed.
type takenThenFails struct{ *store.Mem }

func (s *takenThenFails) Write(ctx context.Context, id string, value []byte) error {
	if err := s.Mem.Write(ctx, id, value); err != nil {
		return err
	}
	return &store.UnconfirmedError{Err: errors.New("folder sync failed")}
}

// TestExitSevenMeansNothingWasWritten: once the store has taken a write,
// a failure is "unconfirmed" with exit 1, never "try again".
func TestExitSevenMeansNothingWasWritten(t *testing.T) {
	next := store.Item{V: 1, Rev: 1, Key: canary, Account: "A", Port: 1, Tier: "small"}
	for name, st := range map[string]store.Store{
		"the read-back fails":           &readBackFails{Mem: store.NewMem()},
		"the store could not confirm":   &takenThenFails{Mem: store.NewMem()},
		"the read-back shows something": &writesElse{Mem: store.NewMem()},
	} {
		t.Run(name, func(t *testing.T) {
			var out, errb bytes.Buffer
			e := &keyEnv{st: st, stdout: &out, stderr: &errb}
			code := e.set(context.Background(), keyFlags{connection: "c", expect: "absent"},
				setPayload{Key: next.Key, Account: next.Account, Port: next.Port, Tier: next.Tier})
			assert.Equal(t, exitInternal, code)
			var line map[string]any
			require.NoError(t, json.Unmarshal(out.Bytes(), &line))
			assert.Equal(t, false, line["ok"])
			assert.Equal(t, true, line["unconfirmed"])
			assert.NotContains(t, out.String()+errb.String(), canary)
		})
	}

	t.Run("the item holds the new key", func(t *testing.T) {
		st := &readBackFails{Mem: store.NewMem()}
		var out, errb bytes.Buffer
		e := &keyEnv{st: st, stdout: &out, stderr: &errb}
		require.Equal(t, exitInternal, e.commit(context.Background(), "c", next))
		raw, err := st.Mem.Read(context.Background(), "c")
		require.NoError(t, err)
		got, err := store.DecodeItem(raw)
		require.NoError(t, err)
		assert.Equal(t, next, got)
	})

	t.Run("a write the store refused is exit 7 and nothing changed", func(t *testing.T) {
		st := store.NewMem()
		st.SetFail(store.Temporary(store.ReasonUnreachable, errors.New("disk")))
		var out, errb bytes.Buffer
		e := &keyEnv{st: st, stdout: &out, stderr: &errb}
		assert.Equal(t, exitTemporary, e.commit(context.Background(), "c", next))
		assert.NotContains(t, out.String(), "unconfirmed")
	})
}

// writesElse stores something other than what it was given.
type writesElse struct{ *store.Mem }

func (s *writesElse) Write(ctx context.Context, id string, _ []byte) error {
	other := store.Item{V: 1, Rev: 9, Key: "someone-else", Account: "B", Port: 2, Tier: "full"}
	raw, _ := other.Encode()
	return s.Mem.Write(ctx, id, raw)
}

// TestTwoProcessesOneConnection races real processes on one item: the
// lock and the fp@rev precondition let exactly one of each pair win.
func TestTwoProcessesOneConnection(t *testing.T) {
	race := func(a, b func() result) (result, result) {
		var ra, rb result
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); ra = a() }()
		go func() { defer wg.Done(); rb = b() }()
		wg.Wait()
		return ra, rb
	}
	for round := 0; round < 5; round++ {
		t.Run("set against set from absent", func(t *testing.T) {
			e := newEnv(t)
			ra, rb := race(
				func() result { return e.set("absent", canary+"a", "A", 1, "small") },
				func() result { return e.set("absent", canary+"b", "B", 2, "small") })
			assert.ElementsMatch(t, []int{exitOK, exitPrecondition}, []int{ra.code, rb.code})
			assert.EqualValues(t, 1, e.check().json(t)["rev"])
		})
		t.Run("two retargets from the same rev", func(t *testing.T) {
			e := newEnv(t)
			require.Equal(t, exitOK, e.set("absent", canary, "A", 1, "small").code)
			st := stamp(e.check().json(t))
			ra, rb := race(
				func() result { return e.retarget(st, "A", "1001", "small") },
				func() result { return e.retarget(st, "A", "1002", "small") })
			assert.ElementsMatch(t, []int{exitOK, exitPrecondition}, []int{ra.code, rb.code})
			c := e.check().json(t)
			assert.EqualValues(t, 2, c["rev"])
			want := 1001
			if rb.code == exitOK {
				want = 1002
			}
			assert.EqualValues(t, want, c["port"])
		})
		t.Run("set against delete", func(t *testing.T) {
			e := newEnv(t)
			require.Equal(t, exitOK, e.set("absent", canary, "A", 1, "small").code)
			st := stamp(e.check().json(t))
			ra, rb := race(
				func() result { return e.set(st, canary+"2", "A", 1, "small") },
				func() result { return e.del(st, "A") })
			assert.ElementsMatch(t, []int{exitOK, exitPrecondition}, []int{ra.code, rb.code})
			if ra.code == exitOK {
				assert.EqualValues(t, 2, e.check().json(t)["rev"])
			} else {
				assert.Equal(t, exitNoItem, e.check().code)
			}
		})
	}
}

// heartStub answers like heart's full tier and counts what reaches it.
type heartStub struct {
	srv  *httptest.Server
	hits atomic.Int32
	auth atomic.Value
}

func newHeartStub(t *testing.T) *heartStub {
	h := &heartStub{}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.hits.Add(1)
		h.auth.Store(r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(body, &msg)
		if len(msg.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		result := map[string]any{}
		if msg.Method == "initialize" {
			result["protocolVersion"] = "2025-11-25"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *heartStub) port() int {
	_, p, _ := net.SplitHostPort(h.srv.Listener.Addr().String())
	n, _ := strconv.Atoi(p)
	return n
}

// lockedBuffer collects a child's stderr while the test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// child is a serve process on real pipes.
type child struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	outW   io.Closer
	stderr *lockedBuffer
	done   chan error
}

func (e *env) serve(extra ...string) *child {
	cmd := exec.Command(e.bin, append([]string{"serve", "--connection", "conn-a"}, extra...)...)
	cmd.Env = childEnv(e.home)
	stdin, err := cmd.StdinPipe()
	require.NoError(e.t, err)
	outR, outW, err := os.Pipe()
	require.NoError(e.t, err)
	cmd.Stdout = outW
	c := &child{t: e.t, cmd: cmd, stdin: stdin, stdout: bufio.NewReader(outR), outW: outR, stderr: &lockedBuffer{}, done: make(chan error, 1)}
	cmd.Stderr = c.stderr
	require.NoError(e.t, cmd.Start())
	_ = outW.Close()
	go func() { c.done <- cmd.Wait() }()
	e.t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = outR.Close()
	})
	return c
}

func (c *child) call(line string) map[string]any {
	c.t.Helper()
	_, err := io.WriteString(c.stdin, line+"\n")
	require.NoError(c.t, err)
	got := make(chan string, 1)
	go func() {
		s, _ := c.stdout.ReadString('\n')
		got <- s
	}()
	select {
	case s := <-got:
		var m map[string]any
		require.NoError(c.t, json.Unmarshal([]byte(s), &m), "stdout line %q; stderr: %s", s, c.stderr.String())
		return m
	case <-time.After(10 * time.Second):
		c.t.Fatalf("no reply; stderr: %s", c.stderr.String())
		return nil
	}
}

func (c *child) wait() int {
	c.t.Helper()
	select {
	case err := <-c.done:
		if err == nil {
			return 0
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		c.t.Fatalf("wait: %v", err)
	case <-time.After(15 * time.Second):
		c.t.Fatalf("serve did not exit; stderr: %s", c.stderr.String())
	}
	return -1
}

func errCode(m map[string]any) int {
	e, _ := m["error"].(map[string]any)
	if e == nil {
		return 0
	}
	return int(e["code"].(float64))
}

const initLine = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

func TestServeProcess(t *testing.T) {
	t.Run("forwards over real pipes; stdin EOF exits 0; the key is in no output", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		require.Equal(t, exitOK, e.set("absent", canary, "A", heart.port(), "full").code)
		c := e.serve()
		m := c.call(initLine)
		require.Zero(t, errCode(m), "%v", m)
		assert.Equal(t, "2025-06-18", m["result"].(map[string]any)["protocolVersion"])
		assert.Zero(t, errCode(c.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)))
		assert.Equal(t, "Bearer "+canary, heart.auth.Load())
		require.NoError(t, c.stdin.Close())
		assert.Equal(t, 0, c.wait())
		assert.NotContains(t, c.stderr.String(), canary)
		assert.NotEmpty(t, c.stderr.String())
	})

	t.Run("a serve started with no item never adopts one, whoever connects later", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		c := e.serve()
		assert.Equal(t, -32001, errCode(c.call(initLine)))
		// another account connects under the same connection id
		require.Equal(t, exitOK, e.set("absent", canary, "AccountTwo", heart.port(), "full").code)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, -32001, errCode(c.call(initLine)))
		assert.Equal(t, -32001, errCode(c.call(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"x"}}`)))
		assert.Zero(t, heart.hits.Load(), "this process sent heart nothing")
		require.NoError(t, c.stdin.Close())
		assert.Equal(t, 0, c.wait())

		// a restart is the recovery
		c = e.serve()
		assert.Zero(t, errCode(c.call(initLine)))
		assert.NotZero(t, heart.hits.Load())
	})

	t.Run("a running serve follows a retarget to another port without a restart", func(t *testing.T) {
		e := newEnv(t)
		first, second := newHeartStub(t), newHeartStub(t)
		r := e.set("absent", canary, "A", first.port(), "full")
		require.Equal(t, exitOK, r.code)
		c := e.serve()
		require.Zero(t, errCode(c.call(initLine)))
		first.srv.Close()
		require.Equal(t, exitOK, e.retarget(stamp(r.json(t)), "A", strconv.Itoa(second.port()), "full").code)
		// the first port now refuses; the item is re-read and the request retried
		require.Eventually(t, func() bool {
			return errCode(c.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)) == 0
		}, 10*time.Second, 100*time.Millisecond)
		assert.NotZero(t, second.hits.Load())
	})

	if runtime.GOOS == "windows" {
		return
	}
	t.Run("SIGTERM exits within the deadline", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		require.Equal(t, exitOK, e.set("absent", canary, "A", heart.port(), "full").code)
		c := e.serve()
		require.Zero(t, errCode(c.call(initLine)))
		require.NoError(t, c.cmd.Process.Signal(syscall.SIGTERM))
		assert.Equal(t, 0, c.wait())
	})

	t.Run("a closed stdout with stdin open ends the process: SIGPIPE is ignored", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		require.Equal(t, exitOK, e.set("absent", canary, "A", heart.port(), "full").code)
		c := e.serve()
		require.Zero(t, errCode(c.call(initLine)))
		require.NoError(t, c.outW.Close())
		_, err := io.WriteString(c.stdin, `{"jsonrpc":"2.0","id":2,"method":"ping"}`+"\n")
		require.NoError(t, err)
		assert.Equal(t, 1, c.wait(), "an exit code, not death by signal")
	})
}
