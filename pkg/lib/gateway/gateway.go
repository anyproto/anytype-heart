package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/netutil"
)

const CName = "gateway"

const defaultPort = 47800

const getFileTimeout = 1 * time.Minute

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
	fileService     files.Service
	server          *http.Server
	listener        net.Listener
	handler         *http.ServeMux
	addr            string
	mu              sync.Mutex
	isServerStarted bool
}

func getRandomPort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}

	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		if err := g.server.Shutdown(ctx); err != nil {
			return err
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

// fileHandler gets file meta from the DB, gets the corresponding data from the IPFS and decrypts it
func (g *gateway) fileHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(w)
	ctx, cancel := context.WithTimeout(context.Background(), getFileTimeout)
	defer cancel()
	file, reader, err := g.getFile(ctx, r)
	if err != nil {
		log.With("path", r.URL.Path).Errorf("error getting file: %s", err)
		if strings.Contains(err.Error(), "file not found") {
			http.NotFound(w, r)
			return
		}
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
	fileHashAndPath := strings.TrimPrefix(r.URL.Path, "/file/")
	parts := strings.Split(fileHashAndPath, "/")
	fileHash := parts[0]

	file, err := g.fileService.FileByHash(ctx, fileHash)
	if err != nil {
		return nil, nil, fmt.Errorf("get file by hash: %s", err)
	}

	reader, err := file.Reader(ctx)
	return file, reader, err
}

// fileHandler gets file meta from the DB, gets the corresponding data from the IPFS and decrypts it
func (g *gateway) imageHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(w)

	ctx, cancel := context.WithTimeout(context.Background(), getFileTimeout)
	defer cancel()

	file, reader, err := g.getImage(ctx, r)
	if err != nil {
		log.With("path", r.URL.Path).Errorf("error getting image: %s", err)
		if strings.Contains(err.Error(), "file not found") {
			http.NotFound(w, r)
			return
		}
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
	imageHash := urlParts[2]
	query := r.URL.Query()

	image, err := g.fileService.ImageByHash(ctx, imageHash)
	if err != nil {
		return nil, nil, fmt.Errorf("get image by hash: %w", err)
	}
	var file files.File
	wantWidthStr := query.Get("width")
	if wantWidthStr == "" {
		file, err = image.GetOriginalFile(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("get image file: %w", err)
		}
	} else {
		wantWidth, err := strconv.Atoi(wantWidthStr)
		if err != nil {
			return nil, nil, fmt.Errorf("parse width: %w", err)
		}
		file, err = image.GetFileForWidth(ctx, wantWidth)
		if err != nil {
			return nil, nil, fmt.Errorf("get image file: %w", err)
		}
	}

	reader, err := file.Reader(ctx)
	return file, reader, err
}
