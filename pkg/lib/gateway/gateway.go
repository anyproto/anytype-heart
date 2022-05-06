package gateway

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

const CName = "gateway"

const defaultPort = 47800

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
	Node    core.Service
	server  *http.Server
	handler *http.ServeMux
	addr    string
	mu      sync.Mutex
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

	port, err := getRandomPort()
	if err != nil {
		log.Errorf("failed to get random port for gateway, go with the default %d", defaultPort)
		port = defaultPort
	}

	return fmt.Sprintf("127.0.0.1:%d", port)
}

func (g *gateway) Init(a *app.App) (err error) {
	g.Node = a.MustComponent(core.CName).(core.Service)
	g.addr = GatewayAddr()
	log.Debugf("gateway.Init: %s", g.addr)
	return nil
}

func (g *gateway) Name() string {
	return CName
}

func (g *gateway) Run() error {
	if g.server != nil {
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
func (g *gateway) Close() error {
	err := g.stopServer()
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

	if g.server != nil {
		log.Errorf("server already started")
		return
	}

	g.server = &http.Server{
		Addr:    g.addr,
		Handler: g.handler,
	}

	go func() {
		err := g.server.ListenAndServe()
		g.server = nil
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("gateway error: %s", err)
			return
		}
		log.Info("gateway was shutdown")
	}()

	log.Infof("gateway listening at %s", g.server.Addr)
}

func (g *gateway) stopServer() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err := g.server.Shutdown(ctx)
	g.server = nil
	return err
}

func enableCors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
}

// fileHandler gets file meta from the DB, gets the corresponding data from the IPFS and decrypts it
func (g *gateway) fileHandler(w http.ResponseWriter, r *http.Request) {
	fileHashAndPath := r.URL.Path[len("/file/"):]
	enableCors(w)
	var fileHash string
	parts := strings.Split(fileHashAndPath, "/")
	fileHash = parts[0]
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	file, err := g.Node.FileByHash(ctx, fileHash)
	if err != nil {
		if strings.Contains(err.Error(), "file not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}

	reader, err := file.Reader()
	if err != nil {
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

// fileHandler gets file meta from the DB, gets the corresponding data from the IPFS and decrypts it
func (g *gateway) imageHandler(w http.ResponseWriter, r *http.Request) {
	urlParts := strings.Split(r.URL.Path, "/")
	imageHash := urlParts[2]
	query := r.URL.Query()

	enableCors(w)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	image, err := g.Node.ImageByHash(ctx, imageHash)
	if err != nil {
		if strings.Contains(err.Error(), "file not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	var file core.File
	wantWidthStr := query.Get("width")
	if wantWidthStr == "" {
		file, err = image.GetOriginalFile(ctx)
	} else {
		wantWidth, err2 := strconv.Atoi(wantWidthStr)
		if err2 != nil {
			http.Error(w, err2.Error(), 400)
			return
		}

		file, err = image.GetFileForWidth(ctx, wantWidth)
	}

	if err != nil {
		if strings.Contains(err.Error(), "file not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}

	reader, err := file.Reader()
	if err != nil {
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
