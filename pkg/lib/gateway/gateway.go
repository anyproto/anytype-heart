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

	gatewayHost    = "127.0.0.1"
	defaultPort    = 47800
	getFileTimeout = 1 * time.Minute
	requestLimit   = 32
)

// errGatewayClosed is returned when something tries to start the gateway after the component was
// closed - on mobile a foreground state change can still arrive while the app is shutting down.
var errGatewayClosed = errors.New("gateway is closed")

var (
	log      = logging.Logger("anytype-gateway")
	isMobile = runtime.GOOS == "ios" || runtime.GOOS == "android"
)

func New() Gateway {
	return &gateway{defaultPort: defaultPort}
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
	addrStore         addrStore
	handler           *http.ServeMux
	limitCh           chan struct{}
	defaultPort       int

	// lifecycleMu serializes whole start and stop operations against each other. mu alone is not
	// enough: both have to drop it midway to shut the HTTP server down without holding a lock the
	// serving goroutine needs, and a start landing in that window would adopt a dying listener.
	lifecycleMu sync.Mutex

	mu              sync.Mutex
	server          *http.Server
	listener        *net.TCPListener
	addr            string
	isServerStarted bool
	closed          bool
}

// addrStore remembers the address the gateway bound, so the next run can ask for the same port.
// The config component implements it; the gateway declares its own interface rather than importing
// config, which sits a layer above pkg/lib.
type addrStore interface {
	GatewayAddr() string
	SetGatewayAddr(addr string) error
}

func (g *gateway) Init(a *app.App) (err error) {
	g.fileService = app.MustComponent[files.Service](a)
	g.fileObjectService = app.MustComponent[fileobject.Service](a)
	g.fileDownloader = app.MustComponent[filedownloader.Service](a)
	g.addrStore = app.MustComponent[addrStore](a)

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

// bindLocked binds the listener and remembers the address it got. Must be called with g.mu held.
func (g *gateway) bindLocked() error {
	override := os.Getenv("ANYTYPE_GATEWAY_ADDR")
	previous := g.addr
	ln, err := listenGateway(listenConfig{
		override:   override,
		candidates: g.candidatePortsLocked(),
	})
	if err != nil {
		return err
	}

	tcpLn, ok := ln.(*net.TCPListener)
	if !ok {
		_ = ln.Close()
		return fmt.Errorf("unexpected listener type %T", ln)
	}

	g.listener = tcpLn
	g.addr = tcpLn.Addr().String()
	log.Infof("gateway bound to %s", g.addr)

	if previous != "" && previous != g.addr {
		// clients cache the gateway URL from AccountInfo and no event can hand them a new one, so
		// they keep asking the old port until the next account select
		log.Warnf("gateway moved from %s to %s, cached client URLs are now stale", previous, g.addr)
	}

	if override == "" {
		// best effort: the gateway works either way, the next run just may not get the same port
		if err := g.addrStore.SetGatewayAddr(g.addr); err != nil {
			log.Errorf("gateway: persist address %s: %v", g.addr, err)
		}
	}

	return nil
}

// candidatePortsLocked ranks the ports to try. Must be called with g.mu held.
func (g *gateway) candidatePortsLocked() []int {
	if g.addr != "" {
		// rebinding mid-session: the port clients were already told about comes first
		return []int{portFromAddr(g.addr), g.defaultPort}
	}
	// a fresh start prefers the well-known port, and only then the one the previous run persisted.
	// The other way round, a single busy start would strand us on an OS-assigned port for good -
	// and that port comes from the range the kernel hands out for outbound sockets, which is the
	// worst place to keep asking for a fixed one.
	return []int{g.defaultPort, portFromAddr(g.addrStore.GatewayAddr())}
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
		if err := g.startServer(); err != nil {
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

// startServer binds if needed and serves. It reports why it could not start instead of leaving a
// gateway that answers nothing for the rest of the session.
func (g *gateway) startServer() error {
	g.lifecycleMu.Lock()
	defer g.lifecycleMu.Unlock()

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		return errGatewayClosed
	}
	if g.isServerStarted {
		return errors.New("gateway already started")
	}

	if g.listener == nil {
		if err := g.bindLocked(); err != nil {
			return fmt.Errorf("bind gateway listener: %w", err)
		}
	}

	g.server = &http.Server{
		Addr:    g.addr,
		Handler: g.handler,
	}

	go func(srv *http.Server, ln *net.TCPListener) {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("gateway: serve on %s: %v", ln.Addr(), err)
			g.dropListener(ln)
			return
		}
		log.Info("gateway was shutdown")
	}(g.server, g.listener)

	g.isServerStarted = true

	log.Infof("gateway listening at %s", g.addr)
	return nil
}

// dropListener gives up a listener that stopped working, so that the next start binds a fresh one
// instead of serving nothing for the rest of the session.
func (g *gateway) dropListener(ln *net.TCPListener) {
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
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
}

func (g *gateway) readLimitCh() {
	<-g.limitCh
}

// fileHandler gets file meta from the DB, gets the corresponding data from the IPFS and decrypts it
func (g *gateway) fileHandler(w http.ResponseWriter, r *http.Request) {
	select {
	case g.limitCh <- struct{}{}:
		defer g.readLimitCh()
	case <-r.Context().Done():
		// exit fast in case context is already done(e.g. server stopped or client canceled)
		return
	}
	enableCors(w)

	ctx, cancel := context.WithTimeout(r.Context(), getFileTimeout)
	defer cancel()
	file, reader, err := g.getFile(rpcstore.ContextWithWaitAvailable(ctx), r)
	if err != nil {
		log.With("path", cleanUpPathForLogging(r.URL.Path)).Errorf("error getting file: %s", err)
		http.Error(w, err.Error(), 500)
		return
	}
	meta := file.Meta()
	w.Header().Set("Content-Type", meta.Media)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", meta.Name))
	w.Header().Set("Cache-Control", "max-age=31536000")

	// Note: the DagReader is lazy and streams ~1MB blocks on demand. The CFBDecryptor.Seek
	// fast-path avoids expensive IPFS block preloading during size determination (SeekEnd).
	http.ServeContent(w, r, meta.Name, meta.Added, reader)
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
	select {
	case g.limitCh <- struct{}{}:
		defer g.readLimitCh()
	case <-r.Context().Done():
		// exit fast in case context is already done(e.g. server stopped or client canceled)
		return
	}
	enableCors(w)

	ctx, cancel := context.WithTimeout(r.Context(), getFileTimeout)
	defer cancel()

	res, err := g.getImage(rpcstore.ContextWithWaitAvailable(ctx), r)
	if err != nil {
		log.With("path", cleanUpPathForLogging(r.URL.Path)).Errorf("error getting image: %s", err)
		http.Error(w, err.Error(), 500)
		return
	}

	meta := res.file.Meta()
	w.Header().Set("Content-Type", res.mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", meta.Name))
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
