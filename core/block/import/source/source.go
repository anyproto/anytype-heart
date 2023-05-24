package source

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("import-source")

var extensions = []string{".md", ".csv", ".txt", ".pb", ".json", ".html"}

type Source interface {
	GetFileReaders(importPath string, ext []string) (map[string]io.ReadCloser, error)
}

func GetSource(importPath string) Source {
	importFileExt := filepath.Ext(importPath)
	switch {
	case strings.EqualFold(importFileExt, ".zip"):
		return NewZip()
	case isSupportedExtension(importFileExt, extensions):
		return NewFile()
	default:
		return NewDirectory()
	}
}

func isSupportedExtension(ext string, expectedExt []string) bool {
	return lo.Contains(expectedExt, ext)
}
