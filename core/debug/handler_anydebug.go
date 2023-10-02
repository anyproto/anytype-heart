//go:build anydebug

package debug

import (
	"fmt"
	"net/http"
	"os"

	"github.com/anyproto/any-sync/app"
	"github.com/go-chi/chi/v5"
)

func (d *debug) initHandlers(a *app.App) {
	if addr, ok := os.LookupEnv("ANYDEBUG"); ok && addr != "" {
		r := chi.NewRouter()
		a.IterateComponents(func(c app.Component) {
			if d, ok := c.(Debuggable); ok {
				fmt.Println("debug router registered for component: ", c.Name())
				r.Route("/debug/"+c.Name(), d.DebugRouter)
			}
		})
		routes := r.Routes()
		r.Get("/debug", func(w http.ResponseWriter, req *http.Request) {
			err := renderLinksList(w, "/", routes)
			if err != nil {
				logger.Error("failed to render links list", err)
			}
		})
		d.server = &http.Server{
			Addr:    addr,
			Handler: r,
		}
	}
}
