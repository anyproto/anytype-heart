//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

func startForwarder(t *testing.T, bin, dir, connection string) *client {
	cmd := exec.Command(bin, "serve", "--connection", connection, "--store-dir", dir, "--lock-dir", dir)
	cmd.Env = []string{"HOME=" + t.TempDir()}
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
	msg := map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method}
	if params != nil {
		msg["params"] = params
	}
	line, err := json.Marshal(msg)
	require.NoError(c.t, err)
	_, err = c.stdin.Write(append(line, '\n'))
	require.NoError(c.t, err)
	select {
	case got, ok := <-c.lines:
		require.True(c.t, ok, "the forwarder closed stdout; stderr:\n%s", c.stderr.String())
		var r rpcReply
		require.NoError(c.t, json.Unmarshal([]byte(got), &r), "stdout line: %s", got)
		require.Equal(c.t, strconv.Itoa(c.nextID), string(r.ID), "one reply, with this request's id")
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
	out, err := exec.Command("go", "build", "-tags", "teststore", "-o", bin, "..").CombinedOutput()
	require.NoError(t, err, string(out))
	return bin
}

func runVerb(t *testing.T, bin, stdin string, args ...string) (map[string]any, int) {
	cmd := exec.Command(bin, args...)
	cmd.Env = []string{"HOME=" + t.TempDir()}
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
	store := t.TempDir()
	stamps := map[string]string{}
	for _, tier := range []string{"small", "large", "full"} {
		payload, _ := json.Marshal(map[string]any{"key": heart.APIKey, "account": heart.AccountId, "port": port, "tier": tier})
		m, code := runVerb(t, bin, string(payload)+"\n", "key", "set", "--connection", "it-"+tier, "--expect", "absent", "--store-dir", store, "--lock-dir", store)
		require.Equal(t, 0, code, "key set: %v", m)
		stamps[tier] = fmt.Sprintf("%s@%d", m["fingerprint"], int64(m["rev"].(float64)))
	}

	var outputs []*client
	for _, tier := range []string{"small", "large", "full"} {
		t.Run(tier, func(t *testing.T) {
			c := startForwarder(t, bin, store, "it-"+tier)
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

			// parity: the forwarder adds and hides nothing
			d := &direct{t: t, url: heart.APIURL + "/mcp/" + tier, key: heart.APIKey}
			_, status := d.post("initialize", initParams, 1)
			require.Equal(t, http.StatusOK, status)
			d.version = "2025-06-18"
			_, status = d.post("notifications/initialized", nil, nil)
			require.Equal(t, http.StatusAccepted, status)
			want, status := d.post("tools/list", nil, 2)
			require.Equal(t, http.StatusOK, status)
			assert.JSONEq(t, string(want), string(list.Result), "tools/list parity with heart")

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
			require.NotEmpty(t, names)
			t.Logf("%s tier: %d tools", tier, len(names))

			readAndWrite(t, c, tier, names)

			assert.Equal(t, 0, c.close(), "stdin EOF is a clean exit")
		})
	}

	t.Run("heart still accepts a retargeted key, and the running forwarder keeps working", func(t *testing.T) {
		c := startForwarder(t, bin, store, "it-full")
		outputs = append(outputs, c)
		require.Nil(t, c.request("initialize", initParams).Error)
		m, code := runVerb(t, bin, "", "key", "retarget", "--connection", "it-full", "--expect", stamps["full"], "--account", heart.AccountId,
			"--port", strconv.Itoa(port), "--tier", "full", "--store-dir", store, "--lock-dir", store)
		require.Equal(t, 0, code, "%v", m)
		stamps["full"] = fmt.Sprintf("%s@%d", m["fingerprint"], int64(m["rev"].(float64)))
		require.Nil(t, c.request("tools/list", nil).Error)
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
func readAndWrite(t *testing.T, c *client, tier string, names map[string]bool) {
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
	space := m[1]

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
}
