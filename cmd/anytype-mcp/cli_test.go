package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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
)

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// binaries builds the forwarder three ways, once: the test build (file
// store, overrides), the default build and the release-namespace build.
func binaries(t *testing.T) (test, dev, release string) {
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
			{"test", "-tags", "teststore"},
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
	return filepath.Join(buildDir, exe("test")), filepath.Join(buildDir, exe("dev")), filepath.Join(buildDir, exe("release"))
}

func TestMain(m *testing.M) {
	code := m.Run()
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
	os.Exit(code)
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

// env is one scratch connection store.
type env struct {
	t   *testing.T
	bin string
	dir string
}

func newEnv(t *testing.T) *env {
	bin, _, _ := binaries(t)
	return &env{t: t, bin: bin, dir: t.TempDir()}
}

func (e *env) dirs() []string { return []string{"--store-dir", e.dir, "--lock-dir", e.dir} }

func (e *env) run(stdin string, args ...string) result {
	e.t.Helper()
	return runBin(e.t, e.bin, stdin, args...)
}

func runBin(t *testing.T, bin, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	// nothing from the test's environment may steer the binary
	cmd.Env = []string{"HOME=" + t.TempDir()}
	return runCmd(t, cmd, stdin)
}

func runCmd(t *testing.T, cmd *exec.Cmd, stdin string) result {
	t.Helper()
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
	args := append([]string{"key", "set", "--connection", "conn-a", "--expect", expect}, e.dirs()...)
	return e.run(payload(key, account, port, tier), args...)
}

func (e *env) check() result {
	return e.run("", append([]string{"key", "check", "--connection", "conn-a"}, e.dirs()...)...)
}

func stamp(r map[string]any) string {
	return fmt.Sprintf("%s@%d", r["fingerprint"], int64(r["rev"].(float64)))
}

func TestVersionAndNamespaces(t *testing.T) {
	test, dev, release := binaries(t)
	for _, tc := range []struct {
		bin, namespace, version string
	}{{dev, "dev", "dev"}, {release, "release", "1.2.3"}, {test, "dev", "dev"}} {
		r := runBin(t, tc.bin, "", "version", "--json")
		require.Equal(t, 0, r.code)
		m := r.json(t)
		assert.Equal(t, tc.namespace, m["namespace"])
		assert.Equal(t, tc.version, m["version"])
		assert.Equal(t, "go", m["impl"])
		assert.EqualValues(t, Rev, m["rev"])
	}
	assert.Equal(t, true, runBin(t, test, "", "version", "--json").json(t)["testBuild"])
	assert.Nil(t, runBin(t, release, "", "version", "--json").json(t)["testBuild"])
	assert.Contains(t, runBin(t, dev, "", "version").stdout, "dev namespace")

	// the namespaces can never name the same item
	assert.Equal(t, "Anytype MCP", store.ServiceName("release"))
	assert.Equal(t, "Anytype MCP Dev", store.ServiceName("dev"))
	assert.Equal(t, "Anytype MCP Dev", store.ServiceName("anything-else"))

	t.Run("shipped builds have no overrides", func(t *testing.T) {
		for _, bin := range []string{dev, release} {
			for _, flag := range []string{"--port", "--tier", "--store-dir", "--lock-dir"} {
				r := runBin(t, bin, "", "serve", "--connection", "x", flag, "1")
				assert.Equal(t, exitUsage, r.code, flag)
			}
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

		args := append([]string{"key", "retarget", "--connection", "conn-a", "--expect", stamp(first), "--account", "AccountOne", "--port", "40000", "--tier", "full"}, e.dirs()...)
		r = e.run("", args...)
		require.Equal(t, exitOK, r.code, r.stderr)
		second := r.json(t)
		assert.EqualValues(t, 2, second["rev"])
		assert.Equal(t, first["fingerprint"], second["fingerprint"], "a retarget keeps the key")
		assert.EqualValues(t, 40000, e.check().json(t)["port"])

		// the same retarget again: its precondition is stale
		assert.Equal(t, exitPrecondition, e.run("", args...).code)

		// rotation
		r = e.set(stamp(second), canary+"2", "AccountOne", 40000, "full")
		require.Equal(t, exitOK, r.code, r.stderr)
		third := r.json(t)
		assert.EqualValues(t, 3, third["rev"])
		assert.NotEqual(t, second["fingerprint"], third["fingerprint"])

		del := func(expect, account string) result {
			return e.run("", append([]string{"key", "delete", "--connection", "conn-a", "--expect", expect, "--account", account}, e.dirs()...)...)
		}
		assert.Equal(t, exitPrecondition, del(stamp(second), "AccountOne").code, "stale state")
		assert.Equal(t, exitPrecondition, del(stamp(third), "AccountTwo").code, "not the owner")
		assert.Equal(t, exitOK, e.check().code, "a refused delete changes nothing")
		r = del(stamp(third), "AccountOne")
		assert.Equal(t, exitOK, r.code)
		assert.Equal(t, true, r.json(t)["ok"])
		assert.Equal(t, exitNoItem, e.check().code)
		assert.Equal(t, exitNoItem, del(stamp(third), "AccountOne").code, "an absent item is exit 3")
	})

	t.Run("preconditions", func(t *testing.T) {
		e := newEnv(t)
		assert.Equal(t, exitPrecondition, e.set(store.Fingerprint(canary)+"@1", canary, "A", 1, "small").code, "expected an item, found none")
		require.Equal(t, exitOK, e.set("absent", canary, "A", 1, "small").code)
		assert.Equal(t, exitPrecondition, e.set("absent", canary, "A", 1, "small").code, "expected none, found one")
		assert.Equal(t, exitPrecondition, e.set(store.Fingerprint(canary)+"@2", canary, "A", 1, "small").code, "wrong rev")
		assert.Equal(t, exitPrecondition, e.set(store.Fingerprint("other")+"@1", canary, "A", 1, "small").code, "wrong fingerprint")
		retarget := e.run("", append([]string{"key", "retarget", "--connection", "conn-a", "--expect", store.Fingerprint(canary) + "@1", "--account", "B", "--port", "2", "--tier", "small"}, e.dirs()...)...)
		assert.Equal(t, exitPrecondition, retarget.code, "not the owner")
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
		for name, in := range map[string]string{
			"empty":              "",
			"not JSON":           "hello\n",
			"unknown field":      `{"key":"k","account":"A","port":1,"tier":"small","extra":1}` + "\n",
			"no key":             payload("", "A", 1, "small"),
			"bad tier":           payload("k", "A", 1, "medium"),
			"bad port":           payload("k", "A", 0, "small"),
			"no account":         payload("k", "", 1, "small"),
			"key with a newline": `{"key":"a\nb","account":"A","port":1,"tier":"small"}` + "\n",
		} {
			r := e.run(in, append([]string{"key", "set", "--connection", "conn-a", "--expect", "absent"}, e.dirs()...)...)
			assert.Equal(t, exitInvalidInput, r.code, name)
		}
		assert.Equal(t, exitNoItem, e.check().code, "nothing was written")
	})

	t.Run("no store is exit 4; a locked or denying store is exit 7 and never destructive", func(t *testing.T) {
		e := newEnv(t)
		require.Equal(t, exitOK, e.set("absent", canary, "A", 1, "small").code)
		st := stamp(e.check().json(t))
		state := filepath.Join(e.dir, ".state")
		all := func() []result {
			return []result{
				e.check(),
				e.set(st, canary+"x", "A", 1, "small"),
				e.run("", append([]string{"key", "retarget", "--connection", "conn-a", "--expect", st, "--account", "A", "--port", "2", "--tier", "small"}, e.dirs()...)...),
				e.run("", append([]string{"key", "delete", "--connection", "conn-a", "--expect", st, "--account", "A"}, e.dirs()...)...),
			}
		}
		for _, s := range []string{"locked", "denied"} {
			require.NoError(t, os.WriteFile(state, []byte(s), 0o600))
			for i, r := range all() {
				assert.Equal(t, exitTemporary, r.code, "%s verb %d", s, i)
				assert.Equal(t, s, r.json(t)["store"])
			}
		}
		require.NoError(t, os.WriteFile(state, []byte("nostore"), 0o600))
		for i, r := range all() {
			assert.Equal(t, exitNoStore, r.code, "verb %d", i)
			assert.Equal(t, "none", r.json(t)["store"])
		}
		require.NoError(t, os.Remove(state))
		assert.Equal(t, st, stamp(e.check().json(t)), "the item is untouched")
	})

	t.Run("an invalid stored value counts as absent and is replaced by key set", func(t *testing.T) {
		e := newEnv(t)
		require.NoError(t, os.WriteFile(filepath.Join(e.dir, "conn-a.item"), []byte("garbage"), 0o600))
		r := e.check()
		assert.Equal(t, exitNoItem, r.code)
		assert.Equal(t, true, r.json(t)["invalid"])
		assert.Equal(t, exitOK, e.set("absent", canary, "A", 1, "small").code)
	})

	t.Run("the key is in no output", func(t *testing.T) {
		e := newEnv(t)
		outputs := []result{
			e.set("absent", canary, "A", 1, "small"),
			e.check(),
			e.set("absent", canary, "A", 1, "small"),
			e.run(`{"key":"`+canary+`","oops":1}`+"\n", append([]string{"key", "set", "--connection", "conn-a", "--expect", "absent"}, e.dirs()...)...),
		}
		for _, r := range outputs {
			assert.NotContains(t, r.stdout, canary)
			assert.NotContains(t, r.stderr, canary)
		}
	})
}

// TestTwoProcessesOneConnection races real processes on one item: the
// lock and the fp@rev precondition let exactly one of each pair win.
func TestTwoProcessesOneConnection(t *testing.T) {
	race := func(t *testing.T, a, b func() result) (result, result) {
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
			ra, rb := race(t,
				func() result { return e.set("absent", canary+"a", "A", 1, "small") },
				func() result { return e.set("absent", canary+"b", "B", 2, "small") })
			codes := []int{ra.code, rb.code}
			assert.ElementsMatch(t, []int{exitOK, exitPrecondition}, codes)
			assert.EqualValues(t, 1, e.check().json(t)["rev"])
		})
		t.Run("two retargets from the same rev", func(t *testing.T) {
			e := newEnv(t)
			require.Equal(t, exitOK, e.set("absent", canary, "A", 1, "small").code)
			st := stamp(e.check().json(t))
			re := func(port string) func() result {
				return func() result {
					return e.run("", append([]string{"key", "retarget", "--connection", "conn-a", "--expect", st, "--account", "A", "--port", port, "--tier", "small"}, e.dirs()...)...)
				}
			}
			ra, rb := race(t, re("1001"), re("1002"))
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
			ra, rb := race(t,
				func() result { return e.set(st, canary+"2", "A", 1, "small") },
				func() result {
					return e.run("", append([]string{"key", "delete", "--connection", "conn-a", "--expect", st, "--account", "A"}, e.dirs()...)...)
				})
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
	args := append([]string{"serve", "--connection", "conn-a"}, e.dirs()...)
	cmd := exec.Command(e.bin, append(args, extra...)...)
	cmd.Env = []string{"HOME=" + e.t.TempDir()}
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

	t.Run("a locked store at start is final too", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		require.Equal(t, exitOK, e.set("absent", canary, "A", heart.port(), "full").code)
		state := filepath.Join(e.dir, ".state")
		require.NoError(t, os.WriteFile(state, []byte("locked"), 0o600))
		c := e.serve()
		assert.Equal(t, -32001, errCode(c.call(initLine)))
		require.NoError(t, os.Remove(state))
		assert.Equal(t, -32001, errCode(c.call(initLine)))
		assert.Zero(t, heart.hits.Load())
	})

	t.Run("the test build's overrides replace the item's port and tier", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		require.Equal(t, exitOK, e.set("absent", canary, "A", 1, "small").code)
		c := e.serve("--port", strconv.Itoa(heart.port()), "--tier", "full")
		assert.Zero(t, errCode(c.call(initLine)))
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
