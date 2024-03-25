package os

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransformError(t *testing.T) {
	sep := string(filepath.Separator)

	t.Run("absolute path", func(t *testing.T) {
		pathError := &os.PathError{
			Op:   "read",
			Path: sep + "test" + sep + "file" + sep + "path" + sep,
			Err:  fmt.Errorf("test"),
		}

		resultErrorMessage := "read /***/***/***/: test"
		assert.NotNil(t, TransformError(pathError))
		assert.Equal(t, resultErrorMessage, TransformError(pathError).Error())
	})

	t.Run("relative path", func(t *testing.T) {
		pathError := &os.PathError{
			Op:   "read",
			Path: "test" + sep + "file",
			Err:  fmt.Errorf("test"),
		}

		resultErrorMessage := "read ***/***: test"
		assert.NotNil(t, TransformError(pathError))
		assert.Equal(t, resultErrorMessage, TransformError(pathError).Error())
	})

	t.Run("not os path error", func(t *testing.T) {
		err := fmt.Errorf("test")
		resultErrorMessage := "test"
		assert.NotNil(t, TransformError(err))
		assert.Equal(t, resultErrorMessage, TransformError(err).Error())
	})

	t.Run("url error", func(t *testing.T) {
		err := &url.Error{URL: "http://test.test", Op: "Test", Err: fmt.Errorf("test")}
		resultErrorMessage := "Test \"<masked url>\": test"
		assert.NotNil(t, TransformError(err))
		assert.Equal(t, resultErrorMessage, TransformError(err).Error())
	})
}
