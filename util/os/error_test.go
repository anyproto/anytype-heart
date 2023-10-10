package os

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransformError(t *testing.T) {
	sep := string(filepath.Separator)
	pathError := &os.PathError{
		Op:   "read",
		Path: sep + "test" + sep + "file" + sep + "path" + sep,
		Err:  fmt.Errorf("test"),
	}

	resultErrorMessage := "read /***/***/***/: test"
	assert.NotNil(t, TransformError(pathError))
	assert.Equal(t, resultErrorMessage, TransformError(pathError).Error())

	pathError = &os.PathError{
		Op:   "read",
		Path: "test" + sep + "file",
		Err:  fmt.Errorf("test"),
	}

	resultErrorMessage = "read ***/***: test"
	assert.NotNil(t, TransformError(pathError))
	assert.Equal(t, resultErrorMessage, TransformError(pathError).Error())

	err := fmt.Errorf("test")
	resultErrorMessage = "test"
	assert.NotNil(t, TransformError(err))
	assert.Equal(t, resultErrorMessage, TransformError(err).Error())
}
