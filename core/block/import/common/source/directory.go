package source

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/util/anyerror"
)

type Directory struct {
	fileReaders   map[string]struct{}
	importPath    string
	rootDirs      map[string]bool
	selectedPaths map[string]bool // For filtering
}

func NewDirectory() *Directory {
	return &Directory{fileReaders: make(map[string]struct{}, 0)}
}

func (d *Directory) Initialize(importPath string) error {
	return d.InitializeWithFilter(importPath, nil)
}

func (d *Directory) InitializeWithFilter(importPath string, selectedPaths []string) error {
	// Build selectedPaths map for quick lookup
	d.selectedPaths = make(map[string]bool)
	if len(selectedPaths) > 0 {
		for _, p := range selectedPaths {
			absPath, err := filepath.Abs(p)
			if err != nil {
				continue
			}
			d.selectedPaths[absPath] = true
		}
	}
	
	files := make(map[string]struct{})
	err := filepath.Walk(importPath,
		func(path string, info os.FileInfo, err error) error {
			if strings.HasPrefix(info.Name(), ".DS_Store") {
				return nil
			}
			if info != nil && !info.IsDir() {
				// If we have a filter, check if this file should be included
				if len(d.selectedPaths) > 0 {
					absPath, err := filepath.Abs(path)
					if err != nil {
						return nil
					}
					
					// Check if this file is in the selected paths or is a descendant of a selected directory
					include := false
					if d.selectedPaths[absPath] {
						include = true
					} else {
						// Check if any selected path is a directory that contains this file
						for selectedPath := range d.selectedPaths {
							if strings.HasPrefix(absPath, selectedPath+string(filepath.Separator)) {
								include = true
								break
							}
						}
					}
					
					if !include {
						return nil
					}
				}
				
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
	return fileDir == d.importPath || d.rootDirs[fileDir]
}

func (d *Directory) Close() {}

func findNonEmptyDirs(files map[string]struct{}) map[string]bool {
	dirs := make([]string, 0, len(files))
	for file := range files {
		dir := filepath.Dir(file)
		if dir == "." {
			return map[string]bool{dir: true}
		}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	result := make(map[string]bool)
	visited := make(map[string]bool)

	for _, dir := range dirs {
		if _, ok := visited[dir]; ok {
			continue
		}
		visited[dir] = true
		if isSubdirectoryOfAny(dir, result) {
			continue
		}
		result[dir] = true
	}

	return result
}

func isSubdirectoryOfAny(dir string, directories map[string]bool) bool {
	for base := range directories {
		if strings.HasPrefix(dir, base+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
