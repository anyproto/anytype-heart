package object

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/cmd/api/pagination"
	"github.com/anyproto/anytype-heart/cmd/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type CreateObjectRequest struct {
	Name                string `json:"name"`
	Icon                string `json:"icon"`
	TemplateId          string `json:"template_id"`
	ObjectTypeUniqueKey string `json:"object_type_unique_key"`
	WithChat            bool   `json:"with_chat"`
}

// GetObjectsHandler retrieves objects in a specific space
//
//	@Summary	Retrieve objects in a specific space
//	@Tags		space_objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		offset		query		int						false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int						false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string][]Object		"List of objects"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects [get]
func GetObjectsHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		resp := s.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value: pbtypes.IntList([]int{
						int(model.ObjectType_basic),
						int(model.ObjectType_profile),
						int(model.ObjectType_todo),
						int(model.ObjectType_note),
						int(model.ObjectType_bookmark),
						int(model.ObjectType_set),
						int(model.ObjectType_collection),
						int(model.ObjectType_participant),
					}...),
				},
			},
			Sorts: []*model.BlockContentDataviewSort{{
				RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
				Type:           model.BlockContentDataviewSort_Desc,
				Format:         model.RelationFormat_longtext,
				IncludeTime:    true,
				EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
			}},
			Keys: []string{"id", "name", "type", "layout", "iconEmoji", "iconImage"},
		})

		if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of objects."})
			return
		}

		if len(resp.Records) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"message": "No objects found."})
			return
		}

		paginatedObjects, hasMore := pagination.Paginate(resp.Records, offset, limit)
		objects := make([]Object, 0, len(paginatedObjects))

		for _, record := range paginatedObjects {
			icon := util.GetIconFromEmojiOrImage(s.AccountInfo, record.Fields["iconEmoji"].GetStringValue(), record.Fields["iconImage"].GetStringValue())
			objectTypeName, err := util.ResolveTypeToName(s.mw, spaceId, record.Fields["type"].GetStringValue())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to resolve object type name."})
				return
			}

			objectShowResp := s.mw.ObjectShow(c.Request.Context(), &pb.RpcObjectShowRequest{
				SpaceId:  spaceId,
				ObjectId: record.Fields["id"].GetStringValue(),
			})

			object := Object{
				// TODO fix type inconsistency
				Type:       model.ObjectTypeLayout_name[int32(record.Fields["layout"].GetNumberValue())],
				Id:         record.Fields["id"].GetStringValue(),
				Name:       record.Fields["name"].GetStringValue(),
				Icon:       icon,
				ObjectType: objectTypeName,
				SpaceId:    spaceId,
				RootId:     objectShowResp.ObjectView.RootId,
				Blocks:     s.GetBlocks(objectShowResp),
				Details:    s.GetDetails(objectShowResp),
			}

			objects = append(objects, object)
		}

		pagination.RespondWithPagination(c, http.StatusOK, objects, len(resp.Records), offset, limit, hasMore)
	}
}

// GetObjectHandler retrieves a specific object in a space
//
//	@Summary	Retrieve a specific object in a space
//	@Tags		space_objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		object_id	path		string					true	"The ID of the object"
//	@Success	200			{object}	Object					"The requested object"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects/{object_id} [get]
func GetObjectHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		resp := s.mw.ObjectShow(c.Request.Context(), &pb.RpcObjectShowRequest{
			SpaceId:  spaceId,
			ObjectId: objectId,
		})

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
				c.JSON(http.StatusNotFound, gin.H{"message": "Object not found", "space_id": spaceId, "object_id": objectId})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve object."})
			return
		}

		objectTypeName, err := util.ResolveTypeToName(s.mw, spaceId, resp.ObjectView.Details[0].Details.Fields["type"].GetStringValue())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to resolve object type name."})
			return
		}

		object := Object{
			Type:       "object",
			Id:         objectId,
			Name:       resp.ObjectView.Details[0].Details.Fields["name"].GetStringValue(),
			Icon:       resp.ObjectView.Details[0].Details.Fields["iconEmoji"].GetStringValue(),
			ObjectType: objectTypeName,
			RootId:     resp.ObjectView.RootId,
			Blocks:     s.GetBlocks(resp),
			Details:    s.GetDetails(resp),
		}

		c.JSON(http.StatusOK, gin.H{"object": object})
	}
}

// CreateObjectHandler creates a new object in a specific space
//
//	@Summary	Create a new object in a specific space
//	@Tags		space_objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		object		body		map[string]string		true	"Object details (e.g., name)"
//	@Success	200			{object}	Object					"The created object"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects [post]
func CreateObjectHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		request := CreateObjectRequest{}
		if err := c.BindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid JSON"})
			return
		}

		resp := s.mw.ObjectCreate(c.Request.Context(), &pb.RpcObjectCreateRequest{
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					"name":      pbtypes.String(request.Name),
					"iconEmoji": pbtypes.String(request.Icon),
				},
			},
			TemplateId:          request.TemplateId,
			SpaceId:             spaceId,
			ObjectTypeUniqueKey: request.ObjectTypeUniqueKey,
			WithChat:            request.WithChat,
		})

		if resp.Error.Code != pb.RpcObjectCreateResponseError_NULL {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to create a new object."})
			return
		}

		object := Object{
			Type:       "object",
			Id:         resp.ObjectId,
			Name:       resp.Details.Fields["name"].GetStringValue(),
			Icon:       resp.Details.Fields["iconEmoji"].GetStringValue(),
			ObjectType: request.ObjectTypeUniqueKey,
			SpaceId:    resp.Details.Fields["spaceId"].GetStringValue(),
			// TODO populate other fields
			// RootId:    resp.RootId,
			// Blocks:    []Block{},
			// Details: []Detail{},
		}

		c.JSON(http.StatusOK, gin.H{"object": object})
	}
}

// UpdateObjectHandler updates an existing object in a specific space
//
//	@Summary	Update an existing object in a specific space
//	@Tags		space_objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		object_id	path		string					true	"The ID of the object"
//	@Param		object		body		Object					true	"The updated object details"
//	@Success	200			{object}	Object					"The updated object"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects/{object_id} [put]
func UpdateObjectHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")
		// TODO: Implement logic to update an existing object
		c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet", "space_id": spaceId, "object_id": objectId})
	}
}

// GetObjectTypesHandler retrieves object types in a specific space
//
//	@Summary	Retrieve object types in a specific space
//	@Tags		types_and_templates
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		offset		query		int						false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int						false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string]ObjectType	"List of object types"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objectTypes [get]
func GetObjectTypesHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		resp := s.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
				},
				{
					RelationKey: bundle.RelationKeyIsHidden.String(),
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.Bool(true),
				},
			},
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: "name",
					Type:        model.BlockContentDataviewSort_Asc,
				},
			},
			Keys: []string{"id", "uniqueKey", "name", "iconEmoji"},
		})

		if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve object types."})
			return
		}

		if len(resp.Records) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"message": "No object types found."})
			return
		}

		paginatedTypes, hasMore := pagination.Paginate(resp.Records, offset, limit)
		objectTypes := make([]ObjectType, 0, len(paginatedTypes))

		for _, record := range paginatedTypes {
			objectTypes = append(objectTypes, ObjectType{
				Type:      "object_type",
				Id:        record.Fields["id"].GetStringValue(),
				UniqueKey: record.Fields["uniqueKey"].GetStringValue(),
				Name:      record.Fields["name"].GetStringValue(),
				Icon:      record.Fields["iconEmoji"].GetStringValue(),
			})
		}

		pagination.RespondWithPagination(c, http.StatusOK, objectTypes, len(resp.Records), offset, limit, hasMore)
	}
}

// GetObjectTypeTemplatesHandler retrieves a list of templates for a specific object type in a space
//
//	@Summary	Retrieve a list of templates for a specific object type in a space
//	@Tags		types_and_templates
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string						true	"The ID of the space"
//	@Param		typeId		path		string						true	"The ID of the object type"
//	@Param		offset		query		int							false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int							false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string][]ObjectTemplate	"List of templates"
//	@Failure	403			{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError			"Resource not found"
//	@Failure	502			{object}	util.ServerError			"Internal server error"
//	@Router		/spaces/{space_id}/objectTypes/{typeId}/templates [get]
func GetObjectTypeTemplatesHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("typeId")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		// First, determine the type ID of "ot-template" in the space
		templateTypeIdResp := s.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyUniqueKey.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("ot-template"),
				},
			},
			Keys: []string{"id"},
		})

		if templateTypeIdResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve template type."})
			return
		}

		if len(templateTypeIdResp.Records) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"message": "Template type not found."})
			return
		}

		templateTypeId := templateTypeIdResp.Records[0].Fields["id"].GetStringValue()

		// Then, search all objects of the template type and filter by the target object type
		templateObjectsResp := s.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyType.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(templateTypeId),
				},
			},
			Keys: []string{"id", "targetObjectType", "name", "iconEmoji"},
		})

		if templateObjectsResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve template objects."})
			return
		}

		if len(templateObjectsResp.Records) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"message": "No templates found."})
			return
		}

		templateIds := make([]string, 0)
		for _, record := range templateObjectsResp.Records {
			if record.Fields["targetObjectType"].GetStringValue() == typeId {
				templateIds = append(templateIds, record.Fields["id"].GetStringValue())
			}
		}

		// Finally, open each template and populate the response
		paginatedTemplates, hasMore := pagination.Paginate(templateIds, offset, limit)
		templates := make([]ObjectTemplate, 0, len(paginatedTemplates))

		for _, templateId := range paginatedTemplates {
			templateResp := s.mw.ObjectShow(c.Request.Context(), &pb.RpcObjectShowRequest{
				SpaceId:  spaceId,
				ObjectId: templateId,
			})

			if templateResp.Error.Code != pb.RpcObjectShowResponseError_NULL {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve template."})
				return
			}

			templates = append(templates, ObjectTemplate{
				Type: "object_template",
				Id:   templateId,
				Name: templateResp.ObjectView.Details[0].Details.Fields["name"].GetStringValue(),
				Icon: templateResp.ObjectView.Details[0].Details.Fields["iconEmoji"].GetStringValue(),
			})
		}

		pagination.RespondWithPagination(c, http.StatusOK, templates, len(templateIds), offset, limit, hasMore)
	}
}
