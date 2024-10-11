package common

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

func ProvideFileName(fileName string, filesSource source.Source, path string, tempDirProvider core.TempDirProvider) (string, bool, error) {
	if strings.HasPrefix(strings.ToLower(fileName), "http://") || strings.HasPrefix(strings.ToLower(fileName), "https://") {
		return fileName, false, nil
	}
	var createFileBlock bool
	// first try to check if file exist on local machine
	absolutePath := fileName
	if !filepath.IsAbs(fileName) {
		absolutePath = filepath.Join(path, fileName)
	}
	if _, err := os.Stat(absolutePath); err == nil {
		createFileBlock = true
		return absolutePath, createFileBlock, nil
	}
	// second case for archive, when file is inside zip archive
	if err := filesSource.ProcessFile(fileName, func(fileReader io.ReadCloser) error {
		tempFile, err := extractFileFromArchiveToTempDirectory(fileName, fileReader, tempDirProvider)
		if err != nil {
			return err
		}
		createFileBlock = true
		fileName = tempFile
		return nil
	}); err != nil {
		return "", false, fmt.Errorf("%w: %s", ErrFileImportSourceFileOpenError, err.Error())
	}
	return fileName, createFileBlock, nil
}

func extractFileFromArchiveToTempDirectory(fileName string, rc io.ReadCloser, tempDirProvider core.TempDirProvider) (string, error) {
	tempDir := tempDirProvider.TempDir()
	directoryWithFile := filepath.Dir(fileName)
	if directoryWithFile != "" {
		directoryWithFile = filepath.Join(tempDir, directoryWithFile)
		if err := os.MkdirAll(directoryWithFile, 0777); err != nil && !os.IsExist(err) {
			return "", anyerror.CleanupError(err)
		}
	}
	pathToTmpFile := filepath.Join(tempDir, fileName)
	tmpFile, err := os.Create(pathToTmpFile)
	if os.IsExist(err) {
		return pathToTmpFile, nil
	}
	if err != nil {
		return "", anyerror.CleanupError(err)
	}
	defer tmpFile.Close()
	w := bufio.NewWriter(tmpFile)
	_, err = w.ReadFrom(rc)
	if err != nil {
		return "", err
	}
	if err = w.Flush(); err != nil {
		return "", err
	}
	return pathToTmpFile, nil
}
