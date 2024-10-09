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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/avast/retry-go/v4"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/netutil"
	"github.com/anyproto/anytype-heart/util/svg"
)

const (
	CName = "gateway"

	defaultPort    = 47800
	getFileTimeout = 1 * time.Minute
	requestLimit   = 32
)

var log = logging.Logger("anytype-gateway")

func New() Gateway {
	return new(gateway)
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
	server            *http.Server
	listener          net.Listener
	handler           *http.ServeMux
	addr              string
	mu                sync.Mutex
	isServerStarted   bool
	limitCh           chan struct{}
}

func GatewayAddr() string {
	if addr := os.Getenv("ANYTYPE_GATEWAY_ADDR"); addr != "" {
		return addr
	}

	port, err := netutil.GetRandomPort()
	if err != nil {
		log.Errorf("failed to get random port for gateway, go with the default %d", defaultPort)
		port = defaultPort
	}

	return fmt.Sprintf("127.0.0.1:%d", port)
}

func (g *gateway) Init(a *app.App) (err error) {
	g.fileService = app.MustComponent[files.Service](a)
	g.fileObjectService = app.MustComponent[fileobject.Service](a)
	g.addr = GatewayAddr()
	log.Debugf("gateway.Init: %s", g.addr)
	return nil
}

func (g *gateway) Name() string {
	return CName
}

func (g *gateway) Run(context.Context) error {
	if g.isServerStarted {
		return fmt.Errorf("gateway already started")
	}

	log.Infof("gateway.Run: %s", g.addr)
	g.handler = http.NewServeMux()
	g.handler.HandleFunc("/file/", g.fileHandler)
	g.handler.HandleFunc("/image/", g.imageHandler)
	g.limitCh = make(chan struct{}, requestLimit)

	// check port first
	listener, err := net.Listen("tcp", g.addr)
	if err != nil {
		// todo: choose next available port
		return err
	}

	err = listener.Close()
	if err != nil {
		return err
	}

	g.startServer()

	return nil
}

// Close stops the gateway
func (g *gateway) Close(ctx context.Context) (err error) {
	err = g.stopServer()
	return err
}

// Addr returns the gateway's address
func (g *gateway) Addr() string {
	return g.addr
}

func (g *gateway) StateChange(state int) {
	switch pb.RpcAppSetDeviceStateRequestDeviceState(state) {
	case pb.RpcAppSetDeviceStateRequest_FOREGROUND:
		g.startServer()
	case pb.RpcAppSetDeviceStateRequest_BACKGROUND:
		if err := g.stopServer(); err != nil {
			log.Errorf("err gateway close: %+v", err)
		}
	}
}

func (g *gateway) startServer() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.isServerStarted {
		log.Errorf("server already started")
		return
	}

	ln, err := net.Listen("tcp", g.addr)
	if err != nil {
		log.Errorf("listen addr err: %s", err)
		return
	}

	g.listener = ln

	g.server = &http.Server{
		Addr:    g.addr,
		Handler: g.handler,
	}

	go func(srv *http.Server, l net.Listener) {
		err := srv.Serve(l)
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("gateway error: %s", err)
			return
		}
		log.Info("gateway was shutdown")
	}(g.server, ln)

	g.isServerStarted = true

	log.Infof("gateway listening at %s", g.server.Addr)
}

func (g *gateway) stopServer() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.isServerStarted {
		g.isServerStarted = false
		// don't wait for the server shutdown because we don't care for the requests to interrupt
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(0))
		defer cancel()
		if err := g.server.Shutdown(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			log.Errorf("gateway stop error: %s", err)
		}
		if err := g.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
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
	file, reader, err := g.getFile(ctx, r)
	if err != nil {
		log.With("path", cleanUpPathForLogging(r.URL.Path)).Errorf("error getting file: %s", err)
		http.Error(w, err.Error(), 500)
		return
	}
	meta := file.Meta()
	w.Header().Set("Content-Type", meta.Media)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", meta.Name))

	// todo: inside textile it still requires the file to be fully downloaded and decrypted(consuming 2xSize in ram) to provide the ReadSeeker interface
	// 	need to find a way to use ReadSeeker all the way from downloading files from IPFS to writing the decrypted chunk to the HTTP
	http.ServeContent(w, r, meta.Name, meta.Added, reader)
}

func (g *gateway) getFile(ctx context.Context, r *http.Request) (files.File, io.ReadSeeker, error) {
	fileIdAndPath := strings.TrimPrefix(r.URL.Path, "/file/")
	parts := strings.Split(fileIdAndPath, "/")
	fileId := parts[0]

	var id domain.FullFileId
	// Treat id as fileId to allow download file directly
	if domain.IsFileId(fileId) {
		id = domain.FullFileId{
			FileId: domain.FileId(fileId),
		}
	} else {
		var err error
		id, err = g.fileObjectService.GetFileIdFromObjectWaitLoad(ctx, fileId)
		if err != nil {
			return nil, nil, fmt.Errorf("get file hash from object id: %w", err)
		}
	}

	file, err := g.fileService.FileByHash(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get file by hash: %w", err)
	}

	reader, err := file.Reader(ctx)
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

	file, reader, err := g.getImage(ctx, r)
	if err != nil {
		log.With("path", cleanUpPathForLogging(r.URL.Path)).Errorf("error getting image: %s", err)
		http.Error(w, err.Error(), 500)
		return
	}

	meta := file.Meta()
	w.Header().Set("Content-Type", meta.Media)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", meta.Name))

	// todo: inside textile it still requires the file to be fully downloaded and decrypted(consuming 2xSize in ram) to provide the ReadSeeker interface
	// 	need to find a way to use ReadSeeker all the way from downloading files from IPFS to writing the decrypted chunk to the HTTP
	http.ServeContent(w, r, meta.Name, meta.Added, reader)
}

func (g *gateway) getImage(ctx context.Context, r *http.Request) (files.File, io.ReadSeeker, error) {
	urlParts := strings.Split(r.URL.Path, "/")
	imageId := urlParts[2]

	var id domain.FullFileId
	// Treat id as fileId. We need to handle raw fileIds for backward compatibility in case of spaceview. See editor.SpaceView for details.
	if domain.IsFileId(imageId) {
		id = domain.FullFileId{
			FileId: domain.FileId(imageId),
		}
	} else {
		var err error
		id, err = g.fileObjectService.GetFileIdFromObjectWaitLoad(ctx, imageId)
		if err != nil {
			return nil, nil, fmt.Errorf("get file hash from object id: %w", err)
		}
	}

	retryOptions := []retry.Option{
		retry.Context(ctx),
		retry.Attempts(0),
		retry.Delay(200 * time.Millisecond),
		retry.MaxDelay(2 * time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	}

	result, err := retry.DoWithData(func() (*getImageReaderResult, error) {
		res, err := g.getImageReader(ctx, id, r)
		if err != nil {
			return nil, err
		}
		return res, nil
	}, retryOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("get image reader: %w", err)
	}

	retryReader := newRetryReadSeeker(result.reader, retryOptions...)
	return result.file, retryReader, nil
}

type getImageReaderResult struct {
	file   files.File
	reader io.ReadSeeker
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

func (g *gateway) getImageReader(ctx context.Context, id domain.FullFileId, req *http.Request) (*getImageReaderResult, error) {
	image, err := g.fileService.ImageByHash(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get image by hash: %w", err)
	}
	var file files.File
	query := req.URL.Query()
	wantWidthStr := query.Get("width")
	if wantWidthStr == "" {
		file, err = image.GetOriginalFile()
		if err != nil {
			return nil, fmt.Errorf("get image file: %w", err)
		}
		if filepath.Ext(file.Info().Name) == constant.SvgExt {
			return g.handleSVGFile(ctx, file)
		}
	} else {
		wantWidth, err := strconv.Atoi(wantWidthStr)
		if err != nil {
			return nil, fmt.Errorf("parse width: %w", err)
		}
		file, err = image.GetFileForWidth(wantWidth)
		if err != nil {
			return nil, fmt.Errorf("get image file: %w", err)
		}
		if filepath.Ext(file.Info().Name) == constant.SvgExt {
			return g.handleSVGFile(ctx, file)
		}
	}
	reader, err := file.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("get image reader: %w", err)
	}
	return &getImageReaderResult{file: file, reader: reader}, nil
}

func (g *gateway) handleSVGFile(ctx context.Context, file files.File) (*getImageReaderResult, error) {
	reader, err := svg.ProcessSvg(ctx, file)
	if err != nil {
		return nil, err
	}
	return &getImageReaderResult{file, reader}, nil
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
