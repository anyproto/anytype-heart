package debug

import (
	"bytes"
	"context"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/core/debug/exporter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/anonymize"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/ziputil"
)

type treeExporter struct {
	log        *stdlog.Logger
	s          objectstore.ObjectStore
	anonymized bool
	id         domain.FullID
}

func (e *treeExporter) Export(ctx context.Context, path string, tree objecttree.ReadableObjectTree) (filename string, err error) {
	var (
		exportDirPath  = filepath.Join(path, fmt.Sprintf("at.dbg.%s.%s", e.id, time.Now().Format("20060102.150405.99")))
		dbPath         = filepath.Join(exportDirPath, "db")
		localStorePath = filepath.Join(exportDirPath, "localstore.json")
		logPath        = filepath.Join(exportDirPath, "creation.log")
	)
	filename = exportDirPath + ".zip"
	err = os.MkdirAll(exportDirPath, 0755)
	if err != nil {
		return
	}
	defer func() {
		_ = os.RemoveAll(exportDirPath)
	}()
	anyStore, err := anystore.Open(ctx, dbPath, nil)
	if err != nil {
		return
	}
	defer func() {
		_ = anyStore.Close()
	}()
	exportParams := exporter.ExportParams{
		Readable:  tree,
		Store:     anyStore,
		Converter: &changeDataConverter{anonymize: e.anonymized},
	}
	st := time.Now()
	err = exporter.ExportTree(ctx, exportParams)
	if err != nil {
		return
	}
	logBuf := bytes.NewBuffer(nil)
	e.log = stdlog.New(io.MultiWriter(logBuf, os.Stderr), "", stdlog.LstdFlags)
	e.log.Printf("exported tree for a %v", time.Since(st))
	data, err := e.s.SpaceIndex(e.id.SpaceID).GetInfosByIds([]string{e.id.ObjectID})
	if err != nil {
		e.log.Printf("can't fetch localstore info: %v", err)
	} else {
		if len(data) > 0 {
			data[0].Details = transform(data[0].Details, e.anonymized, anonymize.Details)
			data[0].Snippet = transform(data[0].Snippet, e.anonymized, anonymize.Text)
			for i, r := range data[0].Relations {
				data[0].Relations[i] = transform(r, e.anonymized, anonymize.Relation)
			}
			osData := pbtypes.Sprint(data[0].ToProto())
			er := os.WriteFile(localStorePath, []byte(osData), 0600)
			if er != nil {
				e.log.Printf("localstore.json write error: %v", err)
			} else {
				e.log.Printf("localstore.json wrote")
			}
		} else {
			e.log.Printf("no data in objectstore")
		}
	}
	err = os.WriteFile(logPath, logBuf.Bytes(), 0600)
	if err != nil {
		return
	}
	err = ziputil.ZipFolder(exportDirPath, filename)
	return
}

func transform[T any](in T, transformed bool, f func(T) T) T {
	if transformed {
		return f(in)
	}
	return in
}
