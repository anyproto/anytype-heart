package os

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
)

func TransformError(err error) error {
	if pathErr, ok := err.(*os.PathError); ok {
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
	return err
}
