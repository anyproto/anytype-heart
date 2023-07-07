package converter

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"

	"github.com/anyproto/anytype-heart/pb"
)

var ErrCancel = fmt.Errorf("import is canceled")
var ErrNoObjectsToImport = fmt.Errorf("source path doesn't contain objects to import")
var ErrLimitExceeded = fmt.Errorf("Limit of relations or objects are exceeded ")

type ConvertError struct {
	errors []error
}

func NewError() *ConvertError {
	return &ConvertError{
		errors: make([]error, 0),
	}
}

func NewFromError(initialError error) *ConvertError {
	ce := &ConvertError{}

	ce.Add(initialError)

	return ce
}

func NewCancelError(err error) *ConvertError {
	wrappedError := errors.Wrap(ErrCancel, err.Error())
	cancelError := NewFromError(wrappedError)
	return cancelError
}

func (ce *ConvertError) Add(err error) {
	ce.errors = append(ce.errors, err)
}

func (ce *ConvertError) Merge(err *ConvertError) {
	ce.errors = append(ce.errors, err.errors...)
}

func (ce *ConvertError) IsEmpty() bool {
	return ce == nil || len(ce.errors) == 0
}

func (ce *ConvertError) Error() error {
	var pattern = "error: %s" + "\n"
	var errorString bytes.Buffer
	if ce.IsEmpty() {
		return nil
	}
	for _, err := range ce.errors {
		errorString.WriteString(fmt.Sprintf(pattern, err.Error()))
	}
	return fmt.Errorf(errorString.String())
}

func (ce *ConvertError) GetResultError(importType pb.RpcObjectImportRequestType) error {
	if ce.IsEmpty() {
		return nil
	}
	var countNoObjectsToImport int
	for _, e := range ce.errors {
		switch {
		case errors.Is(e, ErrCancel):
			return errors.Wrapf(ErrCancel, "import type: %s", importType.String())
		case errors.Is(e, ErrLimitExceeded):
			return errors.Wrapf(ErrLimitExceeded, "import type: %s", importType.String())
		case errors.Is(e, ErrNoObjectsToImport):
			countNoObjectsToImport++
		}
	}
	// we return ErrNoObjectsToImport only if all paths has such error, otherwise we assume that import finished with internal code error
	if countNoObjectsToImport == len(ce.errors) {
		return errors.Wrapf(ErrNoObjectsToImport, "import type: %s", importType.String())
	}
	return errors.Wrapf(ce.Error(), "import type: %s", importType.String())
}

func (ce *ConvertError) IsNoObjectToImportError(importPathsCount int) bool {
	var countNoObjectsToImport int
	for _, err := range ce.errors {
		if errors.Is(err, ErrNoObjectsToImport) {
			countNoObjectsToImport++
		}
	}
	return importPathsCount == countNoObjectsToImport
}
