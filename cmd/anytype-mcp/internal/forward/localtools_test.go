package forward

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

var pdf = map[string]fakeFile{"file-1": {name: "report.pdf", mediaType: "application/pdf", bytes: []byte("%PDF-1.7 hello")}}

func withFiles(files map[string]fakeFile) hopt {
	return func(h *harness) { h.heart.files = files }
}

func withHeartTools(tools ...any) hopt {
	return func(h *harness) { h.heart.tools = tools }
}

func withTransferTimeout(d time.Duration) hopt {
	return withOptions(func(o *Options) { o.TransferTimeout = d })
}

// fileTools starts an initialized full-tier connection.
func fileTools(t *testing.T, opts ...hopt) *harness {
	h := start(t, append([]hopt{withTier("full")}, opts...)...)
	h.initialize()
	return h
}

func namedCall(id any, name string, args map[string]any) string {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	return string(b)
}

func (h *harness) tool(name string, args map[string]any) got {
	h.t.Helper()
	g := h.call(namedCall("t", name, args))
	require.Nil(h.t, g.Error, g.raw)
	return g
}

func (g got) structured(t *testing.T) map[string]any {
	t.Helper()
	var r struct {
		StructuredContent map[string]any `json:"structuredContent"`
	}
	require.NoError(t, json.Unmarshal(g.Result, &r))
	return r.StructuredContent
}

// rest are the /v2 requests heart saw.
func (h *harness) rest() []recorded {
	var out []recorded
	for _, r := range h.heart.requests() {
		if strings.HasPrefix(r.Path, "/v2/") {
			out = append(out, r)
		}
	}
	return out
}

func (h *harness) content() recorded {
	h.t.Helper()
	for _, r := range h.rest() {
		if strings.HasSuffix(r.Path, "/content") {
			return r
		}
	}
	h.t.Fatal("no download request")
	return recorded{}
}

func (h *harness) entries() []string {
	h.t.Helper()
	list, err := os.ReadDir(h.root)
	require.NoError(h.t, err)
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func (h *harness) put(name string, content []byte) {
	h.t.Helper()
	path := filepath.Join(h.root, filepath.FromSlash(name))
	require.NoError(h.t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(h.t, os.WriteFile(path, content, 0o600))
}

func (h *harness) noSecret() {
	h.t.Helper()
	assert.NotContains(h.t, h.all.String(), canaryKey, "the key reached stdout")
	assert.NotContains(h.t, h.log.String(), canaryKey, "the key reached the log")
}

func listedNames(t *testing.T, g got) (names []string, raw []json.RawMessage) {
	t.Helper()
	require.Nil(t, g.Error, g.raw)
	var r struct {
		Tools []json.RawMessage `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(g.Result, &r))
	for _, tool := range r.Tools {
		var n struct {
			Name string `json:"name"`
		}
		require.NoError(t, json.Unmarshal(tool, &n))
		names = append(names, n.Name)
	}
	return names, r.Tools
}

var heartTools = []any{
	map[string]any{"name": "list_spaces", "description": "a <b> & c", "inputSchema": map[string]any{"type": "object"}},
	map[string]any{"name": "upload_file", "description": "from a url", "inputSchema": map[string]any{"type": "object"}},
}

const listLine = `{"jsonrpc":"2.0","id":"l","method":"tools/list"}`

func TestLocalToolsListing(t *testing.T) {
	t.Run("the full tier lists heart's tools unchanged, then the two file tools naming the folder", func(t *testing.T) {
		h := fileTools(t, withHeartTools(heartTools...))
		names, raw := listedNames(t, h.call(listLine))
		assert.Equal(t, []string{"list_spaces", "upload_file", "download_file", "upload_local_file"}, names)
		want, _ := json.Marshal(heartTools)
		got, _ := json.Marshal(raw[:2])
		assert.JSONEq(t, string(want), string(got))
		for _, tool := range raw[2:] {
			var def struct {
				Description string `json:"description"`
				InputSchema struct {
					Type     string   `json:"type"`
					Required []string `json:"required"`
				} `json:"inputSchema"`
			}
			require.NoError(t, json.Unmarshal(tool, &def))
			assert.Contains(t, def.Description, h.root, "the description states the folder's absolute path")
			assert.NotContains(t, def.Description, "temporary")
			assert.Equal(t, "object", def.InputSchema.Type)
			assert.Contains(t, def.InputSchema.Required, "space_id")
		}
		assert.Contains(t, h.log.String(), "file tools: folder "+h.root)
	})

	t.Run("a tool heart serves itself is listed once and called on heart", func(t *testing.T) {
		own := map[string]any{"name": "download_file", "description": "served by Anytype", "inputSchema": map[string]any{"type": "object"}}
		h := fileTools(t, withHeartTools(append(append([]any{}, heartTools...), own)...), withFiles(pdf))
		names, raw := listedNames(t, h.call(listLine))
		assert.Equal(t, []string{"list_spaces", "upload_file", "download_file", "upload_local_file"}, names)
		assert.Contains(t, string(raw[2]), "served by Anytype")

		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		assert.Equal(t, "done", g.text(), "heart's own answer")
		assert.Empty(t, h.rest())
		assert.Empty(t, h.entries())
		// the other one is still local
		h.put("a.txt", []byte("x"))
		g = h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		assert.Equal(t, "uploaded-file-id", g.structured(t)["id"])
	})

	t.Run("a paged listing gets them on its last page", func(t *testing.T) {
		h := fileTools(t)
		h.heart.setHook(func(w http.ResponseWriter, _ *http.Request, rec recorded) bool {
			if rec.RPC != "tools/list" {
				return false
			}
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Params struct {
					Cursor string `json:"cursor"`
				} `json:"params"`
			}
			_ = json.Unmarshal([]byte(rec.Body), &msg)
			if msg.Params.Cursor == "" {
				rpcResult(w, msg.ID, map[string]any{"tools": []any{map[string]any{"name": "upload_local_file"}}, "nextCursor": "p2"})
			} else {
				rpcResult(w, msg.ID, map[string]any{"tools": []any{heartTools[0]}})
			}
			return true
		})
		names, _ := listedNames(t, h.call(listLine))
		assert.Equal(t, []string{"upload_local_file"}, names)
		// between the pages: the name heart has just listed is heart's
		h.put("a.txt", []byte("x"))
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		assert.Equal(t, "done", g.text(), "heart's own answer")
		assert.Empty(t, h.rest())
		names, _ = listedNames(t, h.call(`{"jsonrpc":"2.0","id":"l2","method":"tools/list","params":{"cursor":"p2"}}`))
		assert.Equal(t, []string{"list_spaces", "download_file"}, names, "a name heart listed on an earlier page is heart's")
	})

	for _, tier := range []string{"small", "large"} {
		t.Run("the "+tier+" tier has no file tools, and their names go to heart", func(t *testing.T) {
			h := start(t, withTier(tier), withHeartTools(heartTools...), withFiles(pdf))
			h.initialize()
			names, _ := listedNames(t, h.call(listLine))
			assert.Equal(t, []string{"list_spaces", "upload_file"}, names)
			g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
			assert.Equal(t, "done", g.text())
			assert.Empty(t, h.rest())
			assert.Nil(t, h.srv.local)
		})
	}

	t.Run("a file tool before initialize does not run", func(t *testing.T) {
		h := start(t, withTier("full"), withFiles(pdf))
		g := h.call(namedCall(2, "download_file", map[string]any{"space_id": "s", "file_id": "file-1"}))
		require.NotNil(t, g.Error)
		assert.Equal(t, codeNotInitialized, g.Error.Code)
		assert.Nil(t, h.call(`{"jsonrpc":"2.0","id":3,"method":"ping"}`).Error)
		assert.Empty(t, h.heart.requests())
		assert.Empty(t, h.entries())
	})

	t.Run("without an item there is no folder and no tool", func(t *testing.T) {
		h := start(t, withoutItem(), withTempFiles())
		g := h.call(namedCall(2, "download_file", map[string]any{"space_id": "s", "file_id": "f"}))
		require.NotNil(t, g.Error)
		assert.Equal(t, codeNotConnected, g.Error.Code)
		assert.Nil(t, h.srv.local)
	})
}

func TestTemporaryFilesFolder(t *testing.T) {
	folder := func(h *harness) string {
		_, raw := listedNames(t, h.call(listLine))
		var def struct {
			Description string `json:"description"`
		}
		require.NoError(t, json.Unmarshal(raw[0], &def))
		m := regexp.MustCompile(`Download a file's bytes into (.+) on the computer running`).FindStringSubmatch(def.Description)
		require.Len(t, m, 2, def.Description)
		path := m[1]
		t.Cleanup(func() { _ = os.RemoveAll(path) })
		return path
	}
	a := fileTools(t, withTempFiles(), withFiles(pdf))
	b := fileTools(t, withTempFiles())
	dirA, dirB := folder(a), folder(b)
	assert.NotEqual(t, dirA, dirB, "a new folder for every process")
	for _, dir := range []string{dirA, dirB} {
		assert.True(t, filepath.IsAbs(dir))
		assert.Equal(t, filepath.Clean(os.TempDir()), filepath.Dir(dir))
		assert.True(t, strings.HasPrefix(filepath.Base(dir), "anytype-mcp-files-"))
		fi, err := os.Lstat(dir)
		require.NoError(t, err)
		assert.True(t, fi.IsDir())
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
		}
	}
	_, raw := listedNames(t, a.call(listLine))
	assert.Contains(t, string(raw[0]), "temporary and new for every session")

	g := a.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
	require.False(t, g.isError(), g.text())
	assert.Equal(t, filepath.Join(dirA, "report.pdf"), g.structured(t)["path"])

	// the folder and the download outlive the process; nothing else does
	_ = a.in.Close()
	assert.Equal(t, 0, a.waitExit())
	list, err := os.ReadDir(dirA)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "report.pdf", list[0].Name())
}

func TestDownloadFile(t *testing.T) {
	t.Run("saves the whole file into the folder, private, named after the object", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		g := h.tool("download_file", map[string]any{"space_id": "space-1", "file_id": "file-1"})
		require.False(t, g.isError(), g.text())
		want := map[string]any{"path": filepath.Join(h.root, "report.pdf"), "filename": "report.pdf", "media_type": "application/pdf", "size": float64(14)}
		assert.Equal(t, want, g.structured(t))
		var inText map[string]any
		require.NoError(t, json.Unmarshal([]byte(g.text()), &inText))
		assert.Equal(t, want, inText)
		raw, err := os.ReadFile(filepath.Join(h.root, "report.pdf"))
		require.NoError(t, err)
		assert.Equal(t, "%PDF-1.7 hello", string(raw))
		if runtime.GOOS != "windows" {
			fi, err := os.Stat(filepath.Join(h.root, "report.pdf"))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
		}
		c := h.content()
		assert.Equal(t, http.MethodGet, c.Method)
		assert.Equal(t, "/v2/spaces/space-1/files/file-1/content", c.Path)
		assert.Equal(t, "", c.Query)
		assert.Equal(t, "Bearer "+canaryKey, c.Auth)
		assert.Empty(t, c.Header.Get("Range"))
		assert.Equal(t, []string{"report.pdf"}, h.entries())
		h.noSecret()
	})

	for stored, mediaType := range map[string]string{
		"notes.markdown": "text/plain", "report.v2.pdf": "application/pdf", "archive.tar.gz": "application/gzip", "example.cmd.txt": "text/plain",
	} {
		t.Run("joins the object's stem and file_ext for "+stored, func(t *testing.T) {
			h := fileTools(t, withFiles(map[string]fakeFile{"f": {name: stored, mediaType: mediaType, bytes: []byte("x")}}))
			g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "f"})
			assert.Equal(t, stored, g.structured(t)["filename"])
		})
	}

	t.Run("a Content-Disposition name is used whole, the extended form first", func(t *testing.T) {
		for header, want := range map[string]string{
			`attachment; filename="Q3 report.pdf"`:                             "Q3 report.pdf",
			`attachment; filename="plain.bin"; filename*=UTF-8''%E5%A0%B1.txt`: "報.txt",
			`attachment; filename="../../etc/passwd"`:                          "_.._etc_passwd",
			`attachment; filename="CON.txt"`:                                   "download.txt",
		} {
			h := fileTools(t, withFiles(pdf))
			h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
				if !strings.HasSuffix(r.URL.Path, "/content") {
					return false
				}
				w.Header().Set("Content-Type", "application/pdf")
				w.Header().Set("Content-Disposition", header)
				_, _ = io.WriteString(w, "x")
				return true
			})
			g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
			assert.Equal(t, want, g.structured(t)["filename"], header)
			assert.Equal(t, []string{want}, h.entries())
		}
	})

	t.Run("falls back to the id and the media type's extension", func(t *testing.T) {
		assert.Equal(t, "Q3 report.pdf", fileName("Q3 report.pdf", "ignored", ".txt", "application/pdf", "id"))
		assert.Equal(t, "notes.txt", fileName("", "notes", "", "text/plain", "id"))
		assert.Equal(t, "notes.txt", fileName("", "notes.txt", "", "text/plain", "id"))
		assert.Equal(t, "bafyid", fileName("", "", "", "application/x-unknown", "bafyid"))
		assert.Equal(t, "bafyid.png", fileName("", "", "", "image/png", "bafyid"))
	})

	t.Run("never overwrites: each download gets a new name", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		h.put("report.pdf", []byte("mine"))
		first := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		second := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		assert.Equal(t, "report (2).pdf", first.structured(t)["filename"])
		assert.Equal(t, "report (3).pdf", second.structured(t)["filename"])
		raw, _ := os.ReadFile(filepath.Join(h.root, "report.pdf"))
		assert.Equal(t, "mine", string(raw))
	})

	t.Run("downloads a long international name twice", func(t *testing.T) {
		h := fileTools(t, withFiles(map[string]fakeFile{"f": {name: strings.Repeat("報", 83) + ".pdf", mediaType: "application/pdf", bytes: []byte("x")}}))
		first := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "f"})
		second := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "f"})
		require.False(t, first.isError(), first.text())
		require.False(t, second.isError(), second.text())
		assert.True(t, strings.HasSuffix(second.structured(t)["filename"].(string), " (2).pdf"))
	})

	t.Run("saves nothing that is not HTTP 200", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Range", "bytes 0-1/14")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "%P")
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		assert.True(t, g.isError())
		assert.Contains(t, g.text(), "expected the whole file (HTTP 200), got HTTP 206; nothing was saved")
		assert.Empty(t, h.entries())
	})

	t.Run("forwards an image width and refuses anything but a whole number", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1", "width": 320})
		require.False(t, g.isError(), g.text())
		assert.Equal(t, "width=320", h.content().Query)
		// the schema says integer: every spelling of one is taken
		for spelled, sent := range map[string]string{"320.0": "width=320", "3.2e2": "width=320", "1000000": "width=1000000", "0": "width=0"} {
			g := h.call(`{"jsonrpc":"2.0","id":"w","method":"tools/call","params":{"name":"download_file","arguments":{"space_id":"s","file_id":"file-1","width":` + spelled + `}}}`)
			require.False(t, g.isError(), "%s: %s", spelled, g.text())
			assert.Equal(t, sent, h.rest()[len(h.rest())-1].Query, spelled)
		}
		before := len(h.rest())
		for _, width := range []string{"-1", "1.5", `"320"`, "true", "null", "1e99", "4294967296", "[1]"} {
			g := h.call(`{"jsonrpc":"2.0","id":"w","method":"tools/call","params":{"name":"download_file","arguments":{"space_id":"s","file_id":"file-1","width":` + width + `}}}`)
			assert.True(t, g.isError(), width)
			assert.Contains(t, g.text(), "width must be", width)
		}
		assert.Len(t, h.rest(), before)
	})

	t.Run("does not follow a redirect", func(t *testing.T) {
		h := fileTools(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasPrefix(r.URL.Path, "/v2/") {
				return false
			}
			w.Header().Set("Location", "http://example.invalid/elsewhere")
			w.WriteHeader(http.StatusFound)
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "f"})
		assert.True(t, g.isError())
		assert.Contains(t, g.text(), "HTTP 302 with a redirect, which is not followed")
		var paths []string
		for _, r := range h.rest() {
			paths = append(paths, r.Path)
		}
		assert.Equal(t, []string{"/v2/spaces/s/objects/f", "/v2/spaces/s/files/f/content"}, paths)
	})

	t.Run("returns Anytype's error envelope in band", func(t *testing.T) {
		h := fileTools(t)
		g := h.tool("download_file", map[string]any{"space_id": "space-1", "file_id": "missing"})
		assert.True(t, g.isError())
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(g.text()), &e))
		assert.Equal(t, "not_found", e["code"])
		assert.Equal(t, float64(404), e["status"])
		assert.Empty(t, h.entries())
	})

	t.Run("refuses anything but one path segment as an id, before calling Anytype", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		for _, id := range []any{"", ".", "..", "a/b", `a\b`, 7, nil} {
			g := h.tool("download_file", map[string]any{"space_id": "space-1", "file_id": id})
			assert.True(t, g.isError())
			assert.Contains(t, g.text(), "file_id must be an id")
			g = h.tool("download_file", map[string]any{"space_id": id, "file_id": "file-1"})
			assert.Contains(t, g.text(), "space_id must be an id")
		}
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1", "path": "/etc"})
		assert.Contains(t, g.text(), "unexpected: path")
		g = h.call(`{"jsonrpc":"2.0","id":"t","method":"tools/call","params":{"name":"download_file","arguments":[1]}}`)
		assert.True(t, g.isError())
		assert.Empty(t, h.rest())
	})

	t.Run("an id is one escaped segment of the route", func(t *testing.T) {
		h := fileTools(t)
		h.tool("download_file", map[string]any{"space_id": "s?x=1", "file_id": "f#y %"})
		r := h.rest()
		require.NotEmpty(t, r)
		assert.Equal(t, "/v2/spaces/s?x=1/objects/f#y %", r[0].Path, "decoded by the server as one segment each")
		assert.Equal(t, "", r[0].Query)
	})

	t.Run("shows nothing under the final name until the file is complete", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		var midway []string
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = io.WriteString(w, "%PDF-1.7 ")
			w.(http.Flusher).Flush()
			require.Eventually(t, func() bool { midway = h.entries(); return len(midway) > 0 }, 3*time.Second, 5*time.Millisecond)
			time.Sleep(50 * time.Millisecond)
			midway = h.entries()
			_, _ = io.WriteString(w, "hello")
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		require.False(t, g.isError(), g.text())
		require.Len(t, midway, 1)
		assert.True(t, strings.HasPrefix(midway[0], ".anytype-mcp-") && strings.HasSuffix(midway[0], ".part"), midway[0])
		assert.Equal(t, []string{"report.pdf"}, h.entries())
		assert.Equal(t, float64(14), g.structured(t)["size"])
	})

	t.Run("a transfer that breaks removes its part file and nothing else", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		h.put("report.pdf", []byte("mine"))
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Length", "14")
			_, _ = io.WriteString(w, "%PDF")
			w.(http.Flusher).Flush()
			time.Sleep(30 * time.Millisecond)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		assert.True(t, g.isError())
		assert.Equal(t, "the file could not be transferred", g.text())
		assert.Equal(t, []string{"report.pdf"}, h.entries())
		raw, _ := os.ReadFile(filepath.Join(h.root, "report.pdf"))
		assert.Equal(t, "mine", string(raw))
	})

	t.Run("a transfer past the deadline ends, and its part file goes", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf), withTransferTimeout(300*time.Millisecond))
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = io.WriteString(w, "%PDF")
			w.(http.Flusher).Flush()
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		assert.True(t, g.isError())
		assert.Equal(t, "the transfer did not finish within 1 minutes", g.text())
		assert.Empty(t, h.entries())
	})

	t.Run("a canceled download writes no reply and leaves no part file", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		started := make(chan struct{})
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = io.WriteString(w, "%PDF")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			return true
		})
		h.send(namedCall(41, "download_file", map[string]any{"space_id": "s", "file_id": "file-1"}))
		<-started
		require.Eventually(t, func() bool { return len(h.entries()) == 1 }, 3*time.Second, 5*time.Millisecond)
		h.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":41}}`) //nolint:misspell
		require.Eventually(t, func() bool { return len(h.entries()) == 0 }, 3*time.Second, 5*time.Millisecond)
		h.silent()
		g := h.call(`{"jsonrpc":"2.0","id":42,"method":"ping"}`)
		assert.Equal(t, "42", string(g.ID), "one reply per live request, and none for the canceled one")
	})

	t.Run("a shutdown ends a running download and removes its part file", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		started := make(chan struct{})
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			_, _ = io.WriteString(w, "%PDF")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			return true
		})
		h.send(namedCall(1, "download_file", map[string]any{"space_id": "s", "file_id": "file-1"}))
		<-started
		require.Eventually(t, func() bool { return len(h.entries()) == 1 }, 3*time.Second, 5*time.Millisecond)
		h.cancel()
		assert.Equal(t, 0, h.waitExit())
		assert.Empty(t, h.entries())
	})

	t.Run("quotes nothing from a body that is not Anytype's error", func(t *testing.T) {
		h := fileTools(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasPrefix(r.URL.Path, "/v2/") {
				return false
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad gateway", "request_headers": map[string]string{"Authorization": "Bearer " + canaryKey}})
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "f"})
		assert.True(t, g.isError())
		assert.Equal(t, "Anytype answered HTTP 502", g.text())
		h.noSecret()
	})

	t.Run("keeps Anytype's documented members only, with the key redacted", func(t *testing.T) {
		h := fileTools(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": 404, "code": "not_found", "message": "no file for " + canaryKey,
				"issues": []map[string]any{{"path": "file_id", "message": "unknown id", "hint": "x"}},
				"debug":  map[string]any{"headers": map[string]string{"Authorization": "Bearer " + canaryKey}},
			})
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "f"})
		assert.JSONEq(t, `{"status":404,"code":"not_found","message":"no file for [redacted]","issues":[{"path":"file_id","message":"unknown id"}]}`, g.text())
		h.noSecret()
	})

	t.Run("a name or a media type carrying the key is not echoed", func(t *testing.T) {
		h := fileTools(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			w.Header().Set("Content-Type", "application/x-"+strings.ToLower(canaryKey))
			w.Header().Set("Content-Disposition", `attachment; filename="`+canaryKey+`.txt"`)
			_, _ = io.WriteString(w, "x")
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "f"})
		require.False(t, g.isError(), g.text())
		assert.Equal(t, "redacted.txt", g.structured(t)["filename"])
		h.noSecret()
	})

	t.Run("a rejected key fails this transfer and the next one runs on the new item", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		const next = "NEXT-key-0123456789abcdefghijklmnopqrstuvwxyz"
		h.heart.mu.Lock()
		h.heart.key = next
		h.heart.mu.Unlock()
		h.st.Put(testConn, store.Item{V: 1, Rev: 2, Key: next, Account: testAccount, Port: h.heart.port(), Tier: "full"})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		assert.True(t, g.isError())
		assert.Equal(t, msgRejected, g.text())
		assert.Empty(t, h.entries())
		require.Len(t, h.rest(), 1, "the rejected name lookup ends the transfer: no content request follows")
		assert.Equal(t, "/v2/spaces/s/objects/file-1", h.rest()[0].Path)
		g = h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		require.False(t, g.isError(), g.text())
		assert.Equal(t, "Bearer "+next, h.rest()[len(h.rest())-1].Auth)
	})

	t.Run("reports an unreachable Anytype in band", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		h.stopHeart()
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		assert.True(t, g.isError())
		assert.Equal(t, msgStart, g.text())
	})

	t.Run("a file that contains the key is not saved", func(t *testing.T) {
		h := fileTools(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			// a gateway's diagnostic page that reflects the request, the
			// key split over two writes
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, strings.Repeat("x", 70<<10)+"Authorization: Bearer "+canaryKey[:9])
			w.(http.Flusher).Flush()
			time.Sleep(30 * time.Millisecond)
			_, _ = io.WriteString(w, canaryKey[9:]+"\n")
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "f"})
		assert.True(t, g.isError())
		assert.Equal(t, "The file contains this connection's key and was not saved.", g.text())
		assert.Empty(t, h.entries())
		h.noSecret()
	})

	t.Run("a long error from Anytype still arrives as JSON", func(t *testing.T) {
		h := fileTools(t)
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			issues := make([]map[string]any, 30)
			for i := range issues {
				issues[i] = map[string]any{"path": strings.Repeat("p", 500), "message": strings.Repeat("m", 500)}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 400, "code": "validation", "message": strings.Repeat("long ", 300), "issues": issues})
			return true
		})
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "f"})
		assert.True(t, g.isError())
		var e struct {
			Code   string `json:"code"`
			Issues []struct {
				Path string `json:"path"`
			} `json:"issues"`
		}
		require.NoError(t, json.Unmarshal([]byte(g.text()), &e), g.text())
		assert.Equal(t, "validation", e.Code)
		assert.Len(t, e.Issues, 20)
		assert.Less(t, len(g.text()), 16<<10)
	})

	t.Run("stdin ending during a stalled download still removes the part file", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		started := make(chan struct{})
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasSuffix(r.URL.Path, "/content") {
				return false
			}
			_, _ = io.WriteString(w, "%PDF")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			return true
		})
		h.send(namedCall(1, "download_file", map[string]any{"space_id": "s", "file_id": "file-1"}))
		<-started
		require.Eventually(t, func() bool { return len(h.entries()) == 1 }, 3*time.Second, 5*time.Millisecond)
		require.NoError(t, h.in.Close())
		assert.Equal(t, 1, h.waitExit(), "its reply was lost")
		assert.Empty(t, h.entries())
	})

	t.Run("a folder that was moved gets no path in the receipt", func(t *testing.T) {
		h := fileTools(t, withFiles(pdf))
		moved := h.root + "-moved"
		if err := os.Rename(h.root, moved); err != nil {
			t.Skipf("an open folder can't be renamed here: %v", err)
		}
		require.NoError(t, os.Mkdir(h.root, 0o700))
		g := h.tool("download_file", map[string]any{"space_id": "s", "file_id": "file-1"})
		assert.True(t, g.isError())
		assert.Contains(t, g.text(), "the file was saved as report.pdf, but the folder")
		assert.Contains(t, g.text(), "was moved or replaced")
		assert.Empty(t, h.entries(), "nothing went to the folder now at the path")
		_, err := os.Stat(filepath.Join(moved, "report.pdf"))
		require.NoError(t, err)
	})
}

func TestKeyGuard(t *testing.T) {
	const key = `k"ey/with<odd>&chars\1234567890`
	plain, _ := json.Marshal(map[string]string{"a": key})
	var loose bytes.Buffer
	enc := json.NewEncoder(&loose)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{"a": key})
	slashed := bytes.ReplaceAll(loose.Bytes(), []byte("/"), []byte(`\/`))
	for name, body := range map[string][]byte{"raw": []byte("xx" + key + "yy"), "JSON": plain, "JSON without HTML escapes": loose.Bytes(), "JSON with escaped slashes": slashed} {
		// every way of cutting the body in two reads
		for cut := 0; cut <= len(body); cut++ {
			guard := newKeyGuard(io.MultiReader(bytes.NewReader(body[:cut]), bytes.NewReader(body[cut:])), key)
			_, err := io.ReadAll(guard)
			require.ErrorIs(t, err, errCarriesKey, "%s cut at %d", name, cut)
		}
	}
	clean := bytes.Repeat([]byte(key[:len(key)-1]), 1000)
	got, err := io.ReadAll(newKeyGuard(bytes.NewReader(clean), key))
	require.NoError(t, err)
	assert.Equal(t, clean, got, "what carries no key passes through unchanged")
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func (h *harness) posts() []recorded {
	var out []recorded
	for _, r := range h.rest() {
		if r.Method == http.MethodPost {
			out = append(out, r)
		}
	}
	return out
}

func TestUploadLocalFile(t *testing.T) {
	t.Run("uploads a file from the folder as the multipart form heart's route takes", func(t *testing.T) {
		h := fileTools(t)
		h.put("notes/today.txt", []byte("hello from disk"))
		g := h.tool("upload_local_file", map[string]any{"space_id": "space-1", "path": "notes/today.txt"})
		require.False(t, g.isError(), g.text())
		assert.Equal(t, map[string]any{"object": "file", "id": "uploaded-file-id"}, g.structured(t))
		assert.JSONEq(t, `{"object":"file","id":"uploaded-file-id"}`, g.text())
		require.Len(t, h.rest(), 1)
		r := h.rest()[0]
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v2/spaces/space-1/files", r.Path)
		assert.Equal(t, "", r.Query)
		assert.Equal(t, "Bearer "+canaryKey, r.Auth)
		assert.Regexp(t, uuidRe, r.Header.Get("Idempotency-Key"))
		assert.Regexp(t, `^multipart/form-data; boundary=anytype-mcp-`, r.Header.Get("Content-Type"))
		assert.Contains(t, r.Body, `name="file"; filename="today.txt"`)
		assert.Contains(t, r.Body, "\r\n\r\nhello from disk\r\n--")
		h.noSecret()

		h.tool("upload_local_file", map[string]any{"space_id": "space-1", "path": "notes/today.txt"})
		assert.NotEqual(t, r.Header.Get("Idempotency-Key"), h.rest()[1].Header.Get("Idempotency-Key"), "a fresh key per call")
	})

	t.Run("streams a large file intact", func(t *testing.T) {
		h := fileTools(t)
		big := make([]byte, 5<<20+17)
		_, _ = rand.Read(big)
		h.put("big.bin", big)
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "big.bin"})
		require.False(t, g.isError(), g.text())
		assert.True(t, bytes.Contains([]byte(h.rest()[0].Body), big))
	})

	t.Run("passes dry_run through", func(t *testing.T) {
		h := fileTools(t)
		h.put("a.txt", []byte("x"))
		g := h.tool("upload_local_file", map[string]any{"space_id": "space-1", "path": "a.txt", "dry_run": true})
		require.False(t, g.isError(), g.text())
		assert.Equal(t, map[string]any{"dry_run": true}, g.structured(t))
		assert.Equal(t, "/v2/spaces/space-1/files", h.rest()[0].Path)
		assert.Equal(t, "dry_run=true", h.rest()[0].Query)
		// anything but the two literals is an error, never an upload: a
		// client may spell an unset option as null
		for _, value := range []string{`"yes"`, "null", "1", `"true"`, "[]"} {
			g = h.call(`{"jsonrpc":"2.0","id":"d","method":"tools/call","params":{"name":"upload_local_file","arguments":{"space_id":"space-1","path":"a.txt","dry_run":` + value + `}}}`)
			assert.True(t, g.isError(), value)
			assert.Contains(t, g.text(), "dry_run must be true or false", value)
		}
		require.Len(t, h.rest(), 1, "none of them reached Anytype")
		g = h.tool("upload_local_file", map[string]any{"space_id": "space-1", "path": "a.txt", "dry_run": false})
		assert.Equal(t, "", h.rest()[1].Query)
		assert.False(t, g.isError())
	})

	t.Run("refuses every path that is not a regular file inside the folder, naming the folder", func(t *testing.T) {
		h := fileTools(t)
		require.NoError(t, os.MkdirAll(filepath.Join(h.root, "sub"), 0o700))
		outside := filepath.Join(filepath.Dir(h.root), "outside.txt")
		require.NoError(t, os.WriteFile(outside, []byte("not yours"), 0o600))
		for path, message := range map[string]string{
			"/etc/passwd":              "must be relative to",
			outside:                    "must be relative to",
			"~/secret.txt":             "must be relative to",
			`\\server\share\x.txt`:     "must be relative to",
			"//server/share/x.txt":     "must be relative to",
			`C:\Users\x.txt`:           "must be relative to",
			"c:x.txt":                  "must be relative to",
			"../outside.txt":           "is outside",
			"sub/../../outside.txt":    "is outside",
			"sub/missing/../x.txt":     "is outside",
			"..":                       "is outside",
			"nope.txt":                 "there is no file nope.txt in",
			`..\outside.txt`:           "",
			"sub":                      "is not a regular file",
			".":                        "is not a regular file",
			"":                         "path must name a file in",
			"  ":                       "path must name a file in",
			"a\x00b":                   "must be relative to",
			"sub/" + outside:           "",
			strings.Repeat("a/", 5000): "",
		} {
			g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": path})
			assert.True(t, g.isError(), path)
			assert.Contains(t, g.text(), message, path)
			assert.Contains(t, g.text(), h.root, path)
		}
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": 7})
		assert.Contains(t, g.text(), "path must name a file in")
		g = h.tool("upload_local_file", map[string]any{"space_id": "s"})
		assert.Contains(t, g.text(), "path must name a file in")
		assert.Empty(t, h.rest(), "nothing reached Anytype")
	})

	t.Run("refuses links: one that leads out, one that stays in, and a linked folder", func(t *testing.T) {
		h := fileTools(t)
		outside := filepath.Join(filepath.Dir(h.root), "outside")
		require.NoError(t, os.MkdirAll(outside, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not yours"), 0o600))
		h.put("real.txt", []byte("mine"))
		if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(h.root, "link.txt")); err != nil {
			t.Skipf("no symbolic links here: %v", err)
		}
		require.NoError(t, os.Symlink("real.txt", filepath.Join(h.root, "inner.txt")))
		require.NoError(t, os.Symlink(outside, filepath.Join(h.root, "dir")))
		require.NoError(t, os.Symlink("../outside", filepath.Join(h.root, "rel")))
		for path, message := range map[string]string{
			"link.txt":       "is a link, which is not followed",
			"inner.txt":      "is a link, which is not followed",
			"dir/secret.txt": "links that lead out of it are not followed",
			"rel/secret.txt": "links that lead out of it are not followed",
		} {
			g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": path})
			assert.True(t, g.isError(), path)
			assert.Contains(t, g.text(), message, path)
			assert.NotContains(t, g.text(), "not yours")
		}
		assert.Empty(t, h.rest())
	})

	t.Run("reports an unreachable Anytype in band", func(t *testing.T) {
		h := fileTools(t)
		h.put("a.txt", []byte("x"))
		h.stopHeart()
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		assert.True(t, g.isError())
		assert.Equal(t, msgStart, g.text())
	})

	t.Run("only heart's own receipt is a success", func(t *testing.T) {
		for name, c := range map[string]struct {
			status int
			ctype  string
			body   string
			dryRun bool
		}{
			"a login page":                 {200, "text/html", "<html>Please sign in</html>", false},
			"a 201 without an id":          {201, "application/json", `{"object":"file"}`, false},
			"a 201 that is not an object":  {201, "application/json", `["x"]`, false},
			"a 200 for a committed upload": {200, "application/json", `{"object":"file","id":"x"}`, false},
			"a 201 for a dry run":          {201, "application/json", `{"object":"file","id":"x"}`, true},
			"a dry run without dry_run":    {200, "application/json", `{"ok":true}`, true},
			"a 204":                        {204, "application/json", ``, false},
			"an id under another name":     {201, "application/json", `{"ID":"x"}`, false},
			"an id that is not a string":   {201, "application/json", `{"id":7}`, false},
			"a dry run under another name": {200, "application/json", `{"DRY_RUN":true}`, true},
			"a dry run that says false":    {200, "application/json", `{"dry_run":false}`, true},
		} {
			h := fileTools(t)
			h.put("a.txt", []byte("x"))
			h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
				if !strings.HasPrefix(r.URL.Path, "/v2/") {
					return false
				}
				w.Header().Set("Content-Type", c.ctype)
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
				return true
			})
			args := map[string]any{"space_id": "s", "path": "a.txt"}
			if c.dryRun {
				args["dry_run"] = true
			}
			g := h.tool("upload_local_file", args)
			assert.True(t, g.isError(), name)
			assert.Equal(t, fmt.Sprintf("the upload outcome is unconfirmed: Anytype answered HTTP %d without its upload receipt. Check the space for the file before uploading it again.", c.status), g.text(), name)
			assert.Len(t, h.posts(), 1, name)
		}
	})

	t.Run("a receipt that breaks off is unconfirmed, not a failed transfer", func(t *testing.T) {
		h := fileTools(t)
		h.put("a.txt", []byte("x"))
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasPrefix(r.URL.Path, "/v2/") {
				return false
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "40")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"ba`)
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
			return true
		})
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		assert.True(t, g.isError())
		assert.Contains(t, g.text(), "upload outcome is unconfirmed: Anytype answered HTTP 201")
	})

	t.Run("a receipt that arrives slowly, within the transfer's time, is a success", func(t *testing.T) {
		h := fileTools(t, withOptions(func(o *Options) { o.ErrorBodyWait = 50 * time.Millisecond }))
		h.put("a.txt", []byte("x"))
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasPrefix(r.URL.Path, "/v2/") {
				return false
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"object":"file",`)
			w.(http.Flusher).Flush()
			time.Sleep(400 * time.Millisecond)
			_, _ = io.WriteString(w, `"id":"late"}`)
			return true
		})
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		require.False(t, g.isError(), g.text())
		assert.Equal(t, "late", g.structured(t)["id"])
	})

	t.Run("a file that also has a name elsewhere is not uploaded", func(t *testing.T) {
		h := fileTools(t)
		secret := filepath.Join(filepath.Dir(h.root), "secret.txt")
		require.NoError(t, os.WriteFile(secret, []byte("not yours"), 0o600))
		if err := os.Link(secret, filepath.Join(h.root, "innocent.txt")); err != nil {
			t.Skipf("no hard links here: %v", err)
		}
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "innocent.txt"})
		assert.True(t, g.isError())
		assert.Contains(t, g.text(), "more than one name")
		assert.Empty(t, h.rest())
	})

	t.Run("a connection lost after the body was sent is unconfirmed, and the upload is not sent again", func(t *testing.T) {
		h := fileTools(t)
		h.put("a.txt", []byte("x"))
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasPrefix(r.URL.Path, "/v2/") {
				return false
			}
			// the whole upload has been read; the connection drops before
			// any header
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
			return true
		})
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		assert.True(t, g.isError())
		assert.Equal(t, "the upload outcome is unconfirmed: the connection to Anytype closed before it answered. Check the space for the file before uploading it again.", g.text())
		time.Sleep(50 * time.Millisecond)
		assert.Len(t, h.posts(), 1, "never replayed")
	})

	t.Run("no answer by the deadline after the whole file was sent is unconfirmed", func(t *testing.T) {
		h := fileTools(t, withTransferTimeout(300*time.Millisecond))
		h.put("a.txt", []byte("x"))
		h.heart.setHook(func(_ http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasPrefix(r.URL.Path, "/v2/") {
				return false
			}
			<-r.Context().Done()
			return true
		})
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		assert.True(t, g.isError())
		assert.Equal(t, "the upload outcome is unconfirmed: Anytype did not answer within 1 minutes after the whole file was sent. Check the space for the file before uploading it again.", g.text())
		assert.Len(t, h.posts(), 1)
	})

	t.Run("the deadline while the file is still being sent is a plain unfinished transfer", func(t *testing.T) {
		// a listener that accepts and never reads: the upload stalls in
		// the socket buffers
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		var held []net.Conn
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				held = append(held, conn)
			}
		}()
		h := fileTools(t, withTransferTimeout(400*time.Millisecond))
		h.put("big.bin", make([]byte, 64<<20))
		h.st.Put(testConn, store.Item{V: 1, Rev: 2, Key: canaryKey, Account: testAccount, Port: ln.Addr().(*net.TCPAddr).Port, Tier: "full"})
		h.clock.advance(time.Minute)
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "big.bin"})
		assert.True(t, g.isError())
		assert.Equal(t, "the transfer did not finish within 1 minutes", g.text())
	})

	t.Run("a receipt is honoured although the key was rotated while the upload ran", func(t *testing.T) {
		h := fileTools(t)
		h.put("a.txt", []byte("x"))
		const next = "NEXT-key-0123456789abcdefghijklmnopqrstuvwxyz"
		h.heart.setHook(func(_ http.ResponseWriter, r *http.Request, _ recorded) bool {
			if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/") {
				// heart has taken the upload under the old key; the item
				// moves on before the receipt arrives
				h.st.Put(testConn, store.Item{V: 1, Rev: 2, Key: next, Account: testAccount, Port: h.heart.port(), Tier: "full"})
				h.clock.advance(time.Minute)
			}
			return false
		})
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		require.False(t, g.isError(), g.text())
		assert.Equal(t, "uploaded-file-id", g.structured(t)["id"])
		assert.Len(t, h.posts(), 1)
	})

	t.Run("a key rotated before the upload is the one that is sent", func(t *testing.T) {
		h := fileTools(t)
		h.put("a.txt", []byte("x"))
		const next = "NEXT-key-0123456789abcdefghijklmnopqrstuvwxyz"
		h.heart.mu.Lock()
		h.heart.key = next
		h.heart.mu.Unlock()
		h.st.Put(testConn, store.Item{V: 1, Rev: 2, Key: next, Account: testAccount, Port: h.heart.port(), Tier: "full"})
		h.clock.advance(time.Minute)
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		require.False(t, g.isError(), g.text())
		assert.Equal(t, "Bearer "+next, h.posts()[0].Auth)
	})

	t.Run("a receipt carrying the key is withheld", func(t *testing.T) {
		h := fileTools(t)
		h.put("a.txt", []byte("x"))
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasPrefix(r.URL.Path, "/v2/") {
				return false
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"x","echo":"`+canaryKey+`"}`)
			return true
		})
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		assert.True(t, g.isError())
		assert.Equal(t, msgWithheld, g.text())
		assert.NotContains(t, h.all.String(), canaryKey)
	})

	t.Run("Anytype's refusal comes back in band, redacted", func(t *testing.T) {
		h := fileTools(t)
		h.put("a.txt", []byte("x"))
		h.heart.setHook(func(w http.ResponseWriter, r *http.Request, _ recorded) bool {
			if !strings.HasPrefix(r.URL.Path, "/v2/") {
				return false
			}
			rest(w, http.StatusRequestEntityTooLarge, "too large for "+canaryKey)
			return true
		})
		g := h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "a.txt"})
		assert.True(t, g.isError())
		assert.JSONEq(t, `{"status":413,"code":"x","message":"too large for [redacted]"}`, g.text())
		h.noSecret()
	})
}
