package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/avast/retry-go/v4"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/filedownloader"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/svg"
)

const (
	CName = "gateway"

	gatewayHost   = "127.0.0.1"
	wellKnownPort = 47800
	// fileStallTimeout cuts loose a file transfer that has stopped making
	// progress. It is deliberately not a total deadline: a large file, or one
	// still being fetched from a node, legitimately runs longer than any fixed
	// budget, and a total deadline killed those mid-stream. The clock restarts
	// on every byte, so only a window with nothing moving ends the request.
	fileStallTimeout = 1 * time.Minute
	// getImageTimeout stays a total deadline. Image responses are bounded and
	// serve a render that has to either appear or give up quickly.
	getImageTimeout = 1 * time.Minute
	requestLimit    = 32
)

// errGatewayClosed is returned when something tries to start the gateway after the component was
// closed - on mobile a foreground state change can still arrive while the app is shutting down.
var (
	errGatewayClosed = errors.New("gateway is closed")
	// errGatewayAlreadyStarted is expected: mobile reports the same foreground state more than once
	errGatewayAlreadyStarted = errors.New("gateway already started")
)

var (
	log      = logging.Logger("anytype-gateway")
	isMobile = runtime.GOOS == "ios" || runtime.GOOS == "android"
)

func New() Gateway {
	return &gateway{wellKnownPort: wellKnownPort}
}

// Gateway is a HTTP API for getting files and links from IPFS
type Gateway interface {
	Addr() string
	app.ComponentRunnable
	app.ComponentStatable
}

type gateway struct {
	fileService       files.Service
	fileObjectService fileobject.Service
	fileDownloader    filedownloader.Service
	addrStore         AddrStore
	handler           *http.ServeMux
	limitCh           chan struct{}
	fileStallTimeout  time.Duration
	wellKnownPort     int

	// lifecycleMu serializes whole start and stop operations against each other. mu alone is not
	// enough: a stop has to drop it midway, to shut the HTTP server down without holding a lock the
	// serving goroutine needs, and a start landing in that window would adopt a dying listener.
	lifecycleMu sync.Mutex

	mu              sync.Mutex
	server          *http.Server
	listener        net.Listener
	addr            string
	isServerStarted bool
	closed          bool
}

// AddrStore remembers the address the gateway bound, so the next run can ask for the same port.
// The config component implements it; the gateway declares its own interface rather than importing
// config, which sits a layer above pkg/lib.
type AddrStore interface {
	GatewayAddr() string
	SetGatewayAddr(addr string) error
}

func (g *gateway) Init(a *app.App) (err error) {
	g.fileService = app.MustComponent[files.Service](a)
	g.fileObjectService = app.MustComponent[fileobject.Service](a)
	g.fileDownloader = app.MustComponent[filedownloader.Service](a)
	g.addrStore = app.MustComponent[AddrStore](a)
	g.fileStallTimeout = fileStallTimeout

	g.handler = http.NewServeMux()
	g.handler.HandleFunc("/file/", g.fileHandler)
	g.handler.HandleFunc("/image/", g.imageHandler)
	g.limitCh = make(chan struct{}, requestLimit)

	return nil
}

func (g *gateway) Name() string {
	return CName
}

func (g *gateway) Run(context.Context) error {
	return g.startServer()
}

// bindLocked binds the listener. It returns the address worth remembering for the next run, or ""
// when there is nothing to remember. Persisting is left to the caller: it writes and fsyncs
// config.json, which must not happen under the lock Addr() needs. Must be called with g.mu held.
func (g *gateway) bindLocked() (remember string, err error) {
	override := os.Getenv("ANYTYPE_GATEWAY_ADDR")
	previous := g.addr
	ln, err := listenGateway(listenConfig{
		override:   override,
		candidates: g.candidatePortsLocked(),
	})
	if err != nil {
		return "", fmt.Errorf("listen gateway: %w", err)
	}

	g.listener = ln
	g.addr = ln.Addr().String()
	log.Infof("gateway bound to %s", g.addr)

	if previous != "" && previous != g.addr {
		// clients cache the gateway URL from AccountInfo and no event carries a new one, so they
		// keep asking the old port until they next open a space or select the account
		log.Warnf("gateway moved from %s to %s, cached client URLs are now stale", previous, g.addr)
	}

	if override != "" {
		// a dev-only address must not become the next run's preference, and it is the only one that
		// can be non-loopback
		return "", nil
	}
	if portFromAddr(g.addr) == g.wellKnownPort {
		// what is worth remembering is the port that worked when the well-known one did not.
		// Storing the well-known port instead would erase that, and the next start that finds it
		// busy would have to take a fresh random port rather than the one it used last time.
		return "", nil
	}
	return g.addr, nil
}

// candidatePortsLocked ranks the ports to try. Must be called with g.mu held.
func (g *gateway) candidatePortsLocked() []int {
	ranked := []int{g.wellKnownPort, portFromAddr(g.addrStore.GatewayAddr())}
	if g.addr != "" {
		// rebinding mid-session: the port clients were already told about comes first, because
		// moving would strand every URL they have cached
		ranked = []int{portFromAddr(g.addr), g.wellKnownPort}
	}
	// a fresh start prefers the well-known port over the persisted one. The other way round, a
	// single busy start would strand us on an OS-assigned port for good - and that port comes from
	// the range the kernel hands out for outbound sockets, the worst place to keep a fixed one.

	candidates := make([]int, 0, len(ranked))
	for _, port := range ranked {
		if port <= 0 || slices.Contains(candidates, port) {
			continue
		}
		candidates = append(candidates, port)
	}
	return candidates
}

// Close stops the gateway for good: nothing may start it again, so a foreground state change
// arriving during shutdown cannot resurrect it on a socket nobody will ever close.
func (g *gateway) Close(ctx context.Context) error {
	g.lifecycleMu.Lock()
	defer g.lifecycleMu.Unlock()

	err := g.stopServingLocked()

	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()

	return err
}

// Addr returns the gateway's address
func (g *gateway) Addr() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.addr
}

func (g *gateway) StateChange(state int) {
	// Desktop: gateway runs continuously, mobile: start/stop on foreground/background
	if !isMobile {
		if domain.CompState(state) == domain.CompStateAppClosingInitiated {
			// Stop pending file requests for faster shutdown
			if err := g.stopServer(); err != nil {
				log.Errorf("err gateway close: %+v", err)
			}
		}
		return
	}

	switch domain.CompState(state) {
	case domain.CompStateAppWentForeground:
		if err := g.startServer(); err != nil && !errors.Is(err, errGatewayAlreadyStarted) {
			log.Errorf("err gateway start: %+v", err)
		}
	case domain.CompStateAppWentBackground:
		if err := g.stopServer(); err != nil {
			log.Errorf("err gateway close: %+v", err)
		}
	case domain.CompStateAppClosingInitiated:
		// Stop pending file requests for faster shutdown
		if err := g.stopServer(); err != nil {
			log.Errorf("err gateway close: %+v", err)
		}
	}
}

// startServer binds and serves. It reports why it could not start instead of leaving a gateway that
// answers nothing for the rest of the session.
func (g *gateway) startServer() error {
	g.lifecycleMu.Lock()
	defer g.lifecycleMu.Unlock()

	remember, addr, err := g.startServingLocked()
	if err != nil {
		return err
	}

	if remember != "" {
		// best effort, and deliberately outside g.mu: this fsyncs config.json, and Addr() is on the
		// account-info path. The gateway works either way; the next run just may not get the same
		// port.
		if err := g.addrStore.SetGatewayAddr(remember); err != nil {
			log.Errorf("gateway: persist address %s: %v", remember, err)
		}
	}

	log.Infof("gateway listening at %s", addr)
	return nil
}

// startServingLocked binds and starts serving. It returns the address worth remembering for the
// next run (or "") and the address it is now serving on, leaving the caller to persist outside
// g.mu. Must be called with g.lifecycleMu held.
func (g *gateway) startServingLocked() (remember string, addr string, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		return "", "", errGatewayClosed
	}
	if g.isServerStarted {
		return "", "", errGatewayAlreadyStarted
	}

	// a stop always gives the listener up, so there is never one to reuse here
	remember, err = g.bindLocked()
	if err != nil {
		return "", "", fmt.Errorf("bind gateway listener: %w", err)
	}

	g.server = &http.Server{Handler: g.handler}
	g.isServerStarted = true
	srv, ln := g.server, g.listener

	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("gateway: serve on %s: %v", ln.Addr(), err)
			// Serve giving up early leaves this server's connections open and nobody else will ever
			// shut it down: the next start replaces g.server, and every stop skips it because
			// dropListener has already cleared isServerStarted
			_ = srv.Close()
			g.dropListener(ln)
			return
		}
		log.Info("gateway was shutdown")
	}()

	return remember, g.addr, nil
}

// dropListener gives up a listener that stopped working, so that the next start binds a fresh one
// instead of serving nothing for the rest of the session.
func (g *gateway) dropListener(ln net.Listener) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.listener != ln {
		return
	}
	_ = g.listener.Close()
	g.listener = nil
	g.isServerStarted = false
}

// stopServer stops serving and gives the port back. Apple's TN2277 is explicit that a listening
// socket must not be held across suspension - the system may reclaim its resources without telling
// the app, and a socket left bound makes the kernel accept connections the suspended app will never
// answer, so clients hang instead of failing fast. The next start asks for the same port again.
func (g *gateway) stopServer() error {
	g.lifecycleMu.Lock()
	defer g.lifecycleMu.Unlock()

	return g.stopServingLocked()
}

// stopServingLocked must be called with g.lifecycleMu held.
func (g *gateway) stopServingLocked() error {
	g.mu.Lock()
	if !g.isServerStarted {
		g.mu.Unlock()
		return nil
	}
	g.isServerStarted = false
	srv, ln := g.server, g.listener
	g.listener = nil
	g.mu.Unlock()

	// don't wait for the server shutdown because we don't care for the requests to interrupt
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(0))
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		log.Errorf("gateway stop error: %s", err)
	}

	// Shutdown closes the listener it serves on, so this is usually already done
	if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close gateway listener: %w", err)
	}
	return nil
}

func enableCors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	// Without this a cross-origin reader sees none of these: only a short
	// safelist is readable by default, and Content-Disposition is not on it. A
	// client fetching a download would get no filename and, worse, no
	// Accept-Ranges or Content-Range to resume with.
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Length, Content-Range, Accept-Ranges")
}

// allowReadMethod answers a CORS preflight and refuses anything that is not a
// read, before a limiter slot is taken.
//
// The mux routes every method to the same handler and ServeContent omits the
// body only for HEAD, so without this an OPTIONS preflight — or a stray POST —
// streams the entire object. On the multi-gigabyte path that is a
// multi-gigabyte preflight, holding one of the shared slots throughout.
func allowReadMethod(w http.ResponseWriter, r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return true
	case http.MethodOptions:
		// A preflight that does not allow Range is no use to a download.
		w.Header().Set("Access-Control-Allow-Headers", "Range, If-Range, If-None-Match, If-Modified-Since")
		w.Header().Set("Access-Control-Max-Age", "600")
		w.WriteHeader(http.StatusNoContent)
		return false
	default:
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
}

// transientStatusWriter rewrites the status ServeContent chose when the request
// was canceled underneath it.
//
// ServeContent maps any seek failure while serving a range to 416, and a
// failure while sizing to 500. Under the stall timeout both are reachable
// transiently: seeking into an encrypted file fetches the previous block to
// recover the IV, which is real network I/O, and the window can expire during
// it. A resuming client told 416 concludes its partial file is invalid and
// discards it — so a stall is reported as a timeout, which is what it is.
//
// Wrapping costs the ReadFrom fast path, which for a non-file source is a
// buffered copy either way — the same 32KB reads the stall window already
// counts progress in.
type transientStatusWriter struct {
	http.ResponseWriter
	ctx context.Context
}

func (w *transientStatusWriter) WriteHeader(status int) {
	if w.ctx.Err() != nil &&
		(status == http.StatusRequestedRangeNotSatisfiable || status == http.StatusInternalServerError) {
		status = http.StatusGatewayTimeout
	}
	w.ResponseWriter.WriteHeader(status)
}

func (g *gateway) readLimitCh() {
	<-g.limitCh
}

// fileHandler gets file meta from the DB, gets the corresponding data from the IPFS and decrypts it
func (g *gateway) fileHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(w)
	if !allowReadMethod(w, r) {
		return
	}

	select {
	case g.limitCh <- struct{}{}:
		defer g.readLimitCh()
	case <-r.Context().Done():
		// exit fast in case context is already done(e.g. server stopped or client canceled)
		return
	}

	// The stall window covers the lookup too: nothing has moved yet while the
	// file is being located, so an unreachable file still gives up after one
	// window, exactly as the old total deadline did.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stall := newStallTimeout(g.fileStallTimeout, cancel)
	defer stall.stop()

	file, reader, err := g.getFile(rpcstore.ContextWithWaitAvailable(ctx), r)
	if err != nil {
		log.With("path", cleanUpPathForLogging(r.URL.Path)).Errorf("error getting file: %s", err)
		http.Error(w, err.Error(), 500)
		return
	}
	meta := file.Meta()
	disposition := "inline"
	if attachmentRequested(r) {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", meta.Media)
	w.Header().Set("Content-Disposition", contentDisposition(disposition, meta.Name))
	w.Header().Set("Cache-Control", "max-age=31536000")

	// Note: the DagReader is lazy and streams ~1MB blocks on demand. The CFBDecryptor.Seek
	// fast-path avoids expensive IPFS block preloading during size determination (SeekEnd).
	http.ServeContent(&transientStatusWriter{ResponseWriter: w, ctx: ctx}, r, meta.Name, meta.Added,
		&progressReader{ReadSeeker: reader, onProgress: stall.progress})
}

func (g *gateway) getFile(ctx context.Context, r *http.Request) (files.File, io.ReadSeeker, error) {
	fileIdAndPath := strings.TrimPrefix(r.URL.Path, "/file/")
	parts := strings.Split(fileIdAndPath, "/")
	objectId := parts[0]

	var file files.File
	var reader io.ReadSeeker
	file, err := g.fileObjectService.GetFileData(ctx, objectId)
	if err != nil {
		return nil, nil, fmt.Errorf("get file data: %w", err)
	}
	reader, err = file.Reader(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get reader: %w", err)
	}

	return file, reader, err
}

// imageHandler gets image meta from the DB, gets the corresponding data from the IPFS and decrypts it
func (g *gateway) imageHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(w)
	if !allowReadMethod(w, r) {
		return
	}

	select {
	case g.limitCh <- struct{}{}:
		defer g.readLimitCh()
	case <-r.Context().Done():
		// exit fast in case context is already done(e.g. server stopped or client canceled)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), getImageTimeout)
	defer cancel()

	res, err := g.getImage(rpcstore.ContextWithWaitAvailable(ctx), r)
	if err != nil {
		log.With("path", cleanUpPathForLogging(r.URL.Path)).Errorf("error getting image: %s", err)
		http.Error(w, err.Error(), 500)
		return
	}

	meta := res.file.Meta()
	// Images are always inline: this endpoint backs rendering, and the save
	// flow fetches the original through /file/ instead.
	w.Header().Set("Content-Type", res.mimeType)
	w.Header().Set("Content-Disposition", contentDisposition("inline", meta.Name))
	w.Header().Set("Cache-Control", "max-age=31536000")

	// todo: inside textile it still requires the file to be fully downloaded and decrypted(consuming 2xSize in ram) to provide the ReadSeeker interface
	// 	need to find a way to use ReadSeeker all the way from downloading files from IPFS to writing the decrypted chunk to the HTTP
	http.ServeContent(w, r, meta.Name, meta.Added, res.reader)
}

func (g *gateway) getImage(ctx context.Context, r *http.Request) (*getImageReaderResult, error) {
	urlParts := strings.Split(r.URL.Path, "/")
	imageId := urlParts[2]

	retryOptions := []retry.Option{
		retry.Context(ctx),
		retry.Attempts(0),
		retry.Delay(200 * time.Millisecond),
		retry.MaxDelay(2 * time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	}

	result, err := retry.DoWithData(func() (*getImageReaderResult, error) {
		var img files.Image
		var err error
		if domain.IsFileId(imageId) {
			img, err = g.fileObjectService.GetImageDataFromRawId(ctx, domain.FileId(imageId))
			if err != nil {
				return nil, fmt.Errorf("get image data: %w", err)
			}
		} else {
			img, err = g.fileObjectService.GetImageData(ctx, imageId)
			if err != nil {
				return nil, fmt.Errorf("get image data: %w", err)
			}
		}
		res, err := g.getImageReader(ctx, img, r)
		if err != nil {
			return nil, fmt.Errorf("get image reader: %w", err)
		}
		res.spaceId = img.SpaceId()
		return res, nil
	}, retryOptions...)
	if err != nil {
		return nil, fmt.Errorf("get image reader: %w", err)
	}

	retryReader := newRetryReadSeeker(result.reader, retryOptions...)
	return &getImageReaderResult{
		file:     result.file,
		reader:   retryReader,
		mimeType: result.mimeType,
	}, nil
}

type getImageReaderResult struct {
	file         files.File
	reader       io.ReadSeeker
	mimeType     string
	originalFile files.File
	spaceId      string
}

type retryReadSeeker struct {
	reader  io.ReadSeeker
	options []retry.Option
}

func newRetryReadSeeker(reader io.ReadSeeker, options ...retry.Option) *retryReadSeeker {
	// EOF has special meaning, do not retry on it
	options = append(options, retry.RetryIf(func(err error) bool {
		return !errors.Is(err, io.EOF)
	}))
	return &retryReadSeeker{
		reader:  reader,
		options: options,
	}
}

var _ io.ReadSeeker = (*retryReadSeeker)(nil)

func (r *retryReadSeeker) Read(p []byte) (int, error) {
	return retry.DoWithData(func() (int, error) {
		return r.reader.Read(p)
	}, r.options...)
}

func (r *retryReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return retry.DoWithData(func() (int64, error) {
		return r.reader.Seek(offset, whence)
	}, r.options...)
}

func (g *gateway) getImageReader(ctx context.Context, image files.Image, req *http.Request) (*getImageReaderResult, error) {
	var file files.File
	query := req.URL.Query()
	wantWidthStr := query.Get("width")

	orig, err := image.GetOriginalFile()
	if err != nil {
		return nil, fmt.Errorf("get original file: %w", err)
	}

	if filepath.Ext(orig.Name()) == constant.SvgExt {
		return g.handleSVGFile(ctx, orig)
	}

	if wantWidthStr == "" {
		file = orig
	} else {
		wantWidth, err := strconv.Atoi(wantWidthStr)
		if err != nil {
			return nil, fmt.Errorf("parse width: %w", err)
		}
		file, err = image.GetFileForWidth(wantWidth)
		if err != nil {
			return nil, fmt.Errorf("get image file: %w", err)
		}
	}

	reader, err := file.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("get image reader: %w", err)
	}
	return &getImageReaderResult{
		file:         file,
		reader:       reader,
		mimeType:     file.MimeType(),
		originalFile: orig,
	}, nil
}

func (g *gateway) handleSVGFile(ctx context.Context, file files.File) (*getImageReaderResult, error) {
	reader, mimeType, err := svg.ProcessSvg(ctx, file)
	if err != nil {
		return nil, err
	}
	return &getImageReaderResult{
		file:         file,
		reader:       reader,
		mimeType:     mimeType,
		originalFile: file,
	}, nil
}

func cleanUpPathForLogging(input string) string {
	parts := strings.SplitN(strings.TrimPrefix(input, "/"), "/", 2)
	if len(parts) < 2 {
		return input
	}

	// Don't mask CIDs
	_, err := cid.Parse(parts[1])
	if err == nil {
		return input
	}

	parts[1] = "<masked invalid path>"
	return "/" + strings.Join(parts, "/")
}
