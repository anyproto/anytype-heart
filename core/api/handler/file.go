package handler

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// DownloadFileHandler streams a file's bytes back to the caller. For image
// files, an optional `width` query parameter selects a pre-rendered variant.
//
//	@Summary		Download file
//	@Description	Streams the bytes of a file object. The response Content-Type matches the stored media type. For images, pass `width` to fetch a pre-rendered variant at that pixel width; SVGs are sanitized inline. `width` is ignored for non-image files. The id can be either a file object ID or, for images, a raw file CID.
//	@Id				download_file
//	@Tags			Files
//	@Produce		application/octet-stream
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space the file belongs to"
//	@Param			file_id			path		string					true	"The file object ID (or raw file CID for images)"
//	@Param			width			query		int						false	"Optional pixel width for image variants; ignored on non-images"
//	@Success		200				{file}		binary					"File contents"
//	@Failure		400				{object}	util.ValidationError	"Invalid query parameters"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"File not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/files/{file_id} [get]
func DownloadFileHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		fileId := c.Param("file_id")

		width := 0
		if w := c.Query("width"); w != "" {
			parsed, err := strconv.Atoi(w)
			if err != nil || parsed < 0 {
				apiErr := util.CodeToApiError(http.StatusBadRequest, "invalid width parameter")
				c.JSON(http.StatusBadRequest, apiErr)
				return
			}
			width = parsed
		}

		content, err := s.GetFileContent(c.Request.Context(), fileId, width)
		if err != nil {
			writeFileError(c, err)
			return
		}

		serveContent(c, content)
	}
}

// serveContent writes a FileContent to the response using http.ServeContent so
// that range requests, conditional GETs and content sniffing all work correctly.
func serveContent(c *gin.Context, content *service.FileContent) {
	if content.MimeType != "" {
		c.Header("Content-Type", content.MimeType)
	}
	c.Header("Cache-Control", "max-age=31536000, private")

	modTime := time.Time{}
	if content.ModTime > 0 {
		modTime = time.Unix(content.ModTime, 0)
	}
	http.ServeContent(c.Writer, c.Request, content.Name, modTime, content.Reader)
}

// writeFileError maps a service-layer file error to an HTTP response.
func writeFileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrFileNotFound):
		apiErr := util.CodeToApiError(http.StatusNotFound, err.Error())
		c.JSON(http.StatusNotFound, apiErr)
	default:
		apiErr := util.CodeToApiError(http.StatusInternalServerError, err.Error())
		c.JSON(http.StatusInternalServerError, apiErr)
	}
}

// DeleteFileHandler removes a file object.
//
//	@Summary		Delete file
//	@Description	Removes a file object. By default the file is moved to the bin and can be restored. Pass `skip_bin=true` to permanently delete it instead.
//	@Id				delete_file
//	@Tags			Files
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space the file belongs to"
//	@Param			file_id			path		string					true	"The file object ID"
//	@Param			skip_bin		query		bool					false	"When true, permanently delete instead of moving to bin"	default(false)
//	@Success		204				{string}	string					"File deleted"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/files/{file_id} [delete]
func DeleteFileHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		fileId := c.Param("file_id")

		skipBin := false
		if v := c.Query("skip_bin"); v != "" {
			parsed, err := strconv.ParseBool(v)
			if err != nil {
				apiErr := util.CodeToApiError(http.StatusBadRequest, "invalid skip_bin parameter")
				c.JSON(http.StatusBadRequest, apiErr)
				return
			}
			skipBin = parsed
		}

		err := s.DeleteFile(c.Request.Context(), spaceId, fileId, skipBin)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedDeleteFile, http.StatusInternalServerError),
		)
		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.Status(http.StatusNoContent)
	}
}

// UploadFileHandler handles file uploads
//
//	@Summary		Upload file
//	@Description	Uploads a file to the specified space. Accepts multipart/form-data with a file field. The file is processed and stored, then a file object is created. Returns the file object ID along with its name, MIME type and size.
//	@Id				upload_file
//	@Tags			Files
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string						true	"The ID of the space to upload the file to"
//	@Param			file			formData	file						true	"The file to upload"
//	@Success		200				{object}	apimodel.FileUploadResponse	"File uploaded successfully"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError			"Forbidden — read-only space or no permission"
//	@Failure		404				{object}	util.NotFoundError			"Space not found"
//	@Failure		410				{object}	util.GoneError				"Space was deleted"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/files [post]
func UploadFileHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		// Get uploaded file from multipart form
		fileHeader, err := c.FormFile("file")
		if err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, "missing file in request")
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		// Open uploaded file
		file, err := fileHeader.Open()
		if err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, "failed to read uploaded file")
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}
		defer file.Close()

		// Stage the upload inside a private temp directory so the upload
		// pipeline (which derives the file name from filepath.Base of the
		// path) sees the caller-supplied filename instead of a temp prefix.
		tempDir, err := os.MkdirTemp("", "anytype-upload-")
		if err != nil {
			apiErr := util.CodeToApiError(http.StatusInternalServerError, "failed to create temp dir")
			c.JSON(http.StatusInternalServerError, apiErr)
			return
		}
		defer os.RemoveAll(tempDir)

		// Sanitize: strip any path components so we can't escape tempDir.
		uploadedName := filepath.Base(fileHeader.Filename)
		if uploadedName == "." || uploadedName == ".." || uploadedName == "" || strings.ContainsRune(uploadedName, 0) {
			uploadedName = "upload"
		}
		tempPath := filepath.Join(tempDir, uploadedName)

		tempFile, err := os.Create(tempPath)
		if err != nil {
			apiErr := util.CodeToApiError(http.StatusInternalServerError, "failed to create temp file")
			c.JSON(http.StatusInternalServerError, apiErr)
			return
		}

		// Copy uploaded file to temp file
		_, err = io.Copy(tempFile, file)
		tempFile.Close()
		if err != nil {
			apiErr := util.CodeToApiError(http.StatusInternalServerError, "failed to save uploaded file")
			c.JSON(http.StatusInternalServerError, apiErr)
			return
		}

		// Upload via service
		result, err := s.UploadFile(c.Request.Context(), spaceId, tempPath)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrSpaceNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrSpaceDeleted, http.StatusGone),
			util.ErrToCode(service.ErrForbidden, http.StatusForbidden),
			util.ErrToCode(service.ErrFailedUploadFile, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}
