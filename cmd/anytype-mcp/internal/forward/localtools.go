package forward

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/files"
)

// The two tools the forwarder serves itself, beside heart's full tier.
// Both need the file system of the machine the AI app runs on, which
// heart's MCP server does not offer: its upload_file fetches a URL, and
// its download has no text rendering. Both are confined to one folder
// (internal/files) and use the connection's own key against heart's REST
// routes. A tool heart serves under the same name takes precedence.
const (
	toolDownload = "download_file"
	toolUpload   = "upload_local_file"

	// a file object's document is small: its blocks hold no content
	maxObjectBytes = 1 << 20
	// and so is an upload receipt
	maxReceiptBytes = 1 << 20

	msgCheckSpace = "Check the space for the file before uploading it again."
)

// usualExtensions are the extensions of the media types a download most
// often has; Anytype keeps a file's name without one.
var usualExtensions = map[string]string{
	"application/json": ".json",
	"application/pdf":  ".pdf",
	"application/zip":  ".zip",
	"audio/mpeg":       ".mp3",
	"image/gif":        ".gif",
	"image/jpeg":       ".jpg",
	"image/png":        ".png",
	"image/svg+xml":    ".svg",
	"image/webp":       ".webp",
	"text/csv":         ".csv",
	"text/html":        ".html",
	"text/markdown":    ".md",
	"text/plain":       ".txt",
	"video/mp4":        ".mp4",
}

// toolFailure is a failure whose text the model reads.
type toolFailure struct {
	text string
	// whole: the text is already bounded and free of the key, and must
	// arrive whole (heart's error as JSON)
	whole bool
}

func (f *toolFailure) Error() string { return f.text }

func failf(format string, args ...any) error {
	return &toolFailure{text: fmt.Sprintf(format, args...)}
}

type toolDefinition struct {
	name string
	raw  json.RawMessage
}

type localTools struct {
	s       *server
	dir     *files.Dir
	timeout time.Duration
	defs    []toolDefinition

	// running counts the transfers under way: shutdown leaves them time
	// to remove their part files
	running atomic.Int32

	mu sync.Mutex
	// served are the names heart listed, over the pages of one listing;
	// active are the local tools not among them
	served map[string]bool
	active map[string]bool
}

func newLocalTools(s *server, dir *files.Dir, timeout time.Duration) (*localTools, error) {
	defs, err := definitions(dir)
	if err != nil {
		return nil, err
	}
	l := &localTools{s: s, dir: dir, timeout: timeout, defs: defs, served: map[string]bool{}, active: map[string]bool{}}
	for _, d := range defs {
		l.active[d.name] = true
	}
	return l, nil
}

func definitions(dir *files.Dir) ([]toolDefinition, error) {
	where := dir.Path()
	kept := ""
	if dir.Temporary() {
		kept = " That directory is temporary and new for every session: tell the user to move a file they want to keep."
	}
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	list := []map[string]any{
		{
			"name": toolDownload,
			"description": "Download a file's bytes into " + where + " on the computer running this MCP server and return its local " +
				"path, filename, media_type and size. The file gets a new name there; nothing is overwritten. file_id is " +
				"the file object's id, as upload_file and upload_local_file return it and file and image blocks reference it." + kept,
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"space_id": str("Space id, from `list_spaces`"),
					"file_id":  str("File object id, or an icon_image value"),
					"width": map[string]any{"type": "integer", "minimum": 0,
						"description": "Image variant width in pixels; 0 or absent for the original"},
				},
				"required":             []string{"space_id", "file_id"},
				"additionalProperties": false,
			},
		},
		{
			"name": toolUpload,
			"description": "Upload a file from " + where + " on the computer running this MCP server into a space. Only files in that " +
				"directory can be uploaded, by a path relative to it: ask the user to put the file there first. Returns the stored file object's id, " +
				"which file blocks, image blocks and icon_image values reference. For a file on the web, use upload_file with its url.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"space_id": str("Space id, from `list_spaces`"),
					"path":     str("The file's path relative to " + where + ", e.g. report.pdf"),
					"dry_run":  map[string]any{"type": "boolean", "description": "Validate and report without committing"},
				},
				"required":             []string{"space_id", "path"},
				"additionalProperties": false,
			},
		},
	}
	out := make([]toolDefinition, 0, len(list))
	for _, def := range list {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(def); err != nil {
			return nil, err
		}
		name, _ := def["name"].(string)
		out = append(out, toolDefinition{name: name, raw: bytes.TrimSpace(buf.Bytes())})
	}
	return out, nil
}

// extend adds the local tools to heart's tools/list result, after heart's
// own and leaving out any name heart serves itself. With a paged listing
// they go on the last page. A result of another shape is passed on as it
// is.
func (l *localTools) extend(params, result json.RawMessage) json.RawMessage {
	var members map[string]json.RawMessage
	if json.Unmarshal(result, &members) != nil {
		return result
	}
	var listed []json.RawMessage
	if json.Unmarshal(members["tools"], &listed) != nil {
		return result
	}
	var p struct {
		Cursor string `json:"cursor"`
	}
	if len(params) > 0 && json.Unmarshal(params, &p) != nil {
		p.Cursor = ""
	}
	var next string
	if raw, ok := members["nextCursor"]; ok && json.Unmarshal(raw, &next) != nil {
		return result
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if p.Cursor == "" {
		l.served = map[string]bool{}
	}
	for _, raw := range listed {
		var tool struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &tool) == nil {
			l.served[tool.Name] = true
			// heart's from this moment, not only from the last page on
			delete(l.active, tool.Name)
		}
	}
	if next != "" {
		return result
	}
	active := map[string]bool{}
	for _, d := range l.defs {
		if !l.served[d.name] {
			active[d.name] = true
			listed = append(listed, d.raw)
		}
	}
	l.active = active
	if len(active) == 0 {
		return result
	}
	tools, err := json.Marshal(listed)
	if err != nil {
		return result
	}
	members["tools"] = tools
	extended, err := json.Marshal(members)
	if err != nil {
		return result
	}
	return extended
}

// claims reports whether a tools/call names a local tool that heart does
// not serve itself, and returns its arguments.
func (l *localTools) claims(params json.RawMessage) (name string, args json.RawMessage, ok bool) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(params, &p) != nil {
		return "", nil, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return p.Name, p.Arguments, l.active[p.Name]
}

func (l *localTools) minutes() int {
	return max(int((l.timeout+time.Minute/2)/time.Minute), 1)
}

// call runs one local tool to its single reply; nil when the request was
// canceled or the forwarder is stopping. The whole transfer runs on the
// snapshot it started with and ends within the transfer deadline. Failures
// the model can act on are tool results.
func (l *localTools) call(c *call, name string, rawArgs json.RawMessage) *reply {
	l.running.Add(1)
	defer l.running.Add(-1)
	id := c.msg.id
	snap, err := l.s.conn.current(c.ctx)
	if err != nil {
		return l.s.notConnected(id)
	}
	ctx, cancel := context.WithTimeout(c.ctx, l.timeout)
	defer cancel()

	var text string
	var structured json.RawMessage
	args := map[string]json.RawMessage{}
	if len(rawArgs) > 0 && !isNull(rawArgs) {
		if json.Unmarshal(rawArgs, &args) != nil {
			err = failf("%s takes its arguments as an object", name)
		}
	}
	if err == nil {
		switch name {
		case toolDownload:
			text, structured, err = l.download(ctx, snap, args)
		default:
			text, structured, err = l.upload(ctx, snap, args)
		}
	}

	var failure *toolFailure
	var refusal *files.Refusal
	switch {
	case err == nil:
		result, merr := json.Marshal(map[string]any{
			"content":           []map[string]string{{"type": "text", "text": text}},
			"structuredContent": structured,
		})
		if merr != nil {
			return toolErrorReply(id, "the file tool's result could not be written")
		}
		r, merr := resultReply(id, result)
		if merr != nil {
			return toolErrorReply(id, "the file tool's result could not be written")
		}
		return r
	case c.ctx.Err() != nil:
		return nil
	case errors.Is(err, errNotConnected):
		return l.s.notConnected(id)
	case errors.As(err, &failure):
		if failure.whole {
			return toolErrorReply(id, failure.text)
		}
		return toolErrorReply(id, redact(failure.text, snap.key))
	case ctx.Err() != nil:
		return toolErrorReply(id, fmt.Sprintf("the transfer did not finish within %d minutes", l.minutes()))
	case errors.As(err, &refusal):
		return toolErrorReply(id, redact(refusal.Text, snap.key))
	}
	// a file system or network error: logged, never quoted
	l.s.log.printf("%s: %s", name, redact(err.Error(), snap.key))
	return toolErrorReply(id, "the file could not be transferred")
}

func onlyArguments(tool string, args map[string]json.RawMessage, allowed ...string) error {
	var unexpected []string
	for key := range args {
		known := false
		for _, a := range allowed {
			known = known || a == key
		}
		if !known {
			unexpected = append(unexpected, key)
		}
	}
	if len(unexpected) == 0 {
		return nil
	}
	sort.Strings(unexpected)
	return failf("%s takes %s; unexpected: %s", tool, strings.Join(allowed, ", "), strings.Join(unexpected, ", "))
}

// segment is an argument that becomes one URL path segment: an id, never
// a way onto another route.
func segment(args map[string]json.RawMessage, key string) (raw, escaped string, err error) {
	var value string
	if json.Unmarshal(args[key], &value) != nil || value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return "", "", failf("%s must be an id (one path segment)", key)
	}
	return value, url.PathEscape(value), nil
}

// errRefused: the connection could not be made; nothing was sent.
var errRefused = errors.New("connection refused")

// request sends one REST request on the loopback client: no redirects, no
// proxy, 127.0.0.1 only. The caller closes the body.
func (l *localTools) request(ctx context.Context, method string, snap snapshot, path string, body io.Reader, header map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://%s:%d%s", loopback, snap.port, path), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+snap.key)
	for name, value := range header {
		req.Header.Set(name, value)
	}
	resp, err := l.s.up.client.Do(req)
	if err != nil {
		var de *dialError
		if errors.As(err, &de) && ctx.Err() == nil {
			// §2.4: the next transfer runs on what the item says now
			l.s.conn.refresh(ctx) //nolint:errcheck // a dead connection answers the next call itself
			return nil, errRefused
		}
		return nil, err
	}
	return resp, nil
}

// refusedStatus turns a status that is not the expected success into the
// failure the model reads. It consumes what it needs of the body.
func (l *localTools) refusedStatus(ctx context.Context, snap snapshot, resp *http.Response) error {
	switch st := resp.StatusCode; {
	case st >= 300 && st < 400:
		return failf("Anytype answered HTTP %d with a redirect, which is not followed", st)
	case st == http.StatusUnauthorized:
		// the transfer failed; the next one runs on whatever the item
		// says now
		if _, err := l.s.conn.refresh(ctx); err != nil {
			return errNotConnected
		}
		return failf("%s", msgRejected)
	}
	return &toolFailure{whole: true,
		text: restFailure(l.readBounded(ctx, resp.Body, contract.MaxErrorBodyBytes, l.s.o.ErrorBodyWait), resp.StatusCode, snap.key)}
}

// readBounded reads at most limit bytes of a short body, for at most wait
// (zero: as long as ctx lasts). A body that is longer, breaks off or
// stalls reads as nil.
func (l *localTools) readBounded(ctx context.Context, body io.Reader, limit int64, wait time.Duration) []byte {
	type read struct {
		data []byte
		err  error
	}
	done := make(chan read, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(body, limit+1))
		done <- read{data, err}
	}()
	var expired <-chan time.Time
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		expired = timer.C
	}
	select {
	case r := <-done:
		if r.err != nil || int64(len(r.data)) > limit {
			return nil
		}
		return r.data
	case <-expired:
	case <-ctx.Done():
	}
	return nil
}

// restFailure quotes heart's own error and nothing else: its two shapes
// (util.CodeToApiError and the v2 envelope) always carry the status, a
// code and a message. Only the documented members are kept, each bounded,
// on one line and with the key redacted. Any other body is reported by its
// status alone: a gateway could otherwise put a reflected Authorization
// header into a tool result.
func restFailure(body []byte, status int, key string) string {
	plain := fmt.Sprintf("Anytype answered HTTP %d", status)
	var e struct {
		Object  *string `json:"object"`
		Status  *int    `json:"status"`
		Code    *string `json:"code"`
		Message *string `json:"message"`
		Issues  []struct {
			Path    *string `json:"path"`
			Message *string `json:"message"`
		} `json:"issues"`
	}
	if json.Unmarshal(body, &e) != nil || e.Code == nil || e.Message == nil || e.Status == nil || *e.Status != status ||
		(e.Object != nil && *e.Object != "error") {
		return plain
	}
	type issue struct {
		Path    string `json:"path,omitempty"`
		Message string `json:"message"`
	}
	out := struct {
		Status  int     `json:"status"`
		Code    string  `json:"code"`
		Message string  `json:"message"`
		Issues  []issue `json:"issues,omitempty"`
	}{Status: status, Code: redact(*e.Code, key), Message: redact(*e.Message, key)}
	for _, is := range e.Issues {
		if is.Message == nil || len(out.Issues) == 20 {
			continue
		}
		one := issue{Message: clip(redact(*is.Message, key), 200)}
		if is.Path != nil {
			one.Path = clip(redact(*is.Path, key), 100)
		}
		out.Issues = append(out.Issues, one)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(out) != nil {
		return plain
	}
	return strings.TrimSpace(buf.String())
}

func clip(text string, most int) string {
	if r := []rune(text); len(r) > most {
		return string(r[:most]) + "…"
	}
	return text
}

// wholeNumber reads a JSON number that is a whole number from 0 to the
// largest 32-bit one, however it is spelled (320, 320.0, 3.2e2).
func wholeNumber(raw json.RawMessage) (int64, bool) {
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return 0, false
	}
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	if err != nil || f < 0 || f > math.MaxInt32 || f != math.Trunc(f) {
		return 0, false
	}
	return int64(f), true
}

// errCarriesKey: the download's bytes contain the connection's key.
var errCarriesKey = errors.New("the download contains the connection's key")

// keyGuard passes a download through and stops it when the key shows up
// in it: raw, or spelled as a JSON string would spell it. The key never
// rests in a file the forwarder wrote. A match across two reads is found
// in the tail kept from the read before.
type keyGuard struct {
	src      io.Reader
	patterns [][]byte
	tail     []byte
	longest  int
}

func newKeyGuard(src io.Reader, key string) io.Reader {
	if key == "" {
		return src
	}
	g := &keyGuard{src: src}
	add := func(p []byte) {
		for _, have := range g.patterns {
			if bytes.Equal(have, p) {
				return
			}
		}
		g.patterns = append(g.patterns, p)
		g.longest = max(g.longest, len(p))
	}
	add([]byte(key))
	if quoted, err := json.Marshal(key); err == nil {
		add(quoted[1 : len(quoted)-1]) // HTML characters escaped
	}
	if quoted, err := marshalPlain(key); err == nil {
		add(quoted[1 : len(quoted)-1])
		add(bytes.ReplaceAll(quoted[1:len(quoted)-1], []byte("/"), []byte(`\/`)))
	}
	return g
}

func (g *keyGuard) Read(p []byte) (int, error) {
	n, err := g.src.Read(p)
	if n > 0 {
		window := append(g.tail, p[:n]...) //nolint:gocritic // a new window per read, by design
		for _, pattern := range g.patterns {
			if bytes.Contains(window, pattern) {
				return 0, errCarriesKey
			}
		}
		keep := min(len(window), g.longest-1)
		g.tail = append(g.tail[:0:0], window[len(window)-keep:]...)
	}
	return n, err
}

func (l *localTools) download(ctx context.Context, snap snapshot, args map[string]json.RawMessage) (string, json.RawMessage, error) {
	if err := onlyArguments(toolDownload, args, "space_id", "file_id", "width"); err != nil {
		return "", nil, err
	}
	_, space, err := segment(args, "space_id")
	if err != nil {
		return "", nil, err
	}
	fileID, file, err := segment(args, "file_id")
	if err != nil {
		return "", nil, err
	}
	query := ""
	if raw, ok := args["width"]; ok {
		width, ok := wholeNumber(raw)
		if !ok {
			return "", nil, failf("width must be a whole number of pixels, or 0 for the original image")
		}
		query = fmt.Sprintf("?width=%d", width)
	}
	// the name is looked up first, so no unread download is held open
	// meanwhile
	stem, ext, err := l.objectName(ctx, snap, space, file)
	if err != nil {
		return "", nil, err
	}
	if ctx.Err() != nil {
		return "", nil, ctx.Err()
	}
	resp, err := l.request(ctx, http.MethodGet, snap, "/v2/spaces/"+space+"/files/"+file+"/content"+query, nil, nil)
	if errors.Is(err, errRefused) {
		return "", nil, failf("%s", msgStart)
	}
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return "", nil, failf("expected the whole file (HTTP 200), got HTTP %d; nothing was saved", resp.StatusCode)
		}
		return "", nil, l.refusedStatus(ctx, snap, resp)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		mediaType = "application/octet-stream"
	}
	name := fileName(dispositionName(resp.Header.Get("Content-Disposition")), stem, ext, mediaType, fileID)
	// the name ends up in the reply
	name = strings.ReplaceAll(name, snap.key, "redacted")
	saved, size, err := l.dir.Save(ctx, name, newKeyGuard(resp.Body, snap.key))
	if errors.Is(err, errCarriesKey) {
		l.s.log.printf("%s: the file contained the connection's key and was not saved", toolDownload)
		return "", nil, failf("The file contains this connection's key and was not saved.")
	}
	if err != nil {
		return "", nil, err
	}
	if !l.dir.Located() {
		// the bytes are in the folder this server opened at its start; its
		// path now names something else, so no path is given
		return "", nil, failf("the file was saved as %s, but the folder %s was moved or replaced since this server started, "+
			"so its path is not known. Ask the user where the folder is now, and to restart this app.", saved, l.dir.Path())
	}
	receipt, err := marshalPlain(struct {
		Path      string `json:"path"`
		Filename  string `json:"filename"`
		MediaType string `json:"media_type"`
		Size      int64  `json:"size"`
	}{filepath.Join(l.dir.Path(), saved), saved, redact(mediaType, snap.key), size})
	if err != nil {
		return "", nil, err
	}
	return string(receipt), receipt, nil
}

func marshalPlain(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

var fileExtRe = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`)

// objectName is the file object's name and extension: properties.name (a
// stem) and properties.file_ext of its document. A name is a convenience:
// an ordinary failure leaves it empty and the id will do. A rejected key
// or a refused connection ends the transfer here: the content request
// would only send the bearer the same way again.
func (l *localTools) objectName(ctx context.Context, snap snapshot, space, file string) (stem, ext string, err error) {
	resp, err := l.request(ctx, http.MethodGet, snap, "/v2/spaces/"+space+"/objects/"+file, nil, nil)
	if errors.Is(err, errRefused) {
		return "", "", failf("%s", msgStart)
	}
	if err != nil {
		return "", "", nil //nolint:nilerr // the name is optional
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", "", l.refusedStatus(ctx, snap, resp)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", nil
	}
	var object struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(l.readBounded(ctx, resp.Body, maxObjectBytes, l.s.o.ErrorBodyWait), &object) != nil {
		return "", "", nil
	}
	var name, fileExt string
	if json.Unmarshal(object.Properties["name"], &name) == nil && strings.TrimSpace(name) != "" {
		stem = name
	}
	if json.Unmarshal(object.Properties["file_ext"], &fileExt) == nil && fileExtRe.MatchString(fileExt) {
		ext = "." + fileExt
	}
	return stem, ext, nil
}

// dispositionName is the file name of a Content-Disposition header, the
// extended (filename*) form first.
func dispositionName(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(params["filename"])
}

// fileName is a download's file name. A Content-Disposition name is
// complete as sent. Otherwise Anytype's object name is a stem
// ("report.v2", "archive.tar") and its file_ext the extension, so the two
// are joined; without file_ext the media type's usual extension is added
// unless the stem already ends in it.
func fileName(disposition, stem, ext, mediaType, fileID string) string {
	if disposition != "" {
		return disposition
	}
	if stem == "" {
		stem = fileID
	}
	if ext != "" {
		return stem + ext
	}
	if usual := usualExtensions[mediaType]; usual != "" && !strings.HasSuffix(strings.ToLower(stem), usual) {
		return stem + usual
	}
	return stem
}

// multipartBody is a multipart/form-data body with one "file" part,
// streamed from the open file. sent turns true once the whole body has
// been handed to the HTTP client.
type multipartBody struct {
	io.Reader
	contentType string
	sent        atomic.Bool
}

func (m *multipartBody) Read(p []byte) (int, error) {
	n, err := m.Reader.Read(p)
	if errors.Is(err, io.EOF) {
		m.sent.Store(true)
	}
	return n, err
}

func uuid4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func newMultipartBody(file io.Reader, filename string) (*multipartBody, error) {
	id, err := uuid4()
	if err != nil {
		return nil, err
	}
	boundary := "anytype-mcp-" + id
	// the HTML form encoding of a file name: quote and line breaks
	// percent-encoded
	quoted := strings.NewReplacer(`"`, "%22", "\r", "%0D", "\n", "%0A").Replace(filename)
	head := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"file\"; filename=\"" + quoted + "\"\r\n" +
		"Content-Type: application/octet-stream\r\n\r\n"
	tail := "\r\n--" + boundary + "--\r\n"
	return &multipartBody{
		Reader:      io.MultiReader(strings.NewReader(head), file, strings.NewReader(tail)),
		contentType: "multipart/form-data; boundary=" + boundary,
	}, nil
}

func (l *localTools) upload(ctx context.Context, snap snapshot, args map[string]json.RawMessage) (string, json.RawMessage, error) {
	if err := onlyArguments(toolUpload, args, "space_id", "path", "dry_run"); err != nil {
		return "", nil, err
	}
	_, space, err := segment(args, "space_id")
	if err != nil {
		return "", nil, err
	}
	// absent, or one of the two literals: a null must not turn a rehearsal
	// into an upload
	dryRun := false
	if raw, ok := args["dry_run"]; ok {
		switch string(bytes.TrimSpace(raw)) {
		case "true":
			dryRun = true
		case "false":
		default:
			return "", nil, failf("dry_run must be true or false")
		}
	}
	var path string
	if json.Unmarshal(args["path"], &path) != nil {
		path = ""
	}
	file, name, err := l.dir.OpenUpload(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	form, err := newMultipartBody(file, name)
	if err != nil {
		return "", nil, err
	}
	// a fresh key per call: the upload is never replayed, so it only marks
	// the write
	idempotency, err := uuid4()
	if err != nil {
		return "", nil, err
	}
	query := ""
	if dryRun {
		query = "?dry_run=true"
	}
	// the body is handed over as a plain reader: the client can't rewind
	// it, so it can't send the upload twice
	resp, err := l.request(ctx, http.MethodPost, snap, "/v2/spaces/"+space+"/files"+query, io.NopCloser(form),
		map[string]string{"Content-Type": form.contentType, "Idempotency-Key": idempotency})
	switch {
	case errors.Is(err, errRefused):
		// a connection that never opened sent nothing
		return "", nil, failf("%s", msgStart)
	case err != nil && ctx.Err() == nil:
		// one lost after that may have stored the file
		return "", nil, failf("the upload outcome is unconfirmed: the connection to Anytype closed before it answered. %s", msgCheckSpace)
	case err != nil && form.sent.Load() && errors.Is(ctx.Err(), context.DeadlineExceeded):
		// the deadline passed after the whole file was sent, before any
		// answer: it may have been stored
		return "", nil, failf("the upload outcome is unconfirmed: Anytype did not answer within %d minutes after the whole file was sent. %s", l.minutes(), msgCheckSpace)
	case err != nil:
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, l.refusedStatus(ctx, snap, resp)
	}
	// heart's receipt: 201 with the stored file's id, or 200 with
	// dry_run: true for a dry run. Anything else (a login page from a
	// gateway, a receipt that breaks off) is no proof either way: the file
	// may have been stored, so it is unconfirmed, not a failed transfer.
	// The receipt has the rest of the transfer's time to arrive. Its
	// members are matched by their exact names.
	body := l.readBounded(ctx, resp.Body, maxReceiptBytes, 0)
	var receipt map[string]json.RawMessage
	isObject := json.Unmarshal(body, &receipt) == nil && receipt != nil
	var id string
	confirmed := isObject && resp.StatusCode == http.StatusCreated && json.Unmarshal(receipt["id"], &id) == nil && id != ""
	if dryRun {
		confirmed = isObject && resp.StatusCode == http.StatusOK && string(bytes.TrimSpace(receipt["dry_run"])) == "true"
	}
	if !confirmed {
		return "", nil, failf("the upload outcome is unconfirmed: Anytype answered HTTP %d without its upload receipt. %s", resp.StatusCode, msgCheckSpace)
	}
	if hit, walked := carriesKey(body, snap.key); hit || !walked {
		l.s.log.printf("%s: the receipt contained the connection's key and was withheld", toolUpload)
		return "", nil, failf("%s", msgWithheld)
	}
	body = bytes.TrimSpace(body)
	return string(body), body, nil
}
