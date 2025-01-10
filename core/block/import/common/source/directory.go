package source

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/util/anyerror"
)

type Directory struct {
	fileReaders map[string]struct{}
	importPath  string
	rootDirs    []string
}

func NewDirectory() *Directory {
	return &Directory{fileReaders: make(map[string]struct{}, 0)}
}

func (d *Directory) Initialize(importPath string) error {
	files := make(map[string]struct{})
	err := filepath.Walk(importPath,
		func(path string, info os.FileInfo, err error) error {
			if strings.HasPrefix(info.Name(), ".DS_Store") {
				return nil
			}
			if info != nil && !info.IsDir() {
				files[path] = struct{}{}
			}
			return nil
		},
	)
	d.fileReaders = files
	d.importPath = importPath
	d.rootDirs = findNonEmptyDirs(files)
	if err != nil {
		return err
	}
	return nil
}

func (d *Directory) Iterate(callback func(fileName string, fileReader io.ReadCloser) bool) error {
	for file := range d.fileReaders {
		fileReader, err := os.Open(file)
		if err != nil {
			return anyerror.CleanupError(err)
		}
		isContinue := callback(file, fileReader)
		fileReader.Close()
		if !isContinue {
			break
		}
	}
	return nil
}

func (d *Directory) ProcessFile(fileName string, callback func(fileReader io.ReadCloser) error) error {
	if _, ok := d.fileReaders[fileName]; ok {
		fileReader, err := os.Open(fileName)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return anyerror.CleanupError(err)
		}
		defer fileReader.Close()
		if err = callback(fileReader); err != nil {
			return err
		}
	}
	return nil
}

func (d *Directory) CountFilesWithGivenExtensions(extension []string) int {
	var numberOfFiles int
	for name := range d.fileReaders {
		if lo.Contains(extension, filepath.Ext(name)) {
			numberOfFiles++
		}
	}
	return numberOfFiles
}

func (d *Directory) IsRootFile(fileName string) bool {
	fileDir := filepath.Dir(fileName)
	return fileDir == d.importPath || slices.Contains(d.rootDirs, fileDir)
}

func (d *Directory) Close() {}

func findNonEmptyDirs(files map[string]struct{}) []string {
	dirs := make([]string, 0, len(files))
	for file := range files {
		dir := filepath.Dir(file)
		if dir == "." {
			return []string{dir}
		}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	var result []string
	visited := make(map[string]bool)

	for _, dir := range dirs {
		if isSubdirectoryOfAny(dir, result) {
			continue
		}
		result = lo.Union(result, []string{dir})
		visited[dir] = true
	}

	return result
}

func isSubdirectoryOfAny(dir string, directories []string) bool {
	for _, base := range directories {
		if strings.HasPrefix(dir, base+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
