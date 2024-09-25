package common

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var ErrCancel = fmt.Errorf("import is canceled")
var ErrFailedToReceiveListOfObjects = fmt.Errorf("failed to receive the list of objects")
var ErrNoObjectsToImport = fmt.Errorf("source path doesn't contain objects to import")
var ErrLimitExceeded = fmt.Errorf("Limit of relations or objects are exceeded ")
var ErrFileLoad = fmt.Errorf("file was not synced")

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
	return NewFromError(fmt.Errorf("%w: %w", ErrCancel, err), pb.RpcObjectImportRequest_ALL_OR_NOTHING)
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

func (ce *ConvertError) ErrorOrNil() *ConvertError {
	if ce.IsEmpty() {
		return nil
	}
	return ce
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

func (ce *ConvertError) GetResultError(importType model.ImportType) error {
	if ce.IsEmpty() {
		return nil
	}
	var countNoObjectsToImport int
	for _, e := range ce.errors {
		switch {
		case errors.Is(e, ErrCancel):
			return fmt.Errorf("import type: %s: %w", importType.String(), ErrCancel)
		case errors.Is(e, ErrLimitExceeded):
			return fmt.Errorf("import type: %s: %w", importType.String(), ErrLimitExceeded)
		case errors.Is(e, ErrFailedToReceiveListOfObjects):
			return ErrFailedToReceiveListOfObjects
		case errors.Is(e, ErrFileLoad):
			return fmt.Errorf("import type: %s: %w", importType.String(), e)
		case errors.Is(e, list.ErrInsufficientPermissions):
			return e
		case errors.Is(e, ErrNoObjectsToImport):
			countNoObjectsToImport++
		}
	}
	// we return ErrNoObjectsToImport only if all paths has such error, otherwise we assume that import finished with internal code error
	if countNoObjectsToImport == len(ce.errors) {
		return fmt.Errorf("import type: %s: %w", importType.String(), ErrNoObjectsToImport)
	}
	return fmt.Errorf("import type: %s: %w", importType.String(), ce.Error())
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
func (ce *ConvertError) ShouldAbortImport(pathsCount int, importType model.ImportType) bool {
	return !ce.IsEmpty() && ce.mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING ||
		ce.IsNoObjectToImportError(pathsCount) ||
		errors.Is(ce.GetResultError(importType), ErrLimitExceeded) ||
		errors.Is(ce.GetResultError(importType), ErrCancel)
}

func GetImportErrorCode(err error) model.ImportErrorCode {
	if err == nil {
		return model.Import_NULL
	}
	switch {
	case errors.Is(err, ErrNoObjectsToImport):
		return model.Import_NO_OBJECTS_TO_IMPORT
	case errors.Is(err, ErrCancel):
		return model.Import_IMPORT_IS_CANCELED
	case errors.Is(err, ErrLimitExceeded):
		return model.Import_LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED
	case errors.Is(err, ErrFileLoad):
		return model.Import_FILE_LOAD_ERROR
	case errors.Is(err, list.ErrInsufficientPermissions):
		return model.Import_INSUFFICIENT_PERMISSIONS
	default:
		return model.Import_INTERNAL_ERROR
	}
}

func GetGalleryResponseCode(err error) pb.RpcObjectImportExperienceResponseErrorCode {
	if err == nil {
		return pb.RpcObjectImportExperienceResponseError_NULL
	}
	switch {
	case errors.Is(err, list.ErrInsufficientPermissions):
		return pb.RpcObjectImportExperienceResponseError_INSUFFICIENT_PERMISSION
	default:
		return pb.RpcObjectImportExperienceResponseError_UNKNOWN_ERROR
	}
}
