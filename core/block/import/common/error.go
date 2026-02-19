package common

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"

	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrCancel           = errors.New("import is canceled")
	ErrCsvLimitExceeded = errors.New("Limit of relations or objects are exceeded ")
	ErrFileLoad         = errors.New("file was not synced")

	ErrNoObjectInIntegration       = errors.New("no objects added to Notion integration")
	ErrNotionServerIsUnavailable   = errors.New("notion server is unavailable")
	ErrNotionServerExceedRateLimit = errors.New("rate limit exceeded")

	ErrFileImportNoObjectsInZipArchive = errors.New("no objects in zip archive")
	ErrFileImportNoObjectsInDirectory  = errors.New("no objects in directory")

	ErrPbNotAnyBlockFormat = errors.New("file doesn't match Anyblock format ")

	ErrWrongHTMLFormat = errors.New("html file has wrong structure")

	ErrNoSnapshotToImport = errors.New("no snapshot to import") // for external import
)

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
	return fmt.Errorf("%s", errorString.String())
}

func (ce *ConvertError) GetResultError(importType model.ImportType) error {
	if ce.IsEmpty() {
		return nil
	}
	var countNoObjectsToImport int
	for _, e := range ce.errors {
		switch {
		case isDefinedError(e):
			return fmt.Errorf("import type: %s: %w", importType.String(), e)
		case IsNoObjectError(e):
			countNoObjectsToImport++
		}
	}
	// we return ErrNoObjectsToImport only if all paths has such error, otherwise we assume that import finished with internal code error
	if countNoObjectsToImport == len(ce.errors) {
		return fmt.Errorf("import type: %s: %w", importType.String(), ce.errors[0])
	}
	return fmt.Errorf("import type: %s: %w", importType.String(), ce.Error())
}

func (ce *ConvertError) IsNoObjectToImportError(importPathsCount int) bool {
	if importPathsCount == 0 {
		return false
	}
	var countNoObjectsToImport int
	for _, err := range ce.errors {
		if IsNoObjectError(err) {
			countNoObjectsToImport++
		}
	}
	return importPathsCount == countNoObjectsToImport
}
func (ce *ConvertError) ShouldAbortImport(pathsCount int, importType model.ImportType) bool {
	return !ce.IsEmpty() && ce.mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING ||
		ce.IsNoObjectToImportError(pathsCount) ||
		errors.Is(ce.GetResultError(importType), ErrCsvLimitExceeded) ||
		errors.Is(ce.GetResultError(importType), ErrCancel)
}

func GetImportNotificationErrorCode(err error) model.ImportErrorCode {
	if err == nil {
		return model.Import_NULL
	}
	switch {
	case errors.Is(err, ErrNoObjectInIntegration):
		return model.Import_NOTION_NO_OBJECTS_IN_INTEGRATION
	case errors.Is(err, ErrNotionServerIsUnavailable):
		return model.Import_NOTION_SERVER_IS_UNAVAILABLE
	case errors.Is(err, ErrNotionServerExceedRateLimit):
		return model.Import_NOTION_RATE_LIMIT_EXCEEDED
	case errors.Is(err, ErrFileImportNoObjectsInDirectory):
		return model.Import_FILE_IMPORT_NO_OBJECTS_IN_DIRECTORY
	case errors.Is(err, ErrFileImportNoObjectsInZipArchive):
		return model.Import_FILE_IMPORT_NO_OBJECTS_IN_ZIP_ARCHIVE
	case errors.Is(err, ErrPbNotAnyBlockFormat):
		return model.Import_PB_NOT_ANYBLOCK_FORMAT
	case errors.Is(err, ErrCancel):
		return model.Import_IMPORT_IS_CANCELED
	case errors.Is(err, ErrCsvLimitExceeded):
		return model.Import_CSV_LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED
	case errors.Is(err, ErrFileLoad):
		return model.Import_FILE_LOAD_ERROR
	case errors.Is(err, ErrWrongHTMLFormat):
		return model.Import_HTML_WRONG_HTML_STRUCTURE
	case errors.Is(err, list.ErrInsufficientPermissions):
		return model.Import_INSUFFICIENT_PERMISSIONS
	default:
		return model.Import_INTERNAL_ERROR
	}
}

func ErrorBySourceType(s source.Source) error {
	if _, ok := s.(*source.Directory); ok {
		return ErrFileImportNoObjectsInDirectory
	}
	if _, ok := s.(*source.Zip); ok {
		return ErrFileImportNoObjectsInZipArchive
	}
	return nil
}

func IsNoObjectError(err error) bool {
	return errors.Is(err, ErrNoObjectInIntegration) ||
		errors.Is(err, ErrFileImportNoObjectsInDirectory) ||
		errors.Is(err, ErrFileImportNoObjectsInZipArchive)
}

func isDefinedError(err error) bool {
	return errors.Is(err, ErrCancel) || errors.Is(err, ErrCsvLimitExceeded) || errors.Is(err, ErrNotionServerExceedRateLimit) ||
		errors.Is(err, ErrNotionServerIsUnavailable) || errors.Is(err, ErrFileLoad) || errors.Is(err, ErrPbNotAnyBlockFormat) ||
		errors.Is(err, ErrWrongHTMLFormat)
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
