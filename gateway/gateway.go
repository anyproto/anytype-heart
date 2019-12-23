package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/location"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	logging "github.com/ipfs/go-log"
	gincors "github.com/rs/cors/wrapper/gin"
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
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(location.Default())

	// Add the CORS middleware
	// Merges the API HTTPHeaders (from config/init) into blank/default CORS configuration
	router.Use(gincors.AllowAll())

	router.GET("/health", func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusNoContent)
	})
	router.GET("/file/:filehash", g.fileHandler)

	g.server = &http.Server{
		Addr:    addr,
		Handler: router,
	}

	errc := make(chan error)
	go func() {
		errc <- g.server.ListenAndServe()
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

// fileHandler gets file meta from the DB, gets the corresponding data from the IPFS and decrypts it
func (g *Gateway) fileHandler(c *gin.Context) {
	fileHash := c.Param("filehash")

	reader, index, err := g.Node.FileContent(fileHash)
	if err != nil {
		if strings.Contains(err.Error(), tcore.ErrFileNotFound.Error()) {
			c.String(404, "file not found")
			return
		}
		c.String(500, err.Error())
		return
	}
	// todo: find a way to use readseeker for the Range request
	c.Render(200, render.Reader{
		Reader:        reader,
		ContentType:   index.Media,
		ContentLength: index.Size,
		Headers: map[string]string{
			"Content-Disposition": fmt.Sprintf("inline; filename=\"%s\"", index.Name),
		},
	})
}
