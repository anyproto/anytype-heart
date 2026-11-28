package v2handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// PublishObjectHandler publishes the current snapshot of an object.
//
//	@Summary		Publish an object snapshot to the web
//	@Description	Anyone with the URL can view the page, files and linked object metadata. Republish to show edits. Hosted limits are 10 MiB by default or 100 MiB with a registered Any Name, including files and compressed page data.
//	@Id				publish_object
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string					true	"Space id, from GET /v2/spaces"
//	@Param			object_id		path		string					true	"Object id, from GET /v2/spaces/{space_id}/objects"
//	@Param			dry_run			query		bool					false	"Check the slug and object without exporting or uploading. Size and URL availability are checked when publishing."
//	@Param			Idempotency-Key	header		string					false	"Replay the result of the same request"
//	@Param			body			body		v2model.PublishRequest	true	"Page address and optional space invite"
//	@Success		200				{object}	v2model.PublishResult	"Public URL after publishing, or a dry-run receipt"
//	@Failure		400				{object}	v2model.Error			"Invalid publishing request"
//	@Failure		413				{object}	v2model.Error			"The publication exceeds the account limit or the request body exceeds 1 MiB"
//	@Failure		500				{object}	v2model.Error			"Publishing failed"
//
//	@Failure		501				{object}	v2model.Error			"Publishing unavailable"
//
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects/{object_id}/publish [post]
func PublishObjectHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.PublishRequest
		if !decodeStrictJSONBody(c, &req, v2model.Plain("provide a slug and optionally include_invite"), maxV2StructuredBodySize, "publish") {
			return
		}
		result, err := s.PublishObject(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// ListPublicationsHandler lists this account's published pages in a space.
//
//	@Summary		List published pages
//	@Description	Lists this account's published pages in the space with their URLs, sizes and invite status.
//	@Id				list_publications
//	@Tags			Objects
//	@Produce		json
//	@Param			space_id	path		string										true	"Space id, from GET /v2/spaces"
//	@Param			offset		query		int											false	"Items to skip"		default(0)
//	@Param			limit		query		int											false	"Items to return"	default(25)
//	@Success		200			{object}	v2model.ListResponse[v2model.Publication]	"Published pages"
//	@Failure		500			{object}	v2model.Error								"Listing failed"
//	@Failure		501			{object}	v2model.Error								"Publishing unavailable"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/publications [get]
func ListPublicationsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := s.ListPublications(c.Request.Context(), c.Param("space_id"), c.GetInt(pagination.QueryParamOffset), c.GetInt(pagination.QueryParamLimit))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, list)
	}
}

// RemovePublicationHandler takes a public snapshot offline.
//
//	@Summary		Remove a published page
//	@Description	Takes the public copy offline, even if the source object was deleted.
//	@Id				remove_publication
//	@Tags			Objects
//	@Produce		json
//	@Param			space_id		path		string							true	"Space id, from GET /v2/spaces"
//	@Param			publication_id	path		string							true	"Published object_id, from GET /v2/spaces/{space_id}/publications"
//	@Param			dry_run			query		bool							false	"Check that the public copy exists without removing it"
//	@Param			Idempotency-Key	header		string							false	"Replay the result of the same request"
//	@Success		200				{object}	v2model.RemovePublicationResult	"Removed publication or dry-run receipt"
//	@Failure		404				{object}	v2model.Error					"Publication or space not found"
//	@Failure		500				{object}	v2model.Error					"Removal failed"
//	@Failure		501				{object}	v2model.Error					"Publishing unavailable"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/publications/{publication_id} [delete]
func RemovePublicationHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.RemovePublication(c.Request.Context(), c.Param("space_id"), c.Param("publication_id"), isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
