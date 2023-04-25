package source

import (
	"io"
	"path/filepath"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("source-import")

var extensions = []string{".md", ".csv", ".txt", ".pb", ".json", ".html"}

type Source interface {
	GetFileReaders(importPath string, ext []string) (map[string]io.ReadCloser, error)
}

func GetSource(importPath string) Source {
	ext := filepath.Ext(importPath)
	switch {
	case strings.EqualFold(ext, ".zip"):
		return NewZip()
	case isSupportedExtension(ext, extensions):
		return NewFile()
	default:
		return NewDirectory()
	}
}

func isSupportedExtension(ext string, expectedExt []string) bool {
	return slices.Contains(expectedExt, ext)
}
