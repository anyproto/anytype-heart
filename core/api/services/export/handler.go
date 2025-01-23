package export

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/util"
)

// GetObjectExportHandler exports an object in specified format
//
//	@Summary	Export object
//	@Tags		export
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"Space ID"
//	@Param		object_id	path		string					true	"Object ID"
//	@Param		format		path		string					true	"Export format"
//	@Success	200			{object}	ObjectExportResponse	"Object exported successfully"
//	@Failure	400			{object}	util.ValidationError	"Bad request"
//	@Failure	401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	500			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects/{object_id}/export/{format} [post]
func GetObjectExportHandler(s *ExportService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")
		format := c.Query("format")

		objectAsRequest := ObjectExportRequest{}
		if err := c.ShouldBindJSON(&objectAsRequest); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, ErrBadInput.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		outputPath, err := s.GetObjectExport(c.Request.Context(), spaceId, objectId, format, objectAsRequest.Path)
		code := util.MapErrorCode(err, util.ErrToCode(ErrFailedExportObjectAsMarkdown, http.StatusInternalServerError))

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, ObjectExportResponse{Path: outputPath})
	}
}
