package common

import (
	"errors"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestConvertError(t *testing.T) {
	t.Run("new error from existing", func(t *testing.T) {
		// given
		err := ErrCancel

		// when
		ce := NewFromError(err, pb.RpcObjectImportRequest_IGNORE_ERRORS)

		// then
		assert.Len(t, ce.errors, 1)
		assert.Equal(t, err, ce.errors[0])
	})
	t.Run("add", func(t *testing.T) {
		// given
		ce := NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS)
		err := fmt.Errorf("another error")

		// when
		ce.Add(err)

		// then
		assert.Len(t, ce.errors, 1)
		assert.Equal(t, err, ce.errors[0])
	})
	t.Run("merge", func(t *testing.T) {
		// given
		ce1 := NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS)
		ce1.Add(fmt.Errorf("error1"))

		ce2 := NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)
		ce2.Add(fmt.Errorf("error2"))

		// when
		ce1.Merge(ce2)

		// then
		assert.Len(t, ce1.errors, 2)
		assert.Equal(t, "error1", ce1.errors[0].Error())
		assert.Equal(t, "error2", ce1.errors[1].Error())
	})
	t.Run("ShouldAbortImport", func(t *testing.T) {
		// given
		ce := NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

		// when
		ce.Add(ErrCancel)

		// then
		assert.True(t, ce.ShouldAbortImport(1, model.Import_Notion))
	})
}

func TestGetImportNotificationErrorCode(t *testing.T) {
	t.Run("GetImportNotificationErrorCode - NOTION_NO_OBJECTS_IN_INTEGRATION", func(t *testing.T) {
		// given
		err := ErrNoObjectInIntegration

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_NOTION_NO_OBJECTS_IN_INTEGRATION, code)
	})
	t.Run("GetImportNotificationErrorCode - IMPORT_IS_CANCELED", func(t *testing.T) {
		// given
		err := ErrCancel

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_IMPORT_IS_CANCELED, code)
	})
	t.Run("GetImportNotificationErrorCode - INTERNAL_ERROR", func(t *testing.T) {
		// given
		err := errors.New("some random error")

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_INTERNAL_ERROR, code)
	})
	t.Run("GetImportNotificationErrorCode - nil", func(t *testing.T) {
		// given
		var err error

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_NULL, code)
	})
	t.Run("GetImportNotificationErrorCode - notion server is unavailable", func(t *testing.T) {
		// given
		err := ErrNotionServerIsUnavailable

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_NOTION_SERVER_IS_UNAVAILABLE, code)
	})
	t.Run("GetImportNotificationErrorCode - notion server exceeded limit", func(t *testing.T) {
		// given
		err := ErrNotionServerExceedRateLimit

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_NOTION_RATE_LIMIT_EXCEEDED, code)
	})
	t.Run("GetImportNotificationErrorCode - no objects in dir", func(t *testing.T) {
		// given
		err := ErrFileImportNoObjectsInDirectory

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_FILE_IMPORT_NO_OBJECTS_IN_DIRECTORY, code)
	})
	t.Run("GetImportNotificationErrorCode - no objects in zip", func(t *testing.T) {
		// given
		err := ErrFileImportNoObjectsInZipArchive

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_FILE_IMPORT_NO_OBJECTS_IN_ZIP_ARCHIVE, code)
	})
	t.Run("GetImportNotificationErrorCode - not any block format", func(t *testing.T) {
		// given
		err := ErrPbNotAnyBlockFormat

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_PB_NOT_ANYBLOCK_FORMAT, code)
	})
	t.Run("GetImportNotificationErrorCode - csv limit exceeded", func(t *testing.T) {
		// given
		err := ErrCsvLimitExceeded

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_CSV_LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED, code)
	})
	t.Run("GetImportNotificationErrorCode - wrong html", func(t *testing.T) {
		// given
		err := ErrWrongHTMLFormat

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_HTML_WRONG_HTML_STRUCTURE, code)
	})
	t.Run("GetImportNotificationErrorCode - insufficient permissions", func(t *testing.T) {
		// given
		err := list.ErrInsufficientPermissions

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_INSUFFICIENT_PERMISSIONS, code)
	})
	t.Run("GetImportNotificationErrorCode - file load error", func(t *testing.T) {
		// given
		err := ErrFileLoad

		// when
		code := GetImportNotificationErrorCode(err)

		// then
		assert.Equal(t, model.Import_FILE_LOAD_ERROR, code)
	})
}

func TestError(t *testing.T) {
	t.Run("error is not empty", func(t *testing.T) {
		// given
		ce := NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS)
		err1 := fmt.Errorf("error 1")
		err2 := fmt.Errorf("error 2")
		ce.Add(err1)
		ce.Add(err2)

		// when
		actual := ce.Error().Error()

		// then
		expected := "error: error 1\nerror: error 2\n"
		assert.Equal(t, expected, actual)
	})
	t.Run("error is empty", func(t *testing.T) {
		// given
		ce := NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS)

		// when
		actual := ce.Error()

		// then
		assert.Nil(t, actual)
	})
}

func TestConvertError_GetResultError(t *testing.T) {
	t.Run("get result error", func(t *testing.T) {
		// given
		ce := NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS)

		// when
		ce.Add(ErrCancel)

		// then
		result := ce.GetResultError(model.Import_Notion)
		assert.ErrorIs(t, result, ErrCancel)
		assert.EqualError(t, result, "import type: Notion: import is canceled")
	})
	t.Run("get result error - error is empty", func(t *testing.T) {
		// given
		ce := NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS)

		// when
		result := ce.GetResultError(model.Import_Notion)

		// then
		assert.Nil(t, result)
	})
	t.Run("get result error - no object error", func(t *testing.T) {
		// given
		ce := NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS)

		// when
		ce.Add(ErrFileImportNoObjectsInDirectory)
		ce.Add(ErrFileImportNoObjectsInZipArchive)
		result := ce.GetResultError(model.Import_Notion)

		// then
		assert.NotNil(t, result)
	})
}

func TestIsNoObjectError(t *testing.T) {
	t.Run("IsNoObjectError - random error", func(t *testing.T) {
		// given
		err := errors.New("some random error")

		// when
		result := IsNoObjectError(err)

		// then
		assert.False(t, result)
	})
	t.Run("IsNoObjectToImportError", func(t *testing.T) {
		// given
		ce := NewError(pb.RpcObjectImportRequest_ALL_OR_NOTHING)

		// when
		ce.Add(ErrFileImportNoObjectsInDirectory)
		ce.Add(ErrFileImportNoObjectsInZipArchive)

		// then
		assert.True(t, ce.IsNoObjectToImportError(2))
	})
}

func TestGetGalleryResponseCode(t *testing.T) {
	t.Run("GetGalleryResponseCode - no error", func(t *testing.T) {
		// given
		var err error

		// when
		code := GetGalleryResponseCode(err)

		// then
		assert.Equal(t, pb.RpcObjectImportExperienceResponseError_NULL, code)
	})
	t.Run("GetGalleryResponseCode - internal error", func(t *testing.T) {
		// given
		err := ErrCancel

		// when
		code := GetGalleryResponseCode(err)

		// then
		assert.Equal(t, pb.RpcObjectImportExperienceResponseError_UNKNOWN_ERROR, code)
	})
	t.Run("GetGalleryResponseCode - insufficient permission error", func(t *testing.T) {
		// given
		err := list.ErrInsufficientPermissions

		// when
		code := GetGalleryResponseCode(err)

		// then
		assert.Equal(t, pb.RpcObjectImportExperienceResponseError_INSUFFICIENT_PERMISSION, code)
	})
}

func TestGetNoObjectErrorBySourceType(t *testing.T) {
	t.Run("source is directory", func(t *testing.T) {
		// given
		dirSource := &source.Directory{}

		// when
		err := GetNoObjectErrorBySourceType(dirSource)

		// then
		assert.ErrorIs(t, err, ErrFileImportNoObjectsInDirectory)
	})
	t.Run("source is zip", func(t *testing.T) {
		// given
		zipSource := &source.Zip{}

		// when
		err := GetNoObjectErrorBySourceType(zipSource)

		// then
		assert.ErrorIs(t, err, ErrFileImportNoObjectsInZipArchive)
	})
	t.Run("source is nil", func(t *testing.T) {
		// when
		err := GetNoObjectErrorBySourceType(nil)

		// then
		assert.Nil(t, err)
	})
}
