package converter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/anyproto/anytype-heart/pb"
)

var ErrCancel = fmt.Errorf("import is canceled")
var ErrNoObjectsToImport = fmt.Errorf("source path doesn't contain objects to import")
var ErrLimitExceeded = fmt.Errorf("Limit of relations or objects are exceeded ")

type ConvertError map[string]error

func NewError() ConvertError {
	return ConvertError{}
}

func NewFromError(name string, initialError error) ConvertError {
	ce := ConvertError{}

	ce.Add(name, initialError)

	return ce
}

func NewCancelError(path string, err error) ConvertError {
	wrappedError := errors.Wrap(ErrCancel, err.Error())
	cancelError := NewFromError(path, wrappedError)
	return cancelError
}

func (ce ConvertError) Add(objectName string, err error) {
	ce[objectName] = err
}

func (ce ConvertError) Merge(err ConvertError) {
	for fileName, errPb := range err {
		ce[fileName] = errPb
	}
}

func (ce ConvertError) IsEmpty() bool {
	return len(ce) == 0
}

func (ce ConvertError) Error() error {
	var pattern = "source: %s, error: %s" + "\n"
	var errorString bytes.Buffer
	if ce.IsEmpty() {
		return nil
	}
	for name, err := range ce {
		errorString.WriteString(fmt.Sprintf(pattern, name, err.Error()))
	}
	return fmt.Errorf(errorString.String())
}

func (ce ConvertError) Get(objectName string) error {
	return ce[objectName]
}

func (ce ConvertError) GetResultError(importType pb.RpcObjectImportRequestType) error {
	if ce.IsEmpty() {
		return nil
	}
	var (
		countNoObjectsToImport          int
		limitErrorString                strings.Builder
		limitError, noObjectImportError error
	)
	for path, e := range ce {
		switch {
		case errors.Is(e, ErrCancel):
			return errors.Wrapf(ErrCancel, "import type: %s", importType.String())
		case errors.Is(e, ErrLimitExceeded):
			limitError = ErrLimitExceeded
			limitErrorString.WriteString(fmt.Sprintf("import path: %s\n", path))
		case errors.Is(e, ErrNoObjectsToImport):
			noObjectImportError = ErrNoObjectsToImport
			countNoObjectsToImport++
		}
	}
	// we return ErrNoObjectsToImport only if all paths has such error, otherwise we assume that import finished with internal code error
	if (countNoObjectsToImport == len(ce)) && noObjectImportError != nil {
		return errors.Wrapf(noObjectImportError, "import type: %s", importType.String())
	}
	if limitError != nil {
		return errors.Wrap(limitError, limitErrorString.String())
	}
	return errors.Wrapf(ce.Error(), "import type: %s", importType.String())
}

func (ce ConvertError) IsNoObjectToImportError(importPathsCount int) bool {
	var countNoObjectsToImport int
	for _, err := range ce {
		if errors.Is(err, ErrNoObjectsToImport) {
			countNoObjectsToImport++
		}
	}
	return importPathsCount == countNoObjectsToImport
}
