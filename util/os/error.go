package os

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
)

func TransformError(err error) error {
	if pathErr, ok := err.(*os.PathError); ok {
		return anonymizePathError(pathErr)
	}
	if urlErr, ok := err.(*url.Error); ok {
		return anonymizeUrlError(urlErr)
	}
	return err
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
