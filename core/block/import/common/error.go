package common

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

/*
NOTION_NO_OBJECTS_IN_INTEGRATION= 5;

	NOTION_SERVER_IS_UNAVAILABLE = 13;
	NOTION_RATE_LIMIT_EXCEEDED= 15;

	FILE_IMPORT_NO_OBJECTS_IN_ZIP_ARCHIVE = 16;
	FILE_IMPORT_NO_OBJECTS_IN_DIRECTORY = 9;
	FILE_IMPORT_SOURCE_FILE_OPEN_ERROR = 17;

	MD_WRONG_MARKDOWN_SYNTAX = 10;

	HTML_WRONG_HTML_STRUCTURE = 11;

	PB_NOT_ANYBLOCK_FORMAT = 12;

	CSV_LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED = 7;
*/
var (
	ErrCancel                          = fmt.Errorf("import is canceled")
	ErrFailedToReceiveListOfObjects    = fmt.Errorf("failed to receive the list of objects")
	ErrNoObjectsToImport               = fmt.Errorf("source path doesn't contain objects to import")
	ErrCsvLimitExceeded                = fmt.Errorf("Limit of relations or objects are exceeded ")
	ErrFileLoad                        = fmt.Errorf("file was not synced")
	ErrNoObjectInIntegration           = fmt.Errorf("no objects added to Notion integration")
	ErrNotionServerIsUnavailable       = fmt.Errorf("notion server is unavailable")
	ErrNotionServerExceedRateLimit     = fmt.Errorf("rate limit exceeded")
	ErrFileImportNoObjectsInZipArchive = fmt.Errorf("no objects in zip archive")
	ErrFileImportNoObjectsInDirectory  = fmt.Errorf("no objects in directory")
	ErrFileImportSourceFileOpenError   = fmt.Errorf("failed to open imported file")
	ErrMdWrongMarkdownSyntax           = fmt.Errorf("markdown file has wrong syntax")
	ErrPbNotAnyblockFormat             = fmt.Errorf("file doesn't match Anyblock format ")
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
		case errors.Is(e, ErrCsvLimitExceeded):
			return fmt.Errorf("import type: %s: %w", importType.String(), ErrCsvLimitExceeded)
		case errors.Is(e, ErrFailedToReceiveListOfObjects):
			return ErrFailedToReceiveListOfObjects
		case errors.Is(e, ErrFileLoad):
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
		errors.Is(ce.GetResultError(importType), ErrCsvLimitExceeded) ||
		errors.Is(ce.GetResultError(importType), ErrCancel)
}

func GetNotificationErrorCode(err error) model.NotificationImportCode {
	if err == nil {
		return model.NotificationImport_NULL
	}
	switch {
	case errors.Is(err, ErrNoObjectsToImport):
		return model.NotificationImport_NO_OBJECTS_TO_IMPORT
	case errors.Is(err, ErrCancel):
		return model.NotificationImport_IMPORT_IS_CANCELED
	case errors.Is(err, ErrCsvLimitExceeded):
		return model.NotificationImport_LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED
	case errors.Is(err, ErrFileLoad):
		return model.NotificationImport_FILE_LOAD_ERROR
	default:
		return model.NotificationImport_INTERNAL_ERROR
	}
}
