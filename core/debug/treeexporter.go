package debug

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/anytypeio/any-sync/commonspace/object/tree/exporter"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/anytypeio/go-anytype-middleware/core/debug/treearchive"
	"github.com/anytypeio/go-anytype-middleware/util/anonymize"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
)

type treeExporter struct {
	log        *log.Logger
	s          objectstore.ObjectStore
	anonymized bool
	id         string
	zw         *zip.Writer
}

func (e *treeExporter) Export(path string, tree objecttree.ReadableObjectTree) (filename string, err error) {
	filename = filepath.Join(path, fmt.Sprintf("at.dbg.%s.%s.zip", e.id, time.Now().Format("20060102.150405.99")))
	archiveWriter, err := treearchive.NewArchiveWriter(filename)
	if err != nil {
		return
	}
	defer archiveWriter.Close()

	e.zw = archiveWriter.ZipWriter()
	params := exporter.TreeExporterParams{
		ListStorageExporter: archiveWriter,
		TreeStorageExporter: archiveWriter,
		DataConverter:       &changeDataConverter{anonymize: e.anonymized},
	}
	anySyncExporter := exporter.NewTreeExporter(params)
	logBuf := bytes.NewBuffer(nil)

	e.log = log.New(io.MultiWriter(logBuf, os.Stderr), "", log.LstdFlags)
	e.log.Printf("exporting tree and acl")
	st := time.Now()
	err = anySyncExporter.ExportUnencrypted(tree)
	if err != nil {
		e.log.Printf("export tree in zip error: %v", err)
		return
	}

	e.log.Printf("exported tree for a %v", time.Since(st))
	data, err := e.s.GetByIDs(e.id)
	if err != nil {
		e.log.Printf("can't fetch localstore info: %v", err)
	} else {
		if len(data) > 0 {
			data[0].Details = transform(data[0].Details, e.anonymized, anonymize.Struct)
			data[0].Snippet = transform(data[0].Snippet, e.anonymized, anonymize.Text)
			for i, r := range data[0].Relations {
				data[0].Relations[i] = transform(r, e.anonymized, anonymize.Relation)
			}
			osData := pbtypes.Sprint(data[0])
			lsWr, er := e.zw.Create("localstore.json")
			if er != nil {
				e.log.Printf("create file in zip error: %v", er)
			} else {
				if _, err := lsWr.Write([]byte(osData)); err != nil {
					e.log.Printf("localstore.json write error: %v", err)
				} else {
					e.log.Printf("localstore.json wrote")
				}
			}
		} else {
			e.log.Printf("not data in objectstore")
		}
	}
	logW, err := e.zw.Create("creation.log")
	if err != nil {
		return
	}
	io.Copy(logW, logBuf)
	return
}

func transform[T any](in T, transformed bool, f func(T) T) T {
	if transformed {
		return f(in)
	}
	return in
}
