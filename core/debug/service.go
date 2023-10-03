package debug

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/go-chi/chi/v5"
	"github.com/gogo/protobuf/jsonpb"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

const CName = "debug"

var logger = logging.Logger("anytype-debug")

func New() Debug {
	return new(debug)
}

type Debug interface {
	app.Component
	DumpTree(ctx context.Context, objectID string, path string, anonymize bool, withSvg bool) (filename string, err error)
	DumpLocalstore(spaceID string, objectIds []string, path string) (filename string, err error)
	SpaceSummary() (summary SpaceSummary, err error)
	TreeHeads(id string) (info TreeInfo, err error)
}

type debug struct {
	block        *block.Service
	store        objectstore.ObjectStore
	spaceService spacecore.SpaceCoreService

	server *http.Server
}

type Debuggable interface {
	DebugRouter(r chi.Router)
}

func (d *debug) Init(a *app.App) (err error) {
	d.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	d.block = a.MustComponent(block.CName).(*block.Service)
	d.spaceService = app.MustComponent[spacecore.SpaceCoreService](a)

	d.initHandlers(a)
	return nil
}

func joinPath(parent string, child string) string {
	parent = strings.TrimSuffix(parent, "/*")
	return path.Join(parent, child)
}

func renderLinksList(w io.Writer, path string, routes []chi.Route) error {
	for _, r := range routes {
		if r.SubRoutes != nil {
			err := renderLinksList(w, joinPath(path, r.Pattern), r.SubRoutes.Routes())
			if err != nil {
				return err
			}
		} else {
			_, err := fmt.Fprintf(w, `<a href="%s">%s</a><br>`, joinPath(path, r.Pattern), joinPath(path, r.Pattern))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *debug) Run(ctx context.Context) error {
	if d.server != nil {
		go func() {
			err := d.server.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				logger.Error("debug server error:", err)
			}
		}()
	}
	return nil
}

func (d *debug) Close(ctx context.Context) error {
	if d.server != nil {
		err := d.server.Shutdown(ctx)
		if err != nil {
			return fmt.Errorf("debug server shutdown: %w", err)
		}
	}
	return nil
}

func (d *debug) Name() (name string) {
	return CName
}

type TreeInfo struct {
	Heads   []string
	Id      string
	SpaceId string
}

type SpaceSummary struct {
	SpaceId   string
	TreeInfos []TreeInfo
}

func (d *debug) SpaceSummary() (summary SpaceSummary, err error) {
	// TODO: [MR] revive this
	//spc, err := d.clientService.AccountSpace(context.Background())
	//if err != nil {
	//	return
	//}
	//summary.SpaceId = spc.Id()
	//for _, t := range spc.DebugAllHeads() {
	//	summary.TreeInfos = append(summary.TreeInfos, TreeInfo{
	//		Heads: t.Heads,
	//		Id:    t.Id,
	//	})
	//}
	return
}

func (d *debug) TreeHeads(id string) (info TreeInfo, err error) {
	// TODO: [MR] revive this
	//spc, err := d.clientService.AccountSpace(context.Background())
	//
	//if err != nil {
	//	return
	//}
	//tree, err := spc.TreeBuilder().BuildHistoryTree(context.Background(), id, objecttreebuilder.HistoryTreeOpts{})
	//if err != nil {
	//	return
	//}
	//info = TreeInfo{
	//	Id:      id,
	//	Heads:   tree.Heads(),
	//	SpaceId: spc.Id(),
	//}
	return
}

func (d *debug) DumpTree(ctx context.Context, objectID string, path string, anonymize bool, withSvg bool) (filename string, err error) {
	// TODO: [MR] revive this
	//// 0 - get space and tree
	//spc, err := d.clientService.AccountSpace(context.Background())
	//if err != nil {
	//	return
	//}
	//tree, err := spc.TreeBuilder().BuildHistoryTree(context.Background(), objectID, objecttreebuilder.HistoryTreeOpts{BuildFullTree: true})
	//if err != nil {
	//	return
	//}
	//
	//// 1 - create ZIP file
	//// <path>/at.dbg.bafkudtugh626rrqzah3kam4yj4lqbaw4bjayn2rz4ah4n5fpayppbvmq.20220322.121049.23.zip
	//spaceID, err := d.spaceService.ResolveSpaceID(objectID)
	//if err != nil {
	//	return "", fmt.Errorf("resolve spaceID: %w", err)
	//}
	//exporter := &treeExporter{s: d.store, anonymized: anonymize, id: domain.FullID{
	//	SpaceID:  spaceID,
	//	ObjectID: objectID,
	//}}
	//zipFilename, err := exporter.Export(ctx, path, tree)
	//if err != nil {
	//	logger.Error("build tree error:", err)
	//	return "", err
	//}
	//
	//// if client never asked for SVG generation -> return
	//if !withSvg {
	//	return zipFilename, err
	//}
	//
	//// 2 - create SVG file near ZIP
	//// <path>/at.dbg.bafkudtugh626rrqzah3kam4yj4lqbaw4bjayn2rz4ah4n5fpayppbvmq.20220322.121049.23.svg
	////
	//// this will return "graphviz is not supported on the current platform" error if no graphviz!
	//// generate a filename just like zip file had
	//maxReplacements := 1
	//svgFilename := strings.Replace(zipFilename, ".zip", ".svg", maxReplacements)
	//debugInfo, err := tree.Debug(state.ChangeParser{})
	//if err != nil {
	//	return
	//}
	//
	//err = GraphvizSvg(debugInfo.Graphviz, svgFilename)
	//if err != nil {
	//	return
	//}
	//
	//// return zip filename, but not svgFilename
	//return zipFilename, nil
	return "", nil
}

func (d *debug) DumpLocalstore(spaceID string, objIds []string, path string) (filename string, err error) {
	if len(objIds) == 0 {
		objIds, err = d.store.ListIds()
		if err != nil {
			return "", err
		}
	}

	filename = filepath.Join(path, fmt.Sprintf("at.store.dbg.%s.zip", time.Now().Format("20060102.150405.99")))
	f, err := os.Create(filename)
	if err != nil {
		return
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	var wr io.Writer
	m := jsonpb.Marshaler{Indent: " "}

	for _, objId := range objIds {
		doc, err := d.store.GetWithLinksInfoByID(spaceID, objId)
		if err != nil {
			var err2 error
			wr, err2 = zw.Create(fmt.Sprintf("%s.txt", objId))
			if err2 != nil {
				return "", err
			}

			wr.Write([]byte(err.Error()))
			continue
		}
		wr, err = zw.Create(fmt.Sprintf("%s.json", objId))
		if err != nil {
			return "", err
		}

		err = m.Marshal(wr, doc)
		if err != nil {
			return "", err
		}
	}
	return filename, nil
}
