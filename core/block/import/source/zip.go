package source

import (
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
)

type Zip struct{}

func NewZip() *Zip {
	return &Zip{}
}

func (d *Zip) GetFileReaders(importPath, expectedExt string) (map[string]io.ReadCloser, error) {
	r, err := zip.OpenReader(importPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	files := make(map[string]io.ReadCloser, 0)
	zipName := strings.TrimSuffix(importPath, filepath.Ext(importPath))
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		actualExt := filepath.Ext(importPath)
		if !strings.EqualFold(actualExt, expectedExt) {
			log.Errorf("not expected extension: %s, %s", expectedExt, actualExt)
			continue
		}
		shortPath := filepath.Clean(f.Name)
		// remove zip root folder if exists
		shortPath = strings.TrimPrefix(shortPath, zipName+"/")

		rc, err := f.Open()
		if err != nil {
			log.Errorf("failed to read file: %s", err.Error())
			continue
		}
		files[shortPath] = rc
	}

	return files, nil
}
