package os

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samber/lo"
)

// badgerFileErrPattern is a regular expression pattern to match file paths in badger errors.
// seems like all badger errors with file paths have this pattern.
var badgerFileErrPattern = regexp.MustCompile(`file: (.*?), error:`)

// anonymizeBadgerError anonymizes a non-typed badger errors that contain file paths.
func anonymizeBadgerError(err error) error {
	if err == nil {
		return nil
	}
	if submatch := badgerFileErrPattern.FindStringSubmatch(err.Error()); len(submatch) > 0 {
		if len(submatch) > 0 {
			anonymizedPath := "*" + string(os.PathSeparator) + filepath.Base(strings.TrimSpace(submatch[1]))
			err = errors.New(strings.Replace(err.Error(), submatch[1], anonymizedPath, 1))
		}
	}
	return err
}

func TransformError(err error) error {
	if err == nil {
		return nil
	}
	if pathErr, ok := err.(*os.PathError); ok {
		return anonymizePathError(pathErr)
	}
	if urlErr, ok := err.(*url.Error); ok {
		return anonymizeUrlError(urlErr)
	}

	return anonymizeBadgerError(err)
}

func anonymizePathError(pathErr *os.PathError) error {
	filePathParts := strings.Split(pathErr.Path, string(filepath.Separator))
	anonymizedFilePathParts := lo.Map(filePathParts, func(item string, index int) string {
		if item == "" {
			return ""
		}
		return strings.ReplaceAll(item, item, "***")
	})
	newPath := strings.Join(anonymizedFilePathParts, string(filepath.Separator))
	pathErr.Path = newPath
	return pathErr
}

func anonymizeUrlError(urlErr *url.Error) error {
	urlErr.URL = "<masked url>"
	return urlErr
}
