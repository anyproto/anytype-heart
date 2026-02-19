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
	Initialize(importPath string) error
	Iterate(callback func(fileName string, fileReader io.ReadCloser) bool) error
	ProcessFile(fileName string, callback func(fileReader io.ReadCloser) error) error
	CountFilesWithGivenExtensions(extensions []string) int
	Close()
	IsRootFile(fileName string) bool
}

// FilterableSource extends Source with the ability to filter files
type FilterableSource interface {
	Source
	InitializeWithFilter(importPath string, selectedPaths []string) error
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
