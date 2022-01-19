package debug

import (
	"archive/zip"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/gogo/protobuf/jsonpb"
	"io"
	"os"
	"path/filepath"
	"time"
)

const CName = "debug"

func New() Debug {
	return new(debug)
}

type Debug interface {
	app.Component
	DumpTree(blockId, path string, anonymize bool) (filename string, err error)
	DumpLocalstore(objectIds []string, path string) (filename string, err error)
}

type debug struct {
	core  core.Service
	store objectstore.ObjectStore
}

func (d *debug) Init(a *app.App) (err error) {
	d.core = a.MustComponent(core.CName).(core.Service)
	d.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	return nil
}

func (d *debug) Name() (name string) {
	return CName
}

func (d *debug) DumpTree(blockId, path string, anonymize bool) (filename string, err error) {
	block, err := d.core.GetBlock(blockId)
	if err != nil {
		return
	}
	builder := &treeBuilder{b: block, s: d.store, anonymized: anonymize}
	return builder.Build(path)
}

func (d *debug) DumpLocalstore(objIds []string, path string) (filename string, err error) {
	if len(objIds) == 0 {
		objIds, err = d.core.ObjectStore().ListIds()
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
		doc, err := d.core.ObjectStore().GetWithLinksInfoByID(objId)
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
