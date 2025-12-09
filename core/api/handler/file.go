package handler

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// UploadFileHandler handles file uploads
//
//	@Summary		Upload file
//	@Description	Uploads a file to the specified space. Accepts multipart/form-data with a file field. The file is processed and stored, then a file object is created. Returns the file object ID and file ID (IPFS CID).
//	@Id				upload_file
//	@Tags			Files
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string							true	"The ID of the space to upload the file to"
//	@Param			file			formData	file							true	"The file to upload"
//	@Success		200				{object}	apimodel.FileUploadResponse		"File uploaded successfully"
//	@Failure		400				{object}	util.BadRequestError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/files [post]
func UploadFileHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		// Get uploaded file from multipart form
		fileHeader, err := c.FormFile("file")
		if err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, "missing file in request")
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		// Open uploaded file
		file, err := fileHeader.Open()
		if err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, "failed to read uploaded file")
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}
		defer file.Close()

		// Create temp file
		tempFile, err := os.CreateTemp("", "anytype-upload-*-"+filepath.Base(fileHeader.Filename))
		if err != nil {
			apiErr := util.CodeToAPIError(http.StatusInternalServerError, "failed to create temp file")
			c.JSON(http.StatusInternalServerError, apiErr)
			return
		}
		tempPath := tempFile.Name()
		defer os.Remove(tempPath) // cleanup

		// Copy uploaded file to temp file
		_, err = io.Copy(tempFile, file)
		tempFile.Close()
		if err != nil {
			apiErr := util.CodeToAPIError(http.StatusInternalServerError, "failed to save uploaded file")
			c.JSON(http.StatusInternalServerError, apiErr)
			return
		}

		// Upload via service
		result, err := s.UploadFile(c.Request.Context(), spaceId, tempPath)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedUploadFile, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}
