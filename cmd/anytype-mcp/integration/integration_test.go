//go:build integration && !windows

package integration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/apiv2eval/heartboot"
	"github.com/anyproto/anytype-heart/pb"
)

const appName = "anytype-mcp-integration"

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

// client is an MCP client on the forwarder's stdio.
type client struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	stdout *lockedBuffer
	stderr *lockedBuffer
	done   chan error
	nextID int
}

// childEnv is a child's whole environment: a scratch home and nothing
// from this process.
func childEnv(home string) []string {
	return []string{"HOME=" + home, "USERPROFILE=" + home, "TMPDIR=" + home, "TMP=" + home, "TEMP=" + home}
}

// TestMain is the scratch guard: HOME points at a fresh temp folder before
// any test runs, so nothing here can reach the developer's item store.
// The go tool's own folders are pinned first; they default to the home.
func TestMain(m *testing.M) {
	for _, name := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOFLAGS", "GOPROXY", "GOPRIVATE", "GOTOOLCHAIN"} {
		if out, err := exec.Command("go", "env", name).Output(); err == nil {
			_ = os.Setenv(name, strings.TrimSpace(string(out)))
		}
	}
	scratch, err := os.MkdirTemp("", "anytype-mcp-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "scratch home:", err)
		os.Exit(1)
	}
	_ = os.Setenv("HOME", scratch)
	_ = os.Setenv("USERPROFILE", scratch)
	code := m.Run()
	_ = os.RemoveAll(scratch)
	os.Exit(code)
}

func startForwarder(t *testing.T, bin, home, connection string, extra ...string) *client {
	cmd := exec.Command(bin, append([]string{"serve", "--connection", connection}, extra...)...)
	cmd.Env = childEnv(home)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	c := &client{t: t, cmd: cmd, stdin: stdin, lines: make(chan string, 64), stdout: &lockedBuffer{}, stderr: &lockedBuffer{}, done: make(chan error, 1)}
	cmd.Stderr = c.stderr
	require.NoError(t, cmd.Start())
	go func() {
		sc := bufio.NewScanner(io.TeeReader(out, c.stdout))
		sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
		for sc.Scan() {
			c.lines <- sc.Text()
		}
		close(c.lines)
		c.done <- cmd.Wait()
	}()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return c
}

type rpcReply struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *client) notify(method string) {
	c.t.Helper()
	_, err := fmt.Fprintf(c.stdin, `{"jsonrpc":"2.0","method":%q}`+"\n", method)
	require.NoError(c.t, err)
}

func (c *client) request(method string, params any) rpcReply {
	c.t.Helper()
	c.nextID++
	return c.requestAs(strconv.Itoa(c.nextID), method, params)
}

// requestAs sends a request under a raw JSON id and requires the reply to
// carry exactly those bytes.
func (c *client) requestAs(id, method string, params any) rpcReply {
	c.t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "method": method}
	if params != nil {
		msg["params"] = params
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	require.NoError(c.t, enc.Encode(msg))
	_, err := c.stdin.Write(buf.Bytes())
	require.NoError(c.t, err)
	select {
	case got, ok := <-c.lines:
		require.True(c.t, ok, "the forwarder closed stdout; stderr:\n%s", c.stderr.String())
		var r rpcReply
		require.NoError(c.t, json.Unmarshal([]byte(got), &r), "stdout line: %s", got)
		require.Equal(c.t, id, string(r.ID), "one reply, with this request's id byte for byte")
		return r
	case <-time.After(90 * time.Second):
		c.t.Fatalf("no reply to %s; stderr:\n%s", method, c.stderr.String())
		return rpcReply{}
	}
}

type toolResult struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func (r toolResult) text() string {
	var b strings.Builder
	for _, c := range r.Content {
		b.WriteString(c.Text)
	}
	return b.String()
}

func (c *client) tool(name string, args map[string]any) toolResult {
	c.t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	r := c.request("tools/call", map[string]any{"name": name, "arguments": args})
	require.Nil(c.t, r.Error, "tools/call %s: %+v", name, r.Error)
	var res toolResult
	require.NoError(c.t, json.Unmarshal(r.Result, &res))
	return res
}

func (c *client) close() int {
	c.t.Helper()
	require.NoError(c.t, c.stdin.Close())
	select {
	case err := <-c.done:
		if err == nil {
			return 0
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		c.t.Fatalf("wait: %v", err)
	case <-time.After(20 * time.Second):
		c.t.Fatalf("the forwarder did not exit; stderr:\n%s", c.stderr.String())
	}
	return -1
}

// direct speaks to heart's /mcp/<tier> without the forwarder, for parity.
type direct struct {
	t       *testing.T
	url     string
	key     string
	session string
	version string
}

func (d *direct) post(method string, params any, id any) (json.RawMessage, int) {
	d.t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != nil {
		msg["id"] = id
	}
	if params != nil {
		msg["params"] = params
	}
	body, _ := json.Marshal(msg)
	req, err := http.NewRequest(http.MethodPost, d.url, bytes.NewReader(body))
	require.NoError(d.t, err)
	req.Header.Set("Authorization", "Bearer "+d.key)
	req.Header.Set("Content-Type", "application/json")
	if d.session != "" {
		req.Header.Set("Mcp-Session-Id", d.session)
	}
	if d.version != "" {
		req.Header.Set("MCP-Protocol-Version", d.version)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(d.t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if s := resp.Header.Get("Mcp-Session-Id"); s != "" {
		d.session = s
	}
	if resp.StatusCode == http.StatusOK {
		assert.True(d.t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json"), "heart answers JSON only")
	}
	var r rpcReply
	_ = json.Unmarshal(raw, &r)
	return r.Result, resp.StatusCode
}

var initParams = map[string]any{
	"protocolVersion": "2025-06-18",
	"capabilities":    map[string]any{},
	"clientInfo":      map[string]any{"name": "anytype-mcp-integration", "version": "1"},
}

func buildForwarder(t *testing.T) string {
	bin := filepath.Join(t.TempDir(), "anytype-mcp-test")
	out, err := exec.Command("go", "build", "-o", bin, "..").CombinedOutput()
	require.NoError(t, err, string(out))
	return bin
}

func runVerb(t *testing.T, bin, home, stdin string, args ...string) (map[string]any, int) {
	cmd := exec.Command(bin, args...)
	cmd.Env = childEnv(home)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		require.True(t, ok, "%v", err)
		code = ee.ExitCode()
	}
	var m map[string]any
	_ = json.Unmarshal(bytes.TrimSpace(out), &m)
	return m, code
}

// TestForwarderAgainstHeart: the built child over real pipes, an MCP
// client on one side and a throwaway heart on the other. Per tier:
// initialize, notifications/initialized, tools/list (the same list heart
// serves directly), a read and a write. Then the key is revoked and the
// next call gets the 401 reply.
func TestForwarderAgainstHeart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	// a scratch data directory and a port the kernel picks: never the
	// desktop app's 31009
	heart, err := heartboot.Start(ctx, heartboot.Options{
		BinaryPath: os.Getenv("ANYTYPE_MCP_HEART_BINARY"), AccountName: appName, AppName: appName,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = heart.Stop() })
	u, err := url.Parse(heart.APIURL)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", u.Hostname())
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	require.NotEqual(t, 31009, port)

	bin := buildForwarder(t)
	// the item store lives in a scratch home; the children get no other
	// environment and no flags
	store := t.TempDir()
	set := func(connection, tier string, port int) string {
		payload, _ := json.Marshal(map[string]any{"key": heart.APIKey, "account": heart.AccountId, "port": port, "tier": tier})
		m, code := runVerb(t, bin, store, string(payload)+"\n", "key", "set", "--connection", connection, "--expect", "absent")
		require.Equal(t, 0, code, "key set: %v", m)
		return fmt.Sprintf("%s@%d", m["fingerprint"], int64(m["rev"].(float64)))
	}
	for _, tier := range []string{"small", "large", "full"} {
		set("it-"+tier, tier, port)
	}

	var outputs []*client
	for _, tier := range []string{"small", "large", "full"} {
		t.Run(tier, func(t *testing.T) {
			// the full tier's file tools get a folder of the test's own
			var extra []string
			filesDir := filepath.Join(t.TempDir(), "files")
			if tier == "full" {
				extra = []string{"--files-dir", filesDir}
			}
			c := startForwarder(t, bin, store, "it-"+tier, extra...)
			outputs = append(outputs, c)

			init := c.request("initialize", initParams)
			require.Nil(t, init.Error, "%+v", init.Error)
			var info struct {
				ProtocolVersion string `json:"protocolVersion"`
				ServerInfo      struct {
					Name string `json:"name"`
				} `json:"serverInfo"`
			}
			require.NoError(t, json.Unmarshal(init.Result, &info))
			assert.Equal(t, "2025-06-18", info.ProtocolVersion)
			assert.Equal(t, "anytype", info.ServerInfo.Name)
			c.notify("notifications/initialized")

			list := c.request("tools/list", nil)
			require.Nil(t, list.Error, "%+v", list.Error)

			// parity: the forwarder hides nothing, and adds only the full
			// tier's two file tools, after heart's own
			d := &direct{t: t, url: heart.APIURL + "/mcp/" + tier, key: heart.APIKey}
			_, status := d.post("initialize", initParams, 1)
			require.Equal(t, http.StatusOK, status)
			d.version = "2025-06-18"
			_, status = d.post("notifications/initialized", nil, nil)
			require.Equal(t, http.StatusAccepted, status)
			want, status := d.post("tools/list", nil, 2)
			require.Equal(t, http.StatusOK, status)
			var tools struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			}
			require.NoError(t, json.Unmarshal(list.Result, &tools))
			names := map[string]bool{}
			for _, tl := range tools.Tools {
				names[tl.Name] = true
			}
			if tier == "full" {
				var heartList, ours struct {
					Tools []json.RawMessage `json:"tools"`
				}
				require.NoError(t, json.Unmarshal(want, &heartList))
				require.NoError(t, json.Unmarshal(list.Result, &ours))
				n := len(heartList.Tools)
				require.Len(t, tools.Tools, n+2)
				heartOwn, _ := json.Marshal(heartList.Tools)
				relayed, _ := json.Marshal(ours.Tools[:n])
				assert.JSONEq(t, string(heartOwn), string(relayed), "heart's tools, unchanged and first")
				assert.Equal(t, "download_file", tools.Tools[n].Name)
				assert.Equal(t, "upload_local_file", tools.Tools[n+1].Name)
				assert.Contains(t, string(ours.Tools[n+1]), filesDir, "the description names the folder")
			} else {
				assert.JSONEq(t, string(want), string(list.Result), "tools/list parity with heart")
				assert.False(t, names["download_file"] || names["upload_local_file"], "file tools are the full tier's")
			}
			require.NotEmpty(t, names)
			t.Logf("%s tier: %d tools", tier, len(names))

			space := readAndWrite(t, c, tier, names)
			if tier == "full" {
				fileRoundTrip(t, c, space, filesDir)
			}

			assert.Equal(t, 0, c.close(), "stdin EOF is a clean exit")
		})
	}

	t.Run("an id with HTML characters comes back as this request's result", func(t *testing.T) {
		// heart echoes the id with & < > escaped; it is still this reply
		c := startForwarder(t, bin, store, "it-full")
		outputs = append(outputs, c)
		require.Nil(t, c.request("initialize", initParams).Error)
		r := c.requestAs(`"a&b<c>"`, "tools/call", map[string]any{"name": "list_spaces", "arguments": map[string]any{}})
		require.Nil(t, r.Error, "%+v", r.Error)
		var res toolResult
		require.NoError(t, json.Unmarshal(r.Result, &res))
		assert.False(t, res.IsError, res.text())
		assert.Contains(t, res.text(), `"data"`)
		assert.Equal(t, 0, c.close())
	})

	t.Run("a running forwarder follows a retarget: the old port dies, the next request succeeds", func(t *testing.T) {
		// the item first names a loopback relay to heart, so the port the
		// child starts on can be taken away
		relay := newRelay(t, port)
		stamp := set("it-relay", "full", relay.port)
		c := startForwarder(t, bin, store, "it-relay")
		outputs = append(outputs, c)
		require.Nil(t, c.request("initialize", initParams).Error)
		require.Nil(t, c.request("tools/list", nil).Error)
		require.NotZero(t, relay.accepted.Load(), "the child really went through the relay")

		m, code := runVerb(t, bin, store, "", "key", "retarget", "--connection", "it-relay", "--expect", stamp,
			"--account", heart.AccountId, "--port", strconv.Itoa(port), "--tier", "full")
		require.Equal(t, 0, code, "%v", m)
		relay.close()
		time.Sleep(300 * time.Millisecond) // the child's idle connection notices the close

		// the same child: refused on the old port, re-reads the item, retries on heart's
		r := c.request("tools/list", nil)
		require.Nil(t, r.Error, "%+v", r.Error)
		assert.Equal(t, 0, c.close())
	})

	t.Run("a revoked key gets the 401 reply", func(t *testing.T) {
		c := startForwarder(t, bin, store, "it-full")
		outputs = append(outputs, c)
		require.Nil(t, c.request("initialize", initParams).Error)
		require.Nil(t, c.request("tools/list", nil).Error)

		apps, err := heart.GRPCClient().AccountLocalLinkListApps(heart.GRPCContext(ctx), &pb.RpcAccountLocalLinkListAppsRequest{})
		require.NoError(t, err)
		revoked := 0
		for _, app := range apps.App {
			if app.AppName != appName {
				continue
			}
			resp, err := heart.GRPCClient().AccountLocalLinkRevokeApp(heart.GRPCContext(ctx), &pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: app.AppHash})
			require.NoError(t, err)
			require.Equal(t, pb.RpcAccountLocalLinkRevokeAppResponseError_NULL, resp.Error.GetCode(), resp.Error.GetDescription())
			revoked++
		}
		require.NotZero(t, revoked, "the key to revoke was not listed")

		r := c.request("tools/list", nil)
		require.NotNil(t, r.Error, "a revoked key must not be served")
		assert.Equal(t, -32001, r.Error.Code)
		assert.Contains(t, r.Error.Message, "reconnect")
		assert.Equal(t, 0, c.close())
	})

	for _, c := range outputs {
		assert.NotContains(t, c.stdout.String(), heart.APIKey, "the key is in no stdout")
		assert.NotContains(t, c.stderr.String(), heart.APIKey, "the key is in no stderr")
	}
}

// the curated tiers list spaces as "name — id", the full tier as JSON
var (
	curatedSpaceRe = regexp.MustCompile(`— (\S+)`)
	fullSpaceRe    = regexp.MustCompile(`"id":"([^"]+)"`)
)

// readAndWrite makes one read and one write through the tier's own tools
// and reads the write back.
func readAndWrite(t *testing.T, c *client, tier string, names map[string]bool) (space string) {
	name := "anytype-mcp integration " + tier
	listSpaces, create, list := "spaces", "create", "find"
	if tier == "full" {
		listSpaces, create, list = "list_spaces", "create_object", "list_objects"
	}
	for _, n := range []string{listSpaces, create, list} {
		require.True(t, names[n], "the %s tier has no %s tool", tier, n)
	}

	spaces := c.tool(listSpaces, nil)
	require.False(t, spaces.IsError, spaces.text())
	re := curatedSpaceRe
	if tier == "full" {
		re = fullSpaceRe
	}
	m := re.FindStringSubmatch(spaces.text())
	require.Len(t, m, 2, "no space id in: %s", spaces.text())
	space = m[1]

	// the write, then a listing (not a full-text search: its index lags)
	args, listArgs := map[string]any{"space": space, "type": "page", "name": name, "markdown": "written through the forwarder"},
		map[string]any{"space": space, "type": "page"}
	if tier == "full" {
		args = map[string]any{"space_id": space, "type": "page", "name": name, "markdown": "written through the forwarder"}
		listArgs = map[string]any{"space_id": space}
	}
	created := c.tool(create, args)
	require.False(t, created.IsError, created.text())
	require.Eventually(t, func() bool {
		found := c.tool(list, listArgs)
		return !found.IsError && strings.Contains(found.text(), name)
	}, 30*time.Second, 500*time.Millisecond, "the write is readable")
	return space
}

// fileRoundTrip uploads a file from the forwarder's folder through
// upload_local_file, downloads it back through download_file and compares
// the bytes. A path that leaves the folder never reaches heart.
func fileRoundTrip(t *testing.T, c *client, space, filesDir string) {
	content := make([]byte, 3<<20+11)
	_, err := rand.Read(content)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(filesDir, "round trip.bin"), content, 0o600))
	secret := filepath.Join(filepath.Dir(filesDir), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("not yours"), 0o600))

	for _, path := range []string{secret, "../secret.txt"} {
		refused := c.tool("upload_local_file", map[string]any{"space_id": space, "path": path})
		require.True(t, refused.IsError, refused.text())
		assert.Contains(t, refused.text(), filesDir)
	}

	dry := c.tool("upload_local_file", map[string]any{"space_id": space, "path": "round trip.bin", "dry_run": true})
	require.False(t, dry.IsError, dry.text())
	var dryReceipt struct {
		DryRun bool   `json:"dry_run"`
		ID     string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(dry.text()), &dryReceipt))
	assert.True(t, dryReceipt.DryRun)
	assert.Empty(t, dryReceipt.ID, "a dry run stores nothing")

	up := c.tool("upload_local_file", map[string]any{"space_id": space, "path": "round trip.bin"})
	require.False(t, up.IsError, up.text())
	var stored struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(up.text()), &stored))
	require.NotEmpty(t, stored.ID, up.text())

	var receipt struct {
		Path      string `json:"path"`
		Filename  string `json:"filename"`
		MediaType string `json:"media_type"`
		Size      int64  `json:"size"`
	}
	require.Eventually(t, func() bool {
		down := c.tool("download_file", map[string]any{"space_id": space, "file_id": stored.ID})
		if down.IsError {
			t.Logf("download_file: %s", down.text())
			return false
		}
		return json.Unmarshal([]byte(down.text()), &receipt) == nil
	}, 60*time.Second, time.Second, "the upload can be downloaded")
	t.Logf("downloaded as %s (%s, %d bytes)", receipt.Filename, receipt.MediaType, receipt.Size)
	assert.Equal(t, filepath.Join(filesDir, receipt.Filename), receipt.Path)
	assert.NotEqual(t, "round trip.bin", receipt.Filename, "the upload's own file is not replaced")
	assert.Equal(t, int64(len(content)), receipt.Size)
	got, err := os.ReadFile(receipt.Path)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(content, got), "the bytes that went up came back")
	kept, err := os.ReadFile(filepath.Join(filesDir, "round trip.bin"))
	require.NoError(t, err)
	assert.True(t, bytes.Equal(content, kept))

	missing := c.tool("download_file", map[string]any{"space_id": space, "file_id": "bafyreinosuchfile"})
	assert.True(t, missing.IsError, missing.text())
	list, err := os.ReadDir(filesDir)
	require.NoError(t, err)
	assert.Len(t, list, 2, "no part file is left")
}

// relay is a loopback TCP relay to heart: a port a test can close.
type relay struct {
	ln       net.Listener
	port     int
	accepted atomic.Int32
	mu       sync.Mutex
	conns    []net.Conn
}

func newRelay(t *testing.T, target int) *relay {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	r := &relay{ln: ln, port: ln.Addr().(*net.TCPAddr).Port}
	t.Cleanup(r.close)
	go func() {
		for {
			in, err := ln.Accept()
			if err != nil {
				return
			}
			out, err := net.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(target)))
			if err != nil {
				_ = in.Close()
				continue
			}
			r.accepted.Add(1)
			r.mu.Lock()
			r.conns = append(r.conns, in, out)
			r.mu.Unlock()
			go func() { _, _ = io.Copy(out, in); _ = out.Close() }()
			go func() { _, _ = io.Copy(in, out); _ = in.Close() }()
		}
	}()
	return r
}

func (r *relay) close() {
	_ = r.ln.Close()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		_ = c.Close()
	}
}
