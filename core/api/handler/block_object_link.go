package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// SetBlockObjectLinkHandler sets the object link on a block (text→link or link target update) via the editor replace path.
//
//	@Summary		Set block object link
//	@Description	Sets or updates a UI-style object link on the given block: a text block becomes a link block (card layout); a link block gets a new target and card layout. Uses the same internal BlockReplace path as the editor (links/backlinks follow normal derivation). Re-posting the same target upgrades Text preview links to Card when needed.
//	@Id				set_block_object_link
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string								true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string								true	"Space id"
//	@Param			object_id		path		string								true	"Source object id (page/note containing the block)"
//	@Param			block_id		path		string								true	"Block id to turn into / update as object link"
//	@Param			body			body		apimodel.SetBlockObjectLinkRequest	true	"Target object id"
//	@Success		200				{object}	apimodel.SetBlockObjectLinkResponse	"Link set (or already matched target)"
//	@Failure		400				{object}	util.ValidationError				"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError					"Object or block not found"
//	@Failure		410				{object}	util.GoneError						"Object deleted"
//	@Failure		429				{object}	util.RateLimitError					"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError					"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/objects/{object_id}/blocks/{block_id}/link [post]
func SetBlockObjectLinkHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")
		blockId := c.Param("block_id")

		var req apimodel.SetBlockObjectLinkRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, util.CodeToApiError(http.StatusBadRequest, err.Error()))
			return
		}

		out, err := s.SetBlockObjectLink(c.Request.Context(), spaceId, objectId, blockId, req)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrObjectDeleted, http.StatusGone),
			util.ErrToCode(service.ErrBlockNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrRequiredBlock, http.StatusBadRequest),
			util.ErrToCode(service.ErrUnsupportedBlockForLink, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedRetrieveObject, http.StatusInternalServerError),
			util.ErrToCode(service.ErrBlockReplaceFailed, http.StatusInternalServerError),
		)
		if code != http.StatusOK {
			c.JSON(code, util.CodeToApiError(code, err.Error()))
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

// DeleteBlockObjectLinkHandler deletes a link block (optional ?target_object_id= must match).
//
//	@Summary		Delete block object link
//	@Description	Removes a link block from the object. When target_object_id is provided, the link must point to that object or the request fails.
//	@Id				delete_block_object_link
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version		header	string	true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id			path	string	true	"Space id"
//	@Param			object_id			path	string	true	"Source object id"
//	@Param			block_id			path	string	true	"Link block id"
//	@Param			target_object_id	query	string	false	"If set, must equal the link target"
//	@Success		204					"Deleted"
//	@Failure		400					{object}	util.ValidationError	"Bad request"
//	@Failure		401					{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404					{object}	util.NotFoundError		"Object, block, or link mismatch"
//	@Failure		410					{object}	util.GoneError			"Object deleted"
//	@Failure		429					{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500					{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/objects/{object_id}/blocks/{block_id}/link [delete]
func DeleteBlockObjectLinkHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")
		blockId := c.Param("block_id")
		optionalTarget := c.Query("target_object_id")

		err := s.DeleteBlockObjectLink(c.Request.Context(), spaceId, objectId, blockId, optionalTarget)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrObjectDeleted, http.StatusGone),
			util.ErrToCode(service.ErrBlockNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrNotLinkBlock, http.StatusNotFound),
			util.ErrToCode(service.ErrTargetMismatch, http.StatusNotFound),
			util.ErrToCode(service.ErrRequiredBlock, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedRetrieveObject, http.StatusInternalServerError),
			util.ErrToCode(service.ErrBlockDeleteFailed, http.StatusInternalServerError),
		)
		if code != http.StatusOK {
			c.JSON(code, util.CodeToApiError(code, err.Error()))
			return
		}
		c.Status(http.StatusNoContent)
	}
}
