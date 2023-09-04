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
	GetFileReaders(importPath string, ext []string, includeFiles []string) (map[string]io.ReadCloser, error)
	Close()
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

func isFileAllowedToImport(fileName, ext string, expectedExt, includeFiles []string) bool {
	return isSupportedExtension(ext, expectedExt) || lo.Contains(includeFiles, filepath.Base(fileName))
}

func CountFilesWithGivenExtension(fileReaders map[string]io.ReadCloser, extension string) int {
	var numberOfFiles int
	for name := range fileReaders {
		if filepath.Ext(name) == extension {
			numberOfFiles++
		}
	}
	return numberOfFiles
}
