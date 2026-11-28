package forward

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

func download(id any) string {
	return namedCall(id, toolDownload, map[string]any{"space_id": "s", "file_id": "file-1"})
}

func upload(id any) string {
	return namedCall(id, toolUpload, map[string]any{"space_id": "s", "path": "up.txt"})
}

// unreachable asserts the gate's answer: -32001, "open Anytype".
func unreachable(t *testing.T, g got) {
	t.Helper()
	require.NotNil(t, g.Error, g.raw)
	assert.Equal(t, codeNotConnected, g.Error.Code)
	assert.Equal(t, msgUnreachable, g.Error.Message)
}

func TestGateRhythm(t *testing.T) {
	t.Run("the endpoint is read again only when the last good read is 5 s old", func(t *testing.T) {
		h := start(t)
		h.initialize()
		reads := h.ep.readCount()
		h.call(toolCall(1))
		h.call(toolCall(2))
		assert.Equal(t, reads, h.ep.readCount())
		h.clock.advance(5 * time.Second)
		h.call(toolCall(3))
		assert.Equal(t, reads+1, h.ep.readCount())
	})

	t.Run("after a failed read: at most one read a second, never the last port", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.ep.fail(&store.Error{Reason: store.ReasonDenied, Path: "/x", Rule: "a rule"})
		h.clock.advance(6 * time.Second)
		before := len(h.heart.requests())
		reads := h.ep.readCount()
		unreachable(t, h.call(toolCall(1)))
		unreachable(t, h.call(toolCall(2)))
		assert.Equal(t, reads+1, h.ep.readCount(), "one read in the second")
		h.clock.advance(time.Second)
		unreachable(t, h.call(toolCall(3)))
		assert.Equal(t, reads+2, h.ep.readCount())
		assert.Len(t, h.heart.requests(), before, "nothing was sent")
		require.Eventually(t, func() bool { return strings.Contains(h.log.String(), "denied: a rule: /x") },
			3*time.Second, 5*time.Millisecond, "the rule and the path, once")
		assert.Equal(t, 1, strings.Count(h.log.String(), "a rule"))
	})
}

// H1 case 4: a good endpoint, then made unsafe in each way the rules
// know: no request leaves while it lasts (MCP, upload, download, the
// session DELETE), and a repair brings everything back.
func TestH1_04_UnsafeEndpointSendsNothing(t *testing.T) {
	type damage struct {
		hurt, repair func(t *testing.T, f *store.File)
	}
	rewrite := func(t *testing.T, f *store.File, port int) {
		require.NoError(t, os.Remove(f.Path()))
		writeEndpointFile(t, f.Path(), port)
	}
	cases := map[string]func(port int) damage{
		"deleted": func(port int) damage {
			return damage{
				func(t *testing.T, f *store.File) { require.NoError(t, os.Remove(f.Path())) },
				func(t *testing.T, f *store.File) { writeEndpointFile(t, f.Path(), port) },
			}
		},
		"made a symlink": func(port int) damage {
			return damage{
				func(t *testing.T, f *store.File) {
					good := filepath.Join(t.TempDir(), "endpoint.json")
					writeEndpointFile(t, good, port)
					require.NoError(t, os.Remove(f.Path()))
					if err := os.Symlink(good, f.Path()); err != nil {
						t.Skip("this system can't make a symlink here")
					}
				},
				func(t *testing.T, f *store.File) { rewrite(t, f, port) },
			}
		},
	}
	if runtime.GOOS != "windows" {
		cases["owned by another user"] = func(int) damage {
			me := os.Geteuid()
			return damage{
				func(t *testing.T, f *store.File) {
					f.OwnerRule(func(path string) int {
						if path == f.Path() {
							return me + 1
						}
						return me
					})
				},
				func(t *testing.T, f *store.File) { f.OwnerRule(func(string) int { return me }) },
			}
		}
		cases["group-writable"] = func(int) damage {
			return damage{
				func(t *testing.T, f *store.File) { require.NoError(t, os.Chmod(f.Path(), 0o664)) },
				func(t *testing.T, f *store.File) { require.NoError(t, os.Chmod(f.Path(), 0o644)) },
			}
		}
	}
	if runtime.GOOS == "darwin" {
		cases["given a macOS ACL"] = func(int) damage {
			return damage{
				func(t *testing.T, f *store.File) {
					out, err := exec.Command("/bin/chmod", "+a", "everyone allow read", f.Path()).CombinedOutput()
					require.NoError(t, err, string(out))
				},
				func(t *testing.T, f *store.File) {
					require.NoError(t, exec.Command("/bin/chmod", "-N", f.Path()).Run())
				},
			}
		}
	}
	if runtime.GOOS == "windows" {
		cases["given a Windows grant to Users"] = func(port int) damage {
			return damage{
				func(t *testing.T, f *store.File) {
					out, err := exec.Command("icacls", f.Path(), "/grant", "*S-1-5-32-545:R").CombinedOutput()
					require.NoError(t, err, string(out))
				},
				func(t *testing.T, f *store.File) { rewrite(t, f, port) },
			}
		}
	}

	for name, damaged := range cases {
		t.Run(name+": MCP, download and upload", func(t *testing.T) {
			var f *store.File
			h := start(t, withTier("full"), withFileEndpoint(&f), withFiles(pdf))
			d := damaged(h.heart.port())
			h.initialize()
			h.put("up.txt", []byte("hello"))
			before := len(h.heart.requests())
			d.hurt(t, f)
			h.clock.advance(6 * time.Second)
			for i, line := range []string{toolsList(1), download(2), upload(3)} {
				unreachable(t, h.call(line))
				h.clock.advance(time.Second)
				assert.Len(t, h.heart.requests(), before, "request %d left", i)
			}
			d.repair(t, f)
			h.clock.advance(time.Second)
			for _, line := range []string{toolsList(4), download(5), upload(6)} {
				g := h.call(line)
				require.Nil(t, g.Error, g.raw)
				assert.False(t, g.isError(), g.raw)
			}
		})

		t.Run(name+": the session DELETE", func(t *testing.T) {
			var f *store.File
			h := start(t, withFileEndpoint(&f))
			d := damaged(h.heart.port())
			h.initialize()
			d.hurt(t, f)
			h.clock.advance(6 * time.Second)
			unreachable(t, h.call(toolCall(1)))
			d.repair(t, f)
			h.clock.advance(time.Second)
			assert.Equal(t, "done", h.call(toolCall(2)).text())
			d.hurt(t, f)
			h.clock.advance(6 * time.Second)
			require.NoError(t, h.in.Close())
			assert.Equal(t, 0, h.waitExit())
			assert.Zero(t, h.deletes(""), "no DELETE while the endpoint is unsafe")
		})
	}
}

func toolsList(id any) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"method":"tools/list"}`, id)
}

// H1 case 8: the old port is closed: the refused request reads the
// endpoint at once and is retried once, on the new port.
func TestH1_08_PortMoveOldPortClosed(t *testing.T) {
	h := start(t, withTier("full"))
	h.initialize()
	moved := newFakeHeart(t)
	h.stopHeart()
	h.ep.set(moved.port())
	reads := h.ep.readCount()
	g := h.call(toolCall(1))
	require.Nil(t, g.Error, g.raw)
	assert.Equal(t, "done", g.text())
	assert.Equal(t, 1, moved.count("tools/call"))
	assert.Equal(t, reads+1, h.ep.readCount(), "one read for the one retry")

	t.Run("refused on the new port too: open Anytype", func(t *testing.T) {
		moved.srv.Close()
		h.opts.httpClient.CloseIdleConnections()
		unreachable(t, h.call(toolCall(2)))
	})
}

// H1 case 9: the old port still answers: the new one is used once the
// last good read is 5 s old.
func TestH1_09_PortMoveOldPortAnswering(t *testing.T) {
	h := start(t, withTier("full"))
	h.initialize()
	moved := newFakeHeart(t)
	h.ep.set(moved.port())
	h.clock.advance(4900 * time.Millisecond)
	assert.Equal(t, "done", h.call(toolCall(1)).text())
	assert.Equal(t, 1, h.heart.count("tools/call"))
	h.clock.advance(100 * time.Millisecond)
	assert.Equal(t, "done", h.call(toolCall(2)).text())
	assert.Equal(t, 1, moved.count("tools/call"))
	assert.Equal(t, 1, h.heart.count("tools/call"))
}

// H1 case 10: a curated tier moves: one renewal on the new port, the
// first tools/call gets the reset warning and does not run, and no DELETE
// goes to the old port.
func TestH1_10_PortMoveSmallTier(t *testing.T) {
	h := start(t)
	h.initialize()
	moved := newFakeHeart(t)
	h.retarget(moved)
	g := h.call(toolCall(1))
	assert.True(t, g.isError(), g.raw)
	assert.Equal(t, msgExpired, g.text())
	assert.Equal(t, 1, moved.count("initialize"), "one renewal")
	h.warned()
	assert.Equal(t, "done", h.call(toolCall(2)).text())
	assert.Equal(t, 1, moved.count("initialize"))
	require.NoError(t, h.in.Close())
	assert.Equal(t, 0, h.waitExit())
	assert.Zero(t, h.deletes(""), "no DELETE to the old port")
	assert.Equal(t, 1, func() int {
		n := 0
		for _, r := range moved.requests() {
			if r.Method == http.MethodDelete {
				n++
			}
		}
		return n
	}(), "the renewed session is ended where it lives")
}

// heldFolder holds OpenUpload until released.
type heldFolder struct {
	folder
	opening chan struct{}
	release chan struct{}
}

func (f heldFolder) OpenUpload(path string) (io.ReadCloser, string, error) {
	close(f.opening)
	<-f.release
	return f.folder.OpenUpload(path)
}

// H1 case 11: the endpoint is withdrawn, or moves to another live port,
// in the middle of a transfer, with no time passing: the request after it
// never leaves (each request of a transfer reads the endpoint afresh), the
// call is a tool error, and nothing is replayed.
func TestH1_11_EndpointWithdrawnMidTransfer(t *testing.T) {
	const moved = "Anytype moved to another port during the transfer, which was stopped. Try again."
	for name, c := range map[string]struct {
		change func(h *harness, other *fakeHeart)
		text   string
	}{
		"withdrawn":                  {func(h *harness, _ *fakeHeart) { h.ep.set(0) }, msgUnreachable},
		"moved to another live port": {func(h *harness, other *fakeHeart) { h.ep.set(other.port()) }, moved},
	} {
		t.Run(name+" between a download's metadata request and its content request", func(t *testing.T) {
			h := fileTools(t, withFiles(pdf))
			other := newFakeHeart(t)
			other.files = pdf
			h.heart.setHook(func(_ http.ResponseWriter, _ *http.Request, rec recorded) bool {
				if strings.Contains(rec.Path, "/objects/") {
					c.change(h, other)
				}
				return false
			})
			g := h.tool(toolDownload, map[string]any{"space_id": "s", "file_id": "file-1"})
			assert.True(t, g.isError())
			assert.Equal(t, c.text, g.text())
			require.Len(t, h.rest(), 1, "the content request never left")
			assert.Contains(t, h.rest()[0].Path, "/objects/")
			assert.Empty(t, other.requests(), "nor went anywhere else")
			list, err := os.ReadDir(h.root)
			require.NoError(t, err)
			assert.Empty(t, list, "nothing saved")
		})

		t.Run(name+" while an upload waits to open its file", func(t *testing.T) {
			opening, release := make(chan struct{}), make(chan struct{})
			h := fileTools(t, withOptions(func(o *Options) {
				o.wrapFolder = func(f folder) folder { return heldFolder{f, opening, release} }
			}))
			other := newFakeHeart(t)
			h.put("up.txt", []byte("hello"))
			h.send(upload(1))
			<-opening
			c.change(h, other)
			close(release)
			g := h.recv()
			assert.True(t, g.isError(), g.raw)
			assert.Equal(t, c.text, g.text())
			assert.Empty(t, h.rest(), "the upload's only request never left")
			assert.Empty(t, other.requests(), "nor went anywhere else")
			assert.Zero(t, h.heart.storedUploads()+other.storedUploads())
		})
	}
}

// H1 case 12: a 401 fails the request, which is never replayed.
func TestH1_12_RejectedKeyIsNotReplayed(t *testing.T) {
	h := fileTools(t, withFiles(pdf))
	h.put("up.txt", []byte("hello"))
	h.heart.setKey("CANARY-another-key-0123456789")
	reads := h.ep.readCount()

	g := h.call(toolCall(1))
	require.NotNil(t, g.Error, g.raw)
	assert.Equal(t, codeNotConnected, g.Error.Code)
	assert.Equal(t, msgRejected, g.Error.Message)
	assert.Equal(t, 1, h.heart.count("tools/call"))
	assert.Equal(t, reads, h.ep.readCount(), "a 401 is no reason to read the endpoint")

	g = h.tool(toolDownload, map[string]any{"space_id": "s", "file_id": "file-1"})
	assert.Equal(t, msgRejected, g.text())
	g = h.tool(toolUpload, map[string]any{"space_id": "s", "path": "up.txt"})
	assert.Equal(t, msgRejected, g.text())
	require.Len(t, h.rest(), 2, "one metadata request, one upload: no content request, nothing replayed")
	assert.Zero(t, h.heart.storedUploads())
}

// splitFolder hands an upload's bytes over one at a time, so the key
// spans every read boundary.
type splitFolder struct{ folder }

func (f splitFolder) OpenUpload(path string) (io.ReadCloser, string, error) {
	rc, name, err := f.folder.OpenUpload(path)
	if err != nil {
		return nil, "", err
	}
	return struct {
		io.Reader
		io.Closer
	}{iotest.OneByteReader(rc), rc}, name, nil
}

// H1 case 14: an upload whose bytes contain the connection's key is
// aborted with a tool error, and heart stores nothing.
func TestH1_14_UploadCarryingTheKey(t *testing.T) {
	for name, wrap := range map[string]func(folder) folder{
		"whole reads":     nil,
		"one byte a read": func(f folder) folder { return splitFolder{f} },
	} {
		t.Run(name, func(t *testing.T) {
			h := fileTools(t, withOptions(func(o *Options) { o.wrapFolder = wrap }))
			for file, content := range map[string]string{
				"plain.txt":   "config: " + canaryKey + "\n",
				"escaped.txt": `{"key":"` + strings.ReplaceAll(canaryKey, "-", `\u002d`) + `"}`,
			} {
				if file == "escaped.txt" {
					require.NotContains(t, content, canaryKey, "the key only behind escapes")
				}
				h.put(file, []byte(content))
				g := h.tool(toolUpload, map[string]any{"space_id": "s", "path": file})
				assert.True(t, g.isError(), file)
				assert.Equal(t, "The file contains this connection's key and was not uploaded.", g.text(), file)
			}
			assert.Zero(t, h.heart.storedUploads(), "heart stored nothing")
			for _, r := range h.rest() {
				assert.NotContains(t, r.Body, canaryKey, "the key never left whole")
			}
			h.put("fine.txt", []byte("no secret here"))
			assert.False(t, h.tool(toolUpload, map[string]any{"space_id": "s", "path": "fine.txt"}).isError())
			assert.Equal(t, 1, h.heart.storedUploads())
		})
	}
}

func TestBearerOnlyToLoopback(t *testing.T) {
	client := newHTTPClient()
	tr := client.Transport.(*http.Transport)
	assert.Nil(t, tr.Proxy)
	for _, addr := range []string{"localhost:80", "10.0.0.1:80", "[::1]:80", "example.com:443"} {
		_, err := tr.DialContext(t.Context(), "tcp", addr)
		require.Error(t, err, addr)
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	conn, err := tr.DialContext(t.Context(), "tcp", ln.Addr().String())
	require.NoError(t, err)
	_ = conn.Close()
	assert.Equal(t, "http://127.0.0.1:31009/mcp/full", endpoint(snapshot{port: 31009, tier: "full"}))

	t.Run("a redirect is not followed", func(t *testing.T) {
		h := start(t, withTier("full"))
		h.initialize()
		other := newFakeHeart(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, rec recorded) bool {
			http.Redirect(w, r, other.srv.URL+"/mcp/full", http.StatusTemporaryRedirect)
			return true
		})
		g := h.call(toolCall(1))
		assert.True(t, g.isError(), g.raw)
		assert.Empty(t, other.requests())
	})
}

// Theme A: a read that came back after its deadline never authorizes a
// send: the next attempt reads afresh.
func TestH1R1_A_LateReadNeverAuthorizes(t *testing.T) {
	b := &blockingEndpoint{}
	h := start(t, withBlockingEndpoint(b, false))
	h.initialize()
	sent := len(h.heart.requests())
	b.blocked.Store(true)
	h.clock.advance(6 * time.Second)
	unreachable(t, h.call(toolCall(1)))
	reads := h.ep.readCount()
	b.blocked.Store(false)
	close(b.forever) // the stuck read comes back now, with heart's port
	require.Eventually(t, func() bool { return h.ep.readCount() == reads+1 }, 3*time.Second, time.Millisecond)
	h.ep.set(0) // and then the desktop withdraws the endpoint
	h.clock.advance(time.Second)
	unreachable(t, h.call(toolCall(2)))
	assert.Len(t, h.heart.requests(), sent, "the late answer sent nothing")
}

// Theme B: every authenticated request is one attempt on a new
// connection, so Go's transport never replays a GET behind the gate.
func TestH1R1_B_NoReplayBehindTheGate(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	var contentGets atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				br := bufio.NewReader(conn)
				for {
					req, err := http.ReadRequest(br)
					if err != nil {
						return
					}
					_, _ = io.Copy(io.Discard, req.Body)
					if strings.HasSuffix(req.URL.Path, "/content") {
						// read, then hung up on before any answer
						contentGets.Add(1)
						return
					}
					body := `{"object":"object","properties":{"name":"report","file_ext":"pdf"}}`
					_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
					if req.Close {
						return
					}
				}
			}()
		}
	}()
	h := fileTools(t)
	h.ep.set(ln.Addr().(*net.TCPAddr).Port)
	h.clock.advance(6 * time.Second)
	g := h.tool(toolDownload, map[string]any{"space_id": "s", "file_id": "file-1"})
	assert.True(t, g.isError(), g.raw)
	assert.EqualValues(t, 1, contentGets.Load(), "the content GET left once")
}

// Theme F: a curated session lives on the port that opened it. Moved to
// another port, even one that reaches the same heart, the first tool call
// gets the reset warning without running, and the new session is the one
// ended at shutdown.
func TestH1R1_F_SessionOnAnotherPortIsGone(t *testing.T) {
	h := start(t)
	h.initialize()
	same := httptest.NewServer(http.HandlerFunc(h.heart.serve))
	t.Cleanup(same.Close)
	h.ep.set(same.Listener.Addr().(*net.TCPAddr).Port)
	h.clock.advance(6 * time.Second)
	g := h.call(toolCall(1))
	assert.Equal(t, msgExpired, g.text(), g.raw)
	assert.Zero(t, h.heart.count("tools/call"), "the call never ran")
	h.warned()
	assert.Equal(t, "done", h.call(toolCall(2)).text())
	require.NoError(t, h.in.Close())
	assert.Equal(t, 0, h.waitExit())
	assert.Equal(t, 1, h.deletes("session-2"), "the session opened on the new port is ended there")
	assert.Zero(t, h.deletes("session-1"), "never a DELETE for the old one on the new port")
}

// COR-05: the renewal's initialize gets the one refusal retry, and an
// endpoint withdrawn between its two legs is "open Anytype".
func TestH1R1_COR05_RenewalFollowsTheEndpoint(t *testing.T) {
	t.Run("refused between the 404 and the renewal", func(t *testing.T) {
		h := start(t)
		h.initialize()
		moved := newFakeHeart(t)
		h.heart.setHook(func(w http.ResponseWriter, _ *http.Request, rec recorded) bool {
			if rec.RPC != "tools/call" {
				return false
			}
			rest(w, http.StatusNotFound, "unknown or expired MCP session")
			h.ep.set(moved.port())
			_ = h.heart.srv.Listener.Close()
			return true
		})
		g := h.call(toolCall(1))
		assert.Equal(t, msgExpired, g.text(), g.raw)
		assert.Equal(t, 1, moved.count("initialize"))
	})

	t.Run("withdrawn between the renewal's two legs", func(t *testing.T) {
		h := start(t)
		h.initialize()
		h.heart.expireSessions()
		h.heart.setHook(func(_ http.ResponseWriter, _ *http.Request, rec recorded) bool {
			if rec.RPC == "initialize" && strings.Contains(rec.Body, "anytype-mcp-renew-") {
				h.ep.set(0)
			}
			return false
		})
		unreachable(t, h.call(ping(7)))
		assert.Equal(t, 1, h.heart.count("notifications/initialized"), "the client's own; the second leg never left")
	})
}

// COR-06: refusals that meet at once share one read of a missing
// endpoint.
func TestH1R1_COR06_RefusalsShareOneRead(t *testing.T) {
	h := start(t, withTier("full"))
	h.initialize()
	h.stopHeart()
	h.ep.set(0)
	reads := h.ep.readCount()
	for i := range contract.Workers {
		h.send(toolCall(i))
	}
	for range contract.Workers {
		unreachable(t, h.recv())
	}
	assert.LessOrEqual(t, h.ep.readCount()-reads, 1)
}

// COR-07: a request renews its session once; a second 404 is an answer,
// not a second renewal.
func TestH1R1_COR07_OneRenewalPerRequest(t *testing.T) {
	h := start(t)
	h.initialize()
	h.heart.setHook(func(w http.ResponseWriter, _ *http.Request, rec recorded) bool {
		if rec.RPC == "tools/list" {
			rest(w, http.StatusNotFound, "unknown or expired MCP session")
			return true
		}
		return false
	})
	g := h.call(toolsList(5))
	require.NotNil(t, g.Error, g.raw)
	assert.Equal(t, codeInternal, g.Error.Code)
	assert.Equal(t, 2, h.heart.count("initialize"), "the client's and one renewal")
}

// Theme H: every attempt that carries a curated session id, and a
// client's own re-initialize, follow the rule that a session lives on the
// port that opened it.
func TestH1R2_H_SessionPortOnEveryPath(t *testing.T) {
	t.Run("a notification refused on the old port is not retried on the new one", func(t *testing.T) {
		h := start(t)
		h.initialize()
		same := httptest.NewServer(http.HandlerFunc(h.heart.serve))
		t.Cleanup(same.Close)
		h.stopHeart()
		h.ep.set(same.Listener.Addr().(*net.TCPAddr).Port)
		h.send(`{"jsonrpc":"2.0","method":"notifications/roots/list_changed"}`)
		require.Eventually(t, func() bool { return strings.Contains(h.log.String(), "its session is on a port Anytype has left") },
			3*time.Second, 5*time.Millisecond)
		assert.Zero(t, h.heart.count("notifications/roots/list_changed"), "the old session never went to the new port")
	})

	t.Run("a client re-initialize after a move still gets the reset warning first", func(t *testing.T) {
		h := start(t)
		h.initialize()
		same := httptest.NewServer(http.HandlerFunc(h.heart.serve))
		t.Cleanup(same.Close)
		h.ep.set(same.Listener.Addr().(*net.TCPAddr).Port)
		h.clock.advance(6 * time.Second)
		h.initialize()
		g := h.call(toolCall(1))
		assert.Equal(t, msgExpired, g.text(), g.raw)
		assert.Zero(t, h.heart.count("tools/call"), "the call never ran")
		h.warned()
		assert.Equal(t, "done", h.call(toolCall(2)).text())
	})
}

// slowRead makes one read seem to take longer than the read timeout: the
// clock moves on while it runs, as for a process suspended right after it.
type slowRead struct {
	Endpoint
	clock *clock
	once  atomic.Bool
}

func (s *slowRead) Read(ctx context.Context) (int, error) {
	port, err := s.Endpoint.Read(ctx)
	if s.once.CompareAndSwap(false, true) {
		s.clock.advance(6 * time.Second)
	}
	return port, err
}

// Theme I: a result's age counts from the moment its read started; one
// that is only taken after the read timeout authorizes nothing.
func TestH1R2_I_ResultAgeCountsFromTheStart(t *testing.T) {
	h := start(t, func(h *harness) { h.opts.Endpoint = &slowRead{Endpoint: h.ep, clock: h.clock} })
	unreachable(t, h.call(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	assert.Empty(t, h.heart.requests(), "the late result sent nothing")
	h.clock.advance(time.Second)
	h.initialize()
}
