package converter

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"

	"github.com/anyproto/anytype-heart/pb"
)

var ErrCancel = fmt.Errorf("import is canceled")
var ErrFailedToReceiveListOfObjects = fmt.Errorf("failed to receive the list of objects")
var ErrNoObjectsToImport = fmt.Errorf("source path doesn't contain objects to import")
var ErrLimitExceeded = fmt.Errorf("Limit of relations or objects are exceeded ")

type ConvertError struct {
	errors []error
	mode   pb.RpcObjectImportRequestMode
}

func NewError(mode pb.RpcObjectImportRequestMode) *ConvertError {
	return &ConvertError{
		errors: make([]error, 0),
		mode:   mode,
	}
}

func NewFromError(initialError error, mode pb.RpcObjectImportRequestMode) *ConvertError {
	ce := &ConvertError{mode: mode}

	ce.Add(initialError)

	return ce
}

func NewCancelError(err error) *ConvertError {
	return NewFromError(errors.Wrap(ErrCancel, err.Error()), pb.RpcObjectImportRequest_ALL_OR_NOTHING)
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
		case errors.Is(e, ErrFailedToReceiveListOfObjects):
			return ErrFailedToReceiveListOfObjects
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
	if importPathsCount == 0 {
		return false
	}
	var countNoObjectsToImport int
	for _, err := range ce.errors {
		if errors.Is(err, ErrNoObjectsToImport) {
			countNoObjectsToImport++
		}
	}
	return importPathsCount == countNoObjectsToImport
}
func (ce *ConvertError) ShouldAbortImport(pathsCount int, importType pb.RpcObjectImportRequestType) bool {
	return !ce.IsEmpty() && ce.mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING ||
		ce.IsNoObjectToImportError(pathsCount) ||
		errors.Is(ce.GetResultError(importType), ErrLimitExceeded) ||
		errors.Is(ce.GetResultError(importType), ErrCancel)
}
