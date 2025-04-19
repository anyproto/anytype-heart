package export

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/util"
)

// GetObjectExportHandler exports an object in specified format
//
//	@Summary			Export object
//	@Description		This endpoint exports a single object from the specified space into a desired format. The export format is provided as a path parameter (currently supporting "markdown" only). The endpoint calls the export service which converts the object's content into the requested format. It is useful for sharing, or displaying the markdown representation of the objecte externally.
//	@x-ai-description	"Use this endpoint to export an object to markdown format. This is useful when you need to convert an object's content into portable markdown text. The response includes the complete markdown representation of the object that can be shared or used in other applications."
//	@Tags				export
//	@Produce			json
//	@Param				Anytype-Version	header		string					false	"The version of the API to use"	default(2025-03-17)
//	@Param				space_id		path		string					true	"Space ID"
//	@Param				object_id		path		string					true	"Object ID"
//	@Param				format			path		string					true	"Export format"	Enums(markdown)
//	@Success			200				{object}	ObjectExportResponse	"Object exported successfully"
//	@Failure			400				{object}	util.ValidationError	"Bad request"
//	@Failure			401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure			500				{object}	util.ServerError		"Internal server error"
//	@Security			bearerauth
//	@Router				/spaces/{space_id}/objects/{object_id}/{format} [get]
func GetObjectExportHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")
		format := c.Param("format")

		markdown, err := s.GetObjectExport(c.Request.Context(), spaceId, objectId, format)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrInvalidExportFormat, http.StatusInternalServerError))

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, ObjectExportResponse{Markdown: markdown})
	}
}
