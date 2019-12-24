package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	logging "github.com/ipfs/go-log"
	tcore "github.com/textileio/go-textile/core"
)

const defaultGatewayAddr = "127.0.0.1:47800"

var log = logging.Logger("anytype-gateway")

// Host is the instance used by the daemon
var Host *Gateway

// Gateway is a HTTP API for getting files and links from IPFS
type Gateway struct {
	Node   *tcore.Textile
	server *http.Server
}

func GatewayAddr() string {
	if addr := os.Getenv("ANYTYPE_GATEWAY_ADDR"); addr != "" {
		return addr
	}

	return defaultGatewayAddr
}

// Start creates a gateway server
func (g *Gateway) Start(addr string) {
	handler := http.NewServeMux()
	g.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	handler.HandleFunc("/file/", g.fileHandler)

	errc := make(chan error)
	go func() {
		errc <- http.ListenAndServe(addr, handler)
		close(errc)
	}()
	go func() {
		for {
			select {
			case err, ok := <-errc:
				if err != nil && err != http.ErrServerClosed {
					log.Errorf("gateway error: %s", err)
				}
				if !ok {
					log.Info("gateway was shutdown")
					return
				}
			}
		}
	}()

	log.Infof("gateway listening at %s", g.server.Addr)
}

// Stop stops the gateway
func (g *Gateway) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := g.server.Shutdown(ctx); err != nil {
		log.Errorf("error shutting down gateway: %s", err)
		return err
	}
	return nil
}

// Addr returns the gateway's address
func (g *Gateway) Addr() string {
	return g.server.Addr
}

func enableCors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
}

// fileHandler gets file meta from the DB, gets the corresponding data from the IPFS and decrypts it
func (g *Gateway) fileHandler(w http.ResponseWriter, r *http.Request) {
	fileHash := r.URL.Path[len("/file/"):]
	enableCors(w)
	reader, index, err := g.Node.FileContent(fileHash)
	if err != nil {
		if strings.Contains(err.Error(), tcore.ErrFileNotFound.Error()) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", index.Media)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", index.Name))

	added, _ := ptypes.Timestamp(index.Added)
	// todo: inside textile it still requires the file to be fully downloaded and decrypted(consuming 2xSize in ram) to provide the ReadSeeker interface
	// 	need to find a way to use ReadSeeker all the way from downloading files from IPFS to writing the decrypted chunk to the HTTP
	http.ServeContent(w, r, index.Name, added, reader)
}
