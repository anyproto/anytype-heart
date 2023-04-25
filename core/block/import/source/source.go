package source

import (
	"io"
	"path/filepath"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("source-import")

var extensions = []string{".md", ".csv", ".txt", ".pb", ".json"}

type Source interface {
	GetFileReaders(importPath string, ext []string) (map[string]io.ReadCloser, error)
}

func GetSource(importPath string) Source {
	ext := filepath.Ext(importPath)
	if strings.EqualFold(ext, ".zip") {
		return NewZip()
	} else if isSupportedExtension(ext, extensions) {
		return NewFile()
	} else {
		return NewDirectory()
	}
	return nil
}

func isSupportedExtension(ext string, expectedExt []string) bool {
	return slices.Contains(expectedExt, ext)
}
