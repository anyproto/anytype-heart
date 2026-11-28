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
	"os/user"
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
	assert.Contains(t, store.Open("dev").Path(), "anytype-mcp-home-")
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

// env is one scratch home and the environment a child gets.
type env struct {
	t    *testing.T
	bin  string
	home string
	// key is ANYTYPE_API_KEY; unset with noKey
	key   string
	noKey bool
	extra []string
}

func newEnv(t *testing.T) *env {
	dev, _ := binaries(t)
	home := t.TempDir()
	profileHome(t, home)
	return &env{t: t, bin: dev, home: home, key: canary}
}

// childEnv is the whole environment of a child: built from scratch, never
// from os.Environ. The system's temporary folder is the scratch home too:
// a full-tier serve without --files-dir makes its files folder there.
func childEnv(home string) []string {
	return []string{"HOME=" + home, "USERPROFILE=" + home, "TMPDIR=" + home, "TMP=" + home, "TEMP=" + home}
}

func (e *env) environ() []string {
	vars := append(childEnv(e.home), e.extra...)
	if !e.noKey {
		vars = append(vars, "ANYTYPE_API_KEY="+e.key)
	}
	return vars
}

// endpointPath is the development build's endpoint file.
func (e *env) endpointPath() string { return store.OpenAt(e.home, store.NamespaceDev).Path() }

// writeEndpoint writes the endpoint file as the desktop app does.
func (e *env) writeEndpoint(port int) {
	e.t.Helper()
	require.NoError(e.t, os.MkdirAll(filepath.Dir(e.endpointPath()), 0o700))
	require.NoError(e.t, os.WriteFile(e.endpointPath(), []byte(fmt.Sprintf(`{"v":1,"port":%d}`, port)), 0o600))
}

// holders lists every file under the home whose bytes contain the canary.
func (e *env) holders() []string {
	var holders []string
	require.NoError(e.t, filepath.WalkDir(e.home, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err == nil && bytes.Contains(raw, []byte(canary)) {
			holders = append(holders, path)
		}
		return nil
	}))
	return holders
}

func (e *env) run(stdin string, args ...string) result {
	e.t.Helper()
	return runWith(e.t, e.environ(), e.bin, stdin, args...)
}

func runIn(t *testing.T, home, bin, stdin string, args ...string) result {
	t.Helper()
	return runWith(t, childEnv(home), bin, stdin, args...)
}

func runWith(t *testing.T, vars []string, bin, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = vars
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
	// H1 case 16: the key is in no output
	assert.NotContains(t, out.String(), canary, "the key on stdout")
	assert.NotContains(t, errb.String(), canary, "the key on stderr")
	return result{code: code, stdout: out.String(), stderr: errb.String()}
}

// groupWritable opens a path to others the way each OS spells it: a mode
// bit on Unix, a grant to Users on Windows.
func groupWritable(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		out, err := exec.Command("icacls", path, "/grant", "*S-1-5-32-545:M").CombinedOutput()
		require.NoError(t, err, string(out))
		return
	}
	fi, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(path, fi.Mode().Perm()|0o020))
}

// darwinACL adds a macOS ACL entry and removes all of them at cleanup.
func darwinACL(t *testing.T, entry, path string) {
	t.Helper()
	out, err := exec.Command("/bin/chmod", "+a", entry, path).CombinedOutput()
	require.NoError(t, err, string(out))
	t.Cleanup(func() { _ = exec.Command("/bin/chmod", "-N", path).Run() })
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

	t.Run("dev and release read different endpoint files", func(t *testing.T) {
		e := newEnv(t)
		e.writeEndpoint(4000)
		assert.Equal(t, `{"ok":true,"port":4000}`, strings.TrimSpace(runIn(t, e.home, dev, "", "endpoint").stdout))
		r := runIn(t, e.home, release, "", "endpoint")
		assert.Equal(t, exitInternal, r.code)
		assert.Equal(t, "missing", r.json(t)["reason"])
		assert.Equal(t, filepath.Join(e.home, ".anytype", "ai-connect", "endpoint.json"), r.json(t)["path"])
	})

	t.Run("there are no override flags", func(t *testing.T) {
		for _, bin := range []string{dev, release} {
			for _, flag := range []string{"--port", "--connection", "--store-dir", "--lock-dir", "--key"} {
				assert.Equal(t, exitUsage, runIn(t, home, bin, "", "serve", flag, "1").code, flag)
				assert.Equal(t, exitUsage, runIn(t, home, bin, "", "endpoint", flag, "1").code, flag)
			}
			assert.Equal(t, exitUsage, runIn(t, home, bin, "", "serve", "--tier", "medium").code)
			assert.Equal(t, exitUsage, runIn(t, home, bin, "", "serve", "extra").code)
		}
	})
}

// heartStub answers like heart's full tier and its three /v2 file routes,
// and counts what reaches it.
type heartStub struct {
	srv  *httptest.Server
	hits atomic.Int32
	auth atomic.Value
	// tools makes tools/list answer a list; session makes initialize open
	// a session, as a curated tier does; reject answers 401 to everything
	tools   atomic.Bool
	session atomic.Bool
	reject  atomic.Bool
	// files are the file objects heart holds, by id
	files map[string]string
}

func newHeartStub(t *testing.T) *heartStub {
	h := &heartStub{files: map[string]string{}}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.hits.Add(1)
		h.auth.Store(r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		if h.reject.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"object":"error","status":401,"code":"unauthorized","message":"invalid api key"}`)
			return
		}
		if parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v2/spaces/s/"), "/"); strings.HasPrefix(r.URL.Path, "/v2/") {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "objects":
				_, _ = fmt.Fprintf(w, `{"object":"object","properties":{"name":%q,"file_ext":"txt"}}`, parts[1])
			case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "files":
				w.Header().Set("Content-Type", "text/plain")
				_, _ = io.WriteString(w, h.files[parts[1]])
			case r.Method == http.MethodPost && len(parts) == 1 && parts[0] == "files":
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"object":"file","id":"uploaded"}`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
			return
		}
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
			if h.session.Load() {
				w.Header().Set("Mcp-Session-Id", "stub-session")
			}
		}
		if msg.Method == "tools/list" && h.tools.Load() {
			result["tools"] = []any{map[string]any{"name": "list_spaces"}}
		}
		if msg.Method == "tools/call" {
			result["content"] = []any{map[string]any{"type": "text", "text": "done"}}
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
	out    *lockedBuffer
	stderr *lockedBuffer
	done   chan error
}

func (e *env) serve(args ...string) *child {
	cmd := exec.Command(e.bin, append([]string{"serve"}, args...)...)
	cmd.Env = e.environ()
	stdin, err := cmd.StdinPipe()
	require.NoError(e.t, err)
	outR, outW, err := os.Pipe()
	require.NoError(e.t, err)
	cmd.Stdout = outW
	c := &child{t: e.t, cmd: cmd, stdin: stdin, outW: outR, out: &lockedBuffer{}, stderr: &lockedBuffer{}, done: make(chan error, 1)}
	c.stdout = bufio.NewReader(io.TeeReader(outR, c.out))
	cmd.Stderr = c.stderr
	require.NoError(e.t, cmd.Start())
	_ = outW.Close()
	go func() { c.done <- cmd.Wait() }()
	e.t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = outR.Close()
		// H1 case 16: the key is in no output
		if !e.noKey && e.key != "" {
			assert.NotContains(e.t, c.out.String(), e.key, "the key on stdout")
			assert.NotContains(e.t, c.stderr.String(), e.key, "the key on stderr")
		}
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

// close ends stdin and waits for a clean exit.
func (c *child) close() {
	c.t.Helper()
	require.NoError(c.t, c.stdin.Close())
	assert.Equal(c.t, 0, c.wait())
}

func errCode(m map[string]any) int {
	e, _ := m["error"].(map[string]any)
	if e == nil {
		return 0
	}
	return int(e["code"].(float64))
}

func errMessage(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	if e == nil {
		return ""
	}
	s, _ := e["message"].(string)
	return s
}

func toolText(m map[string]any) string {
	r, _ := m["result"].(map[string]any)
	content, _ := r["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	s, _ := content[0].(map[string]any)["text"].(string)
	return s
}

const initLine = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

const callLine = `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"x","arguments":{}}}`

// H1 case 1: a key that is missing, empty, the unresolved bundle field,
// too long or not visible ASCII: -32001, nothing reaches heart, and so it
// stays after the endpoint appears.
func TestH1_01_BadKeyIsNeverSent(t *testing.T) {
	for name, key := range map[string]*string{
		"missing":                nil,
		"empty":                  ptr(""),
		"${user_config.api_key}": ptr("${user_config.api_key}"),
		"1025 characters":        ptr(strings.Repeat("k", 1025)),
		"a control character":    ptr("CANARY-key\x01-0123456789"),
		"a space":                ptr("CANARY key 0123456789"),
		"not ASCII":              ptr("CANARY-kéy-0123456789"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			e.noKey = key == nil
			if key != nil {
				e.key = *key
			}
			heart := newHeartStub(t)
			c := e.serve("--tier", "full")
			m := c.call(initLine)
			assert.Equal(t, -32001, errCode(m))
			assert.Contains(t, errMessage(m), "Anytype is not connected: the app passed")
			e.writeEndpoint(heart.port())
			time.Sleep(1100 * time.Millisecond)
			assert.Equal(t, -32001, errCode(c.call(initLine)))
			assert.Equal(t, -32001, errCode(c.call(callLine)))
			c.close()
			assert.Zero(t, heart.hits.Load(), "nothing reached heart")
			assert.Contains(t, c.stderr.String(), "not connected until restart: the app passed")
			if key != nil && len(*key) > 8 {
				assert.NotContains(t, c.stderr.String(), *key)
			}
		})
	}

	t.Run("1024 visible characters are a key", func(t *testing.T) {
		e := newEnv(t)
		e.key = strings.Repeat("k", 1024)
		heart := newHeartStub(t)
		e.writeEndpoint(heart.port())
		c := e.serve()
		assert.Zero(t, errCode(c.call(initLine)))
		assert.Equal(t, "Bearer "+e.key, heart.auth.Load())
		c.close()
	})
}

func ptr(s string) *string { return &s }

// H1 case 2: an encrypted value is named as such, once, and never shown.
func TestH1_02_EncryptedValue(t *testing.T) {
	const value = "__encrypted__:AAAAQUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo="
	e := newEnv(t)
	e.key = value
	heart := newHeartStub(t)
	e.writeEndpoint(heart.port())
	c := e.serve()
	m := c.call(initLine)
	assert.Equal(t, -32001, errCode(m))
	assert.Contains(t, errMessage(m), "the app passed an encrypted value instead of the key")
	assert.Equal(t, -32001, errCode(c.call(callLine)))
	c.close()
	stderr := c.stderr.String()
	assert.Equal(t, 1, strings.Count(stderr, "the app passed an encrypted value instead of the key"), stderr)
	for _, part := range []string{value, strings.TrimPrefix(value, "__encrypted__:"), "QUJDREVG"} {
		assert.NotContains(t, stderr, part)
		assert.NotContains(t, c.out.String(), part)
	}
	assert.Zero(t, heart.hits.Load(), "in no request")
}

// H1 case 3: no endpoint at start, then one: the first request gets
// -32001, a later one succeeds without a restart, and the full tier lists
// both file tools.
func TestH1_03_EndpointAppearsWithoutRestart(t *testing.T) {
	e := newEnv(t)
	heart := newHeartStub(t)
	heart.tools.Store(true)
	c := e.serve("--tier", "full", "--files-dir", filepath.Join(e.home, "files"))
	m := c.call(initLine)
	assert.Equal(t, -32001, errCode(m))
	assert.Equal(t, "Anytype can't be reached: open Anytype", errMessage(m))
	assert.Zero(t, heart.hits.Load())

	e.writeEndpoint(heart.port())
	require.Eventually(t, func() bool { return errCode(c.call(initLine)) == 0 }, 5*time.Second, 200*time.Millisecond)
	list := c.call(toolsLine)
	require.Zero(t, errCode(list), "%v", list)
	raw, _ := json.Marshal(list)
	assert.Contains(t, string(raw), `"download_file"`)
	assert.Contains(t, string(raw), `"upload_local_file"`)
	c.close()
	assert.Contains(t, c.stderr.String(), "missing: "+filepath.Join(e.home, ".anytype"))
	assert.Contains(t, c.stderr.String(), "endpoint: port "+strconv.Itoa(heart.port()))
}

// verdict is what serve makes of a home, and what the endpoint verb says.
type verdict struct {
	serveOK bool
	stderr  string
	verb    map[string]any
	code    int
}

func (e *env) verdict(t *testing.T) verdict {
	t.Helper()
	c := e.serve()
	m := c.call(initLine)
	c.close()
	r := e.run("", "endpoint")
	return verdict{serveOK: errCode(m) == 0, stderr: c.stderr.String(), verb: r.json(t), code: r.code}
}

// agree asserts that serve and the verb reach the same verdict: both
// accept, or both refuse and serve's stderr names the verb's path and rule.
func (v verdict) agree(t *testing.T, ok bool, reason string) {
	t.Helper()
	assert.Equal(t, ok, v.serveOK, "serve: %s", v.stderr)
	assert.Equal(t, ok, v.verb["ok"], "%v", v.verb)
	if ok {
		assert.Equal(t, 0, v.code)
		assert.Len(t, v.verb, 2, "ok and port")
		return
	}
	assert.Equal(t, 1, v.code)
	assert.Equal(t, reason, v.verb["reason"], "%v", v.verb)
	path, _ := v.verb["path"].(string)
	assert.NotEmpty(t, path)
	assert.Contains(t, v.stderr, path)
	if rule, _ := v.verb["rule"].(string); rule != "" {
		assert.Contains(t, v.stderr, rule)
	}
}

// H1 case 5: an unsafe .anytype or ai-connect that already exists is
// refused with its rule and path on stderr; a stock macOS home (everyone
// deny delete) and a normal Windows profile are accepted.
func TestH1_05_UnsafeFoldersAndStockHomes(t *testing.T) {
	for _, name := range []string{".anytype", "ai-connect"} {
		t.Run("an unsafe "+name, func(t *testing.T) {
			e := newEnv(t)
			heart := newHeartStub(t)
			e.writeEndpoint(heart.port())
			dir := filepath.Join(e.home, ".anytype")
			if name == "ai-connect" {
				dir = filepath.Dir(e.endpointPath())
			}
			groupWritable(t, dir)
			v := e.verdict(t)
			v.agree(t, false, "denied")
			assert.Equal(t, dir, v.verb["path"])
			assert.Contains(t, v.verb["rule"], "folder on the endpoint path")
			assert.Zero(t, heart.hits.Load())
		})
	}

	t.Run("a stock home of this OS", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		if runtime.GOOS == "darwin" {
			darwinACL(t, "everyone deny delete", e.home)
		}
		// on Windows the scratch home is a normal profile folder: every
		// folder and the file inherit the user's, SYSTEM's and
		// Administrators' entries
		e.writeEndpoint(heart.port())
		e.verdict(t).agree(t, true, "")
		assert.NotZero(t, heart.hits.Load())
	})
}

// H1 case 6: the endpoint verb reaches serve's verdict in the states of
// cases 3 to 5, as one JSON line.
func TestH1_06_EndpointVerb(t *testing.T) {
	heart := newHeartStub(t)
	type state struct {
		set    func(t *testing.T, e *env)
		ok     bool
		reason string
	}
	states := map[string]state{
		"no endpoint yet": {func(*testing.T, *env) {}, false, "missing"},
		"written":         {func(t *testing.T, e *env) { e.writeEndpoint(heart.port()) }, true, ""},
		"deleted": {func(t *testing.T, e *env) {
			e.writeEndpoint(heart.port())
			require.NoError(t, os.Remove(e.endpointPath()))
		}, false, "missing"},
		"not an endpoint": {func(t *testing.T, e *env) {
			e.writeEndpoint(heart.port())
			require.NoError(t, os.WriteFile(e.endpointPath(), []byte(`{"v":2,"port":1}`), 0o600))
		}, false, "invalid"},
		"group-writable": {func(t *testing.T, e *env) {
			e.writeEndpoint(heart.port())
			groupWritable(t, e.endpointPath())
		}, false, "denied"},
		"an unsafe .anytype": {func(t *testing.T, e *env) {
			e.writeEndpoint(heart.port())
			groupWritable(t, filepath.Join(e.home, ".anytype"))
		}, false, "denied"},
	}
	if runtime.GOOS != "windows" {
		states["made a symlink"] = state{func(t *testing.T, e *env) {
			e.writeEndpoint(heart.port())
			good := filepath.Join(e.home, "good.json")
			require.NoError(t, os.Rename(e.endpointPath(), good))
			require.NoError(t, os.Symlink(good, e.endpointPath()))
		}, false, "denied"}
	}
	if runtime.GOOS == "darwin" {
		states["given a macOS ACL"] = state{func(t *testing.T, e *env) {
			e.writeEndpoint(heart.port())
			darwinACL(t, "everyone allow read", e.endpointPath())
		}, false, "denied"}
		states["a stock macOS home"] = state{func(t *testing.T, e *env) {
			darwinACL(t, "everyone deny delete", e.home)
			e.writeEndpoint(heart.port())
		}, true, ""}
	}
	for name, s := range states {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			s.set(t, e)
			v := e.verdict(t)
			v.agree(t, s.ok, s.reason)
			r := e.run("", "endpoint")
			assert.Equal(t, 1, strings.Count(r.stdout, "\n"), "one line")
			assert.False(t, strings.HasPrefix(r.stdout, " "))
			if s.ok {
				assert.Equal(t, fmt.Sprintf(`{"ok":true,"port":%d}`, heart.port()), strings.TrimSpace(r.stdout))
			}
		})
	}
	t.Run("it takes no arguments", func(t *testing.T) {
		assert.Equal(t, exitUsage, newEnv(t).run("", "endpoint", "x").code)
	})
}

// H1 case 7: run from under <home>/.anytype, the verb also judges its own
// executable and the folders down to it; run from elsewhere it does not.
func TestH1_07_EndpointVerbJudgesItsExecutable(t *testing.T) {
	dev, _ := binaries(t)
	install := func(t *testing.T, dir string) string {
		t.Helper()
		require.NoError(t, os.MkdirAll(dir, 0o700))
		raw, err := os.ReadFile(dev)
		require.NoError(t, err)
		path := filepath.Join(dir, exe("anytype-mcp"))
		require.NoError(t, os.WriteFile(path, raw, 0o700))
		return path
	}
	verb := func(t *testing.T, e *env, bin string) result {
		t.Helper()
		return runWith(t, e.environ(), bin, "", "endpoint")
	}
	hurts := map[string]func(t *testing.T, e *env, bin string) string{
		"bin open to others": func(t *testing.T, e *env, bin string) string {
			groupWritable(t, filepath.Dir(bin))
			return filepath.Dir(bin)
		},
		"an executable with two links": func(t *testing.T, e *env, bin string) string {
			require.NoError(t, os.Link(bin, filepath.Join(e.home, exe("copy"))))
			return bin
		},
	}
	if runtime.GOOS == "darwin" {
		hurts["bin with an ACL that lets another user write"] = func(t *testing.T, e *env, bin string) string {
			darwinACL(t, "everyone allow add_file,delete_child", filepath.Dir(bin))
			return filepath.Dir(bin)
		}
	}
	for name, hurt := range hurts {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.writeEndpoint(4000)
			bin := install(t, filepath.Join(e.home, ".anytype", "bin"))
			assert.Equal(t, `{"ok":true,"port":4000}`, strings.TrimSpace(verb(t, e, bin).stdout), "a sound copy")
			path := hurt(t, e, bin)
			r := verb(t, e, bin)
			assert.Equal(t, exitInternal, r.code)
			m := r.json(t)
			assert.Equal(t, "denied", m["reason"])
			// the verb may name it resolved (Windows 8.3 temp folders)
			assert.Equal(t, resolved(t, path), resolved(t, m["path"].(string)), "names bin or the executable")
			assert.NotEmpty(t, m["rule"])
			// the endpoint file itself is fine
			assert.Equal(t, 0, e.run("", "endpoint").code)
		})
	}

	t.Run("outside .anytype the executable is not judged", func(t *testing.T) {
		e := newEnv(t)
		e.writeEndpoint(4000)
		bin := install(t, filepath.Join(t.TempDir(), "elsewhere"))
		require.NoError(t, os.Link(bin, filepath.Join(filepath.Dir(bin), exe("copy"))))
		groupWritable(t, filepath.Dir(bin))
		assert.Equal(t, `{"ok":true,"port":4000}`, strings.TrimSpace(verb(t, e, bin).stdout))
	})
}

func resolved(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return real
}

// Theme C: the verb judges the path it was started by, as it is spelled:
// a link on it or another spelling of .anytype is refused, and the path
// must name the file that runs.
func TestH1R1_C_EndpointJudgesTheLaunchPath(t *testing.T) {
	dev, _ := binaries(t)
	raw, err := os.ReadFile(dev)
	require.NoError(t, err)
	put := func(t *testing.T, dir string) string {
		t.Helper()
		require.NoError(t, os.MkdirAll(dir, 0o700))
		path := filepath.Join(dir, exe("anytype-mcp"))
		require.NoError(t, os.WriteFile(path, raw, 0o700))
		return path
	}
	link := func(t *testing.T, target, name string) {
		t.Helper()
		if err := os.Symlink(target, name); err != nil {
			t.Skipf("no symlinks here: %v", err)
		}
	}
	verdict := func(t *testing.T, e *env, argv0, path string) map[string]any {
		t.Helper()
		cmd := &exec.Cmd{Path: path, Args: []string{argv0, "endpoint"}, Env: e.environ()}
		out, _ := cmd.Output()
		var m map[string]any
		require.NoError(t, json.Unmarshal(out, &m), "%q", out)
		return m
	}
	setup := func(t *testing.T) (*env, string) {
		e := newEnv(t)
		e.writeEndpoint(4000)
		return e, filepath.Join(e.home, ".anytype", "bin")
	}
	denied := func(t *testing.T, m map[string]any, path, rule string) {
		t.Helper()
		assert.Equal(t, "denied", m["reason"], "%v", m)
		assert.Equal(t, path, m["path"])
		assert.Contains(t, m["rule"], rule)
	}

	t.Run("a sound copy, started by its path", func(t *testing.T) {
		e, bin := setup(t)
		path := put(t, bin)
		assert.Equal(t, true, verdict(t, e, path, path)["ok"])
	})
	t.Run("the installed name is a link to a sound copy outside", func(t *testing.T) {
		e, bin := setup(t)
		outside := put(t, filepath.Join(t.TempDir(), "real"))
		require.NoError(t, os.MkdirAll(bin, 0o700))
		path := filepath.Join(bin, exe("anytype-mcp"))
		link(t, outside, path)
		denied(t, verdict(t, e, path, path), path, "not a regular file")
	})
	t.Run("bin is a link to a folder outside", func(t *testing.T) {
		e, bin := setup(t)
		outside := filepath.Dir(put(t, filepath.Join(t.TempDir(), "real")))
		require.NoError(t, os.MkdirAll(filepath.Dir(bin), 0o700))
		link(t, outside, bin)
		path := filepath.Join(bin, exe("anytype-mcp"))
		denied(t, verdict(t, e, path, path), bin, "not a real directory")
	})
	t.Run("a path outside that leads into .anytype through a link", func(t *testing.T) {
		e, bin := setup(t)
		put(t, bin)
		alias := filepath.Join(t.TempDir(), "alias")
		link(t, bin, alias)
		path := filepath.Join(alias, exe("anytype-mcp"))
		denied(t, verdict(t, e, path, path), path, "another spelling or through a link")
	})
	t.Run("another case of .anytype on a volume that ignores case", func(t *testing.T) {
		e, bin := setup(t)
		put(t, bin)
		path := filepath.Join(e.home, ".Anytype", "bin", exe("anytype-mcp"))
		if _, err := os.Stat(path); err != nil {
			t.Skip("this volume tells cases apart")
		}
		denied(t, verdict(t, e, path, path), path, "")
	})
	t.Run("a start path that names another file than the one running", func(t *testing.T) {
		e, bin := setup(t)
		running := put(t, bin)
		other := put(t, filepath.Join(e.home, ".anytype", "bin2"))
		denied(t, verdict(t, e, other, running), other, "not the file this process runs")
	})
}

// Theme G: the start path is judged as it is spelled: ".." in it is
// refused, and so is another case of .anytype that a later link leads out
// of again.
func TestH1R2_G_LaunchPathAsSpelled(t *testing.T) {
	dev, _ := binaries(t)
	raw, err := os.ReadFile(dev)
	require.NoError(t, err)
	run := func(t *testing.T, e *env, argv0, path string) map[string]any {
		t.Helper()
		out, _ := (&exec.Cmd{Path: path, Args: []string{argv0, "endpoint"}, Env: e.environ()}).Output()
		var m map[string]any
		require.NoError(t, json.Unmarshal(out, &m), "%q", out)
		return m
	}
	t.Run("a start path with ..", func(t *testing.T) {
		e := newEnv(t)
		e.writeEndpoint(4000)
		bin := filepath.Join(e.home, ".anytype", "bin")
		require.NoError(t, os.MkdirAll(filepath.Join(e.home, ".anytype", "shared", "child"), 0o700))
		require.NoError(t, os.MkdirAll(bin, 0o700))
		path := filepath.Join(bin, exe("anytype-mcp"))
		require.NoError(t, os.WriteFile(path, raw, 0o700))
		sep := string(filepath.Separator)
		argv0 := filepath.Join(e.home, ".anytype", "shared", "child") + sep + ".." + sep + ".." + sep + "bin" + sep + exe("anytype-mcp")
		m := run(t, e, argv0, path)
		assert.Equal(t, "denied", m["reason"], "%v", m)
		assert.Contains(t, m["rule"], "not clean and absolute")
	})
	t.Run("another case of .anytype, then a link out to a shared folder", func(t *testing.T) {
		e := newEnv(t)
		e.writeEndpoint(4000)
		shared := filepath.Join(t.TempDir(), "shared")
		require.NoError(t, os.MkdirAll(shared, 0o777))
		path := filepath.Join(shared, exe("anytype-mcp"))
		require.NoError(t, os.WriteFile(path, raw, 0o700))
		require.NoError(t, os.MkdirAll(filepath.Join(e.home, ".anytype"), 0o700))
		if err := os.Symlink(shared, filepath.Join(e.home, ".anytype", "bin")); err != nil {
			t.Skipf("no symlinks here: %v", err)
		}
		alias := filepath.Join(e.home, ".Anytype", "bin", exe("anytype-mcp"))
		if _, err := os.Stat(alias); err != nil {
			t.Skip("this volume tells cases apart")
		}
		m := run(t, e, alias, alias)
		assert.Equal(t, "denied", m["reason"], "%v", m)
		assert.Equal(t, alias, m["path"])
		assert.Contains(t, m["rule"], "another spelling or through a link")
	})
}

// Theme J: an install outside .anytype reached through a junction is not
// refused (Windows only: elsewhere nothing makes a junction).
func TestH1R2_J_InstallBeneathAJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows thing")
	}
	dev, _ := binaries(t)
	raw, err := os.ReadFile(dev)
	require.NoError(t, err)
	e := newEnv(t)
	e.writeEndpoint(4000)
	real := filepath.Join(t.TempDir(), "real")
	require.NoError(t, os.MkdirAll(filepath.Join(real, "app"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(real, "app", exe("anytype-mcp")), raw, 0o700))
	link := filepath.Join(t.TempDir(), "link")
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, real).CombinedOutput()
	if err != nil {
		t.Skipf("no junctions here: %v: %s", err, out)
	}
	path := filepath.Join(link, "app", exe("anytype-mcp"))
	r := runWith(t, e.environ(), path, "", "endpoint")
	assert.Equal(t, `{"ok":true,"port":4000}`, strings.TrimSpace(r.stdout), r.stderr)
}

// H1 case 16: the key is in no output and no file, through a whole
// session: MCP, a download, a refused download that carried the key, an
// upload and a rejected key.
func TestH1_16_KeyNeverInOutputOrFiles(t *testing.T) {
	e := newEnv(t)
	heart := newHeartStub(t)
	heart.tools.Store(true)
	heart.files["note"] = "plain bytes"
	heart.files["leak"] = "config: " + canary
	e.writeEndpoint(heart.port())
	folder := filepath.Join(e.home, "files")
	c := e.serve("--tier", "full", "--files-dir", folder)
	require.Zero(t, errCode(c.call(initLine)))
	require.Zero(t, errCode(c.call(toolsLine)))
	assert.Equal(t, "done", toolText(c.call(callLine)))
	tool := func(name string, args string) map[string]any {
		return c.call(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`)
	}
	assert.Contains(t, toolText(tool("download_file", `{"space_id":"s","file_id":"note"}`)), "note.txt")
	assert.Contains(t, toolText(tool("download_file", `{"space_id":"s","file_id":"leak"}`)), "was not saved")
	require.NoError(t, os.WriteFile(filepath.Join(folder, "up.txt"), []byte("hello"), 0o600))
	assert.Contains(t, toolText(tool("upload_local_file", `{"space_id":"s","path":"up.txt"}`)), "uploaded")
	heart.reject.Store(true)
	assert.Contains(t, errMessage(c.call(callLine)), "no longer accepts this key")
	c.close()
	assert.Equal(t, "Bearer "+canary, heart.auth.Load())
	assert.Empty(t, e.holders(), "no file under the home holds the key")
}

func TestServeProcess(t *testing.T) {
	t.Run("forwards over real pipes; stdin EOF exits 0; the key is in no output", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		e.writeEndpoint(heart.port())
		c := e.serve()
		m := c.call(initLine)
		require.Zero(t, errCode(m), "%v", m)
		assert.Equal(t, "2025-06-18", m["result"].(map[string]any)["protocolVersion"])
		assert.Zero(t, errCode(c.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)))
		assert.Equal(t, "Bearer "+canary, heart.auth.Load())
		c.close()
		assert.NotEmpty(t, c.stderr.String())
	})

	t.Run("a running serve follows the endpoint to another port without a restart", func(t *testing.T) {
		e := newEnv(t)
		first, second := newHeartStub(t), newHeartStub(t)
		e.writeEndpoint(first.port())
		c := e.serve()
		require.Zero(t, errCode(c.call(initLine)))
		first.srv.Close()
		e.writeEndpoint(second.port())
		// the first port now refuses; the endpoint is read again and the request retried
		require.Eventually(t, func() bool {
			return errCode(c.call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)) == 0
		}, 10*time.Second, 100*time.Millisecond)
		assert.NotZero(t, second.hits.Load())
	})

	t.Run("bytes heart never asked to send don't reach stderr through Go's own logger", func(t *testing.T) {
		// a complete reply, then unsolicited bytes with the canary on the
		// kept-alive connection: net/http reports those through the
		// standard logger
		extra := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var msg struct {
				ID json.RawMessage `json:"id"`
			}
			_ = json.Unmarshal(body, &msg)
			reply := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-11-25"}}`, msg.ID)
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			fmt.Fprintf(rw, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(reply), reply)
			_ = rw.Flush()
			go func() {
				time.Sleep(100 * time.Millisecond)
				_, _ = io.WriteString(conn, "HTTP/1.1 200 "+canary+"\r\n\r\n")
				time.Sleep(200 * time.Millisecond)
				_ = conn.Close()
				close(extra)
			}()
		}))
		t.Cleanup(srv.Close)
		_, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
		port, _ := strconv.Atoi(p)
		e := newEnv(t)
		e.writeEndpoint(port)
		c := e.serve()
		require.Zero(t, errCode(c.call(initLine)))
		<-extra
		c.close()
		assert.NotContains(t, c.stderr.String(), "Unsolicited")
	})

	if runtime.GOOS == "windows" {
		return
	}
	t.Run("SIGTERM exits within the deadline", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		e.writeEndpoint(heart.port())
		c := e.serve()
		require.Zero(t, errCode(c.call(initLine)))
		require.NoError(t, c.cmd.Process.Signal(syscall.SIGTERM))
		assert.Equal(t, 0, c.wait())
	})

	t.Run("a closed stdout with stdin open ends the process: SIGPIPE is ignored", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		e.writeEndpoint(heart.port())
		c := e.serve()
		require.Zero(t, errCode(c.call(initLine)))
		require.NoError(t, c.outW.Close())
		_, err := io.WriteString(c.stdin, `{"jsonrpc":"2.0","id":2,"method":"ping"}`+"\n")
		require.NoError(t, err)
		assert.Equal(t, 1, c.wait(), "an exit code, not death by signal")
	})
}

// profileHome gives a scratch home a normal Windows profile's access list
// (the user, SYSTEM and Administrators, inherited by what is made in it),
// so no test depends on what the runner's temporary folder hands down.
// Elsewhere a scratch home is already a normal one.
func profileHome(t *testing.T, home string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return
	}
	me, err := user.Current()
	require.NoError(t, err)
	for _, args := range [][]string{
		{home, "/grant:r", "*" + me.Uid + ":(OI)(CI)F", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F"},
		{home, "/inheritance:r"},
	} {
		out, err := exec.Command("icacls", args...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
}
