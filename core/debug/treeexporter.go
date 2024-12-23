package debug

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/core/debug/exporter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/anonymize"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	err = os.Mkdir(exportDirPath, 0755)
	if err != nil {
		return
	}
	defer func() {
		_ = os.RemoveAll(exportDirPath)
	}()
	err = os.Mkdir(dbPath, 0755)
	if err != nil {
		return
	}
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
	err = anyStore.Checkpoint(ctx, true)
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
			data[0].Details = transform(data[0].Details, e.anonymized, anonymize.Struct)
			data[0].Snippet = transform(data[0].Snippet, e.anonymized, anonymize.Text)
			for i, r := range data[0].Relations {
				data[0].Relations[i] = transform(r, e.anonymized, anonymize.Relation)
			}
			osData := pbtypes.Sprint(data[0])
			er := ioutil.WriteFile(localStorePath, []byte(osData), 0644)
			if er != nil {
				e.log.Printf("localstore.json write error: %v", err)
			} else {
				e.log.Printf("localstore.json wrote")
			}
		} else {
			e.log.Printf("no data in objectstore")
		}
	}
	err = ioutil.WriteFile(logPath, logBuf.Bytes(), 0644)
	if err != nil {
		return
	}
	err = zipFolder(exportDirPath, filename)
	return
}

func transform[T any](in T, transformed bool, f func(T) T) T {
	if transformed {
		return f(in)
	}
	return in
}

func zipFolder(source, targetZip string) error {
	zipFile, err := os.Create(targetZip)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		if info.IsDir() {
			_, err := writer.Create(strings.ReplaceAll(relPath, "\\", "/") + "/")
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		zipWriter, err := writer.Create(strings.ReplaceAll(relPath, "\\", "/"))
		if err != nil {
			return err
		}

		// Copy the file data to the zip entry.
		_, err = io.Copy(zipWriter, f)
		return err
	})
}
