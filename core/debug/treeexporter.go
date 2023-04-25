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
	exp, err := treearchive.NewExporter(filename)
	if err != nil {
		return
	}
	defer exp.Close()

	e.zw = exp.Writer()
	params := exporter.TreeExporterParams{
		ListStorageExporter: exp,
		TreeStorageExporter: exp,
		DataConverter:       &changeDataConverter{anonymize: e.anonymized},
	}
	logBuf := bytes.NewBuffer(nil)
	e.log = log.New(io.MultiWriter(logBuf, os.Stderr), "", log.LstdFlags)

	st := time.Now()
	treeExporter := exporter.NewTreeExporter(params)
	e.log.Printf("exporting tree and acl")
	err = treeExporter.ExportUnencrypted(tree)
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
