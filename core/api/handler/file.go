package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// UploadFileHandler uploads a file to Anytype
//
//	@Summary		Upload file
//	@Description	Uploads a file to the specified space. The file is uploaded via multipart/form-data. Returns the object ID which can be used as an attachment in chat messages or other objects. Rate limited.
//	@Id				upload_file
//	@Tags			Files
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string						true	"The ID of the space"
//	@Param			file			formData	file						true	"The file to upload"
//	@Param			type			formData	string						false	"The type of file (file, image, video, audio, pdf). If not specified, the type is auto-detected."	Enums(file, image, video, audio, pdf)
//	@Success		201				{object}	apimodel.FileUploadResponse	"The uploaded file"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		429				{object}	util.RateLimitError			"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/files [post]
func UploadFileHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		// Get the uploaded file
		file, err := c.FormFile("file")
		if err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, "file is required")
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		// Create temp file with original extension preserved
		ext := filepath.Ext(file.Filename)
		tempFile, err := os.CreateTemp("", "anytype-upload-*"+ext)
		if err != nil {
			apiErr := util.CodeToApiError(http.StatusInternalServerError, "failed to create temp file")
			c.JSON(http.StatusInternalServerError, apiErr)
			return
		}
		tempPath := tempFile.Name()
		tempFile.Close()

		// Ensure temp file is cleaned up
		defer os.Remove(tempPath)

		// Save uploaded file to temp location
		if err := c.SaveUploadedFile(file, tempPath); err != nil {
			apiErr := util.CodeToApiError(http.StatusInternalServerError, "failed to save uploaded file")
			c.JSON(http.StatusInternalServerError, apiErr)
			return
		}

		// Get optional file type
		fileType := apimodel.FileType(c.PostForm("type"))

		// Upload to Anytype
		objectId, err := s.UploadFile(c.Request.Context(), spaceId, tempPath, fileType)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrInvalidFile, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedUploadFile, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.FileUploadResponse{
			Object:   "file",
			ObjectId: objectId,
		})
	}
}
