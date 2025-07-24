package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ListFilesHandler retrieves a list of file objects in a space
//
//	@Summary		List files
//	@Description	Retrieves a paginated list of file objects (images, videos, audio, PDFs, and generic files) in the given space. The response includes file metadata such as ID, name, space ID, layout (file or image), associated type, properties, and a URL for direct content access via the gateway. The endpoint supports pagination and name filtering. This endpoint is useful for building file galleries, media browsers, or document management interfaces.
//	@Id				list_files
//	@Tags			Files
//	@Produce		json
//	@Param			Anytype-Version	header		string										true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string										true	"The ID of the space in which to list files; must be retrieved from ListSpaces endpoint"
//	@Param			name			query		string										false	"Filter files by name (partial match)"
//	@Param			offset			query		int											false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int											false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.File]	"The list of files in the specified space"
//	@Failure		401				{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure		500				{object}	util.ServerError							"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/files [get]
func ListFilesHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		files, total, hasMore, err := s.ListFiles(c.Request.Context(), spaceId, filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedListFiles, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, files, total, offset, limit, hasMore)
	}
}

// GetFileHandler retrieves a file object in a space
//
//	@Summary		Get file
//	@Description	Fetches the full details of a single file object identified by the file ID within the specified space. The response includes the file's ID, name, space ID, layout (file or image), associated type object, properties with their values, and a URL for direct content access via the gateway. This endpoint is essential when a client needs to display detailed file information or access the file content.
//	@Id				get_file
//	@Tags			Files
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space in which the file exists; must be retrieved from ListSpaces endpoint"
//	@Param			file_id			path		string					true	"The ID of the file to retrieve; must be retrieved from ListFiles endpoint or obtained from response context"
//	@Success		200				{object}	apimodel.FileResponse	"The retrieved file"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/files/{file_id} [get]
func GetFileHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		fileId := c.Param("file_id")

		file, err := s.GetFile(c.Request.Context(), spaceId, fileId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFileNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedGetFile, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.FileResponse{File: file})
	}
}
