package api

import (
	"context"
	"crypto/rand"
	"math/big"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type CreateSpaceRequest struct {
	Name string `json:"name"`
}

type CreateObjectRequest struct {
	Name                string `json:"name"`
	IconEmoji           string `json:"icon_emoji"`
	TemplateID          string `json:"template_id"`
	ObjectTypeUniqueKey string `json:"object_type_unique_key"`
	WithChat            bool   `json:"with_chat"`
}

// authdisplayCodeHandler generates a new challenge and returns the challenge ID
//
//	@Summary	Open a modal window with a code in Anytype Desktop app
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Success	200	{string}	string		"Success"
//	@Failure	502	{object}	ServerError	"Internal server error"
//	@Router		/auth/displayCode [post]
func (a *ApiServer) authDisplayCodeHandler(c *gin.Context) {
	// Call AccountLocalLinkNewChallenge to display code modal
	ctx := context.Background()
	resp := a.mw.AccountLocalLinkNewChallenge(ctx, &pb.RpcAccountLocalLinkNewChallengeRequest{AppName: "api-test"})

	if resp.Error.Code != pb.RpcAccountLocalLinkNewChallengeResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to generate a new challenge."})
	}

	c.JSON(http.StatusOK, gin.H{"challengeId": resp.ChallengeId})
}

// authTokenHandler retrieves an authentication token using a code and challenge ID
//
//	@Summary			Retrieve an authentication token using a code
//	@Tags				auth
//	@Accept				json
//	@Produce			json
//	@Param				code	query		string				true	"The code retrieved from Anytype Desktop app"
//	@ParamchallengeId	query																																																																																								string								true	"The challenge ID"
//	@Success			200		{object}	map[string]string	"Access and refresh tokens"
//	@Failure			400		{object}	ValidationError		"Invalid input"
//	@Failure			502		{object}	ServerError			"Internal server error"
//	@Router				/auth/token [get]
func (a *ApiServer) authTokenHandler(c *gin.Context) {
	// Call AccountLocalLinkSolveChallenge to retrieve session token and app key
	resp := a.mw.AccountLocalLinkSolveChallenge(context.Background(), &pb.RpcAccountLocalLinkSolveChallengeRequest{
		ChallengeId: c.Query("challengeId"),
		Answer:      c.Query("code"),
	})

	if resp.Error.Code != pb.RpcAccountLocalLinkSolveChallengeResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to authenticate user."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"sessionToken": resp.SessionToken,
		"appKey":       resp.AppKey,
	})
}

// getSpacesHandler retrieves a list of spaces
//
//	@Summary	Retrieve a list of spaces
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		offset	query		int					false	"The number of items to skip before starting to collect the result set"
//	@Param		limit	query		int					false	"The number of items to return"	default(100)
//	@Success	200		{object}	map[string][]Space	"List of spaces"
//	@Failure	403		{object}	UnauthorizedError	"Unauthorized"
//	@Failure	502		{object}	ServerError			"Internal server error"
//	@Router		/spaces [get]
func (a *ApiServer) getSpacesHandler(c *gin.Context) {
	// Call ObjectSearch for all objects of type spaceView
	resp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: a.accountInfo.TechSpaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
			},
		},
	})
	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of spaces."})
		return
	}

	spaces := make([]Space, 0, len(resp.Records))
	for _, record := range resp.Records {
		spaceId := record.Fields["targetSpaceId"].GetStringValue()
		workspaceResponse := a.mw.WorkspaceOpen(context.Background(), &pb.RpcWorkspaceOpenRequest{
			SpaceId:  spaceId,
			WithChat: true,
		})

		if workspaceResponse.Error.Code != pb.RpcWorkspaceOpenResponseError_NULL {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to open workspace."})
			return
		}

		// TODO cleanup image logic
		// Convert space image or option to base64 string
		var iconBase64 string
		iconImageId := record.Fields["iconImage"].GetStringValue()
		if iconImageId != "" {
			b64, err2 := a.imageToBase64(a.getGatewayURL(iconImageId))
			if err2 != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to convert image to base64."})
				return
			}
			iconBase64 = b64
		} else {
			iconOption := record.Fields["iconOption"].GetNumberValue()
			// TODO figure out size
			iconBase64 = a.spaceSvg(int(iconOption), 100, string([]rune(record.Fields["name"].GetStringValue())[0]))
		}

		space := Space{
			Type:                   "space",
			ID:                     spaceId,
			Name:                   record.Fields["name"].GetStringValue(),
			Icon:                   iconBase64,
			HomeObjectID:           record.Fields["spaceDashboardId"].GetStringValue(),
			ArchiveObjectID:        workspaceResponse.Info.ArchiveObjectId,
			ProfileObjectID:        workspaceResponse.Info.ProfileObjectId,
			MarketplaceWorkspaceID: workspaceResponse.Info.MarketplaceWorkspaceId,
			DeviceID:               workspaceResponse.Info.DeviceId,
			AccountSpaceID:         workspaceResponse.Info.AccountSpaceId,
			WidgetsID:              workspaceResponse.Info.WidgetsId,
			SpaceViewID:            workspaceResponse.Info.SpaceViewId,
			TechSpaceID:            a.accountInfo.TechSpaceId,
			Timezone:               workspaceResponse.Info.TimeZone,
			NetworkID:              workspaceResponse.Info.NetworkId,
		}
		spaces = append(spaces, space)
	}

	c.JSON(http.StatusOK, gin.H{"spaces": spaces})
}

// createSpaceHandler creates a new space
//
//	@Summary	Create a new Space
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		name	body		string				true	"Space Name"
//	@Success	200		{object}	Space				"Space created successfully"
//	@Failure	403		{object}	UnauthorizedError	"Unauthorized"
//	@Failure	502		{object}	ServerError			"Internal server error"
//	@Router		/spaces [post]
func (a *ApiServer) createSpaceHandler(c *gin.Context) {
	// Create new workspace with a random icon and import default usecase
	nameRequest := CreateSpaceRequest{}
	if err := c.BindJSON(&nameRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid JSON"})
		return
	}
	name := nameRequest.Name
	iconOption, err := rand.Int(rand.Reader, big.NewInt(13))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to generate random icon."})
		return
	}

	resp := a.mw.WorkspaceCreate(context.Background(), &pb.RpcWorkspaceCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				"iconOption": {Kind: &types.Value_NumberValue{NumberValue: float64(iconOption.Int64())}},
				"name":       {Kind: &types.Value_StringValue{StringValue: name}},
				"spaceDashboardId": {Kind: &types.Value_StringValue{
					StringValue: "lastOpened",
				}},
			},
		},
		UseCase:  1,
		WithChat: true,
	})

	if resp.Error.Code != pb.RpcWorkspaceCreateResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to create a new space."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"spaceId": resp.SpaceId, "name": name, "iconOption": iconOption})
}

// getSpaceMembersHandler retrieves a list of members for the specified space
//
//	@Summary	Retrieve a list of members for the specified Space
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string						true	"The ID of the space"
//	@Success	200			{object}	map[string][]SpaceMember	"List of members"
//	@Failure	403			{object}	UnauthorizedError			"Unauthorized"
//	@Failure	404			{object}	NotFoundError				"Resource not found"
//	@Failure	502			{object}	ServerError					"Internal server error"
//	@Router		/spaces/{space_id}/members [get]
func (a *ApiServer) getSpaceMembersHandler(c *gin.Context) {
	// Call ObjectSearch for all objects of type participant
	spaceId := c.Param("space_id")

	resp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_participant)),
			},
		},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of members."})
		return
	}

	members := make([]SpaceMember, 0, len(resp.Records))
	for _, record := range resp.Records {
		member := SpaceMember{
			Type:     "space_member",
			ID:       record.Fields["id"].GetStringValue(),
			Name:     record.Fields["name"].GetStringValue(),
			Identity: record.Fields["identity"].GetStringValue(),
			Role:     model.ParticipantPermissions_name[int32(record.Fields["participantPermissions"].GetNumberValue())],
		}

		members = append(members, member)
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

// getSpaceHandler retrieves objects in a specific space
//
//	@Summary	Retrieve objects in a specific space
//	@Tags		space_objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		offset		query		int						false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int						false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string][]Object	 "List of objects"
//	@Failure	403			{object}	UnauthorizedError		"Unauthorized"
//	@Failure	404			{object}	NotFoundError			"Resource not found"
//	@Failure	502			{object}	ServerError				"Internal server error"
//	@Router		/spaces/{space_id}/objects [get]
func (a *ApiServer) getSpaceObjectsHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	// TODO: implement offset and limit
	// offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	// limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	resp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceID,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value: pbtypes.IntList([]int{
					int(model.ObjectType_basic),
					int(model.ObjectType_note),
					int(model.ObjectType_bookmark),
					int(model.ObjectType_set),
					int(model.ObjectType_collection),
				}...),
			},
		},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of objects."})
		return
	}

	objects := make([]Object, 0, len(resp.Records))
	for _, record := range resp.Records {
		object := Object{
			Type:       model.ObjectTypeLayout_name[int32(record.Fields["layout"].GetNumberValue())],
			ID:         record.Fields["id"].GetStringValue(),
			Name:       record.Fields["name"].GetStringValue(),
			IconEmoji:  record.Fields["iconEmoji"].GetStringValue(),
			ObjectType: record.Fields["type"].GetStringValue(),
			SpaceID:    spaceID,
			// TODO: populate other fields
			// RootID:     record.Fields["rootId"].GetStringValue(),
			// Blocks:  []Block{},
			Details: []Detail{
				{
					ID: "lastModifiedDate",
					Details: map[string]interface{}{
						"lastModifiedDate": record.Fields["lastModifiedDate"].GetNumberValue(),
					},
				},
			},
		}

		objects = append(objects, object)
	}

	c.JSON(http.StatusOK, gin.H{"objects": objects})
}

// getObjectHandler retrieves a specific object in a space
//
//	@Summary	Retrieve a specific object in a space
//	@Tags		space_objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string				true	"The ID of the space"
//	@Param		object_id	path		string				true	"The ID of the object"
//	@Success	200			{object}	Object				"The requested object"
//	@Failure	403			{object}	UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	NotFoundError		"Resource not found"
//	@Failure	502			{object}	ServerError			"Internal server error"
//	@Router		/spaces/{space_id}/objects/{object_id} [get]
func (a *ApiServer) getObjectHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	objectID := c.Param("object_id")

	resp := a.mw.ObjectOpen(context.Background(), &pb.RpcObjectOpenRequest{
		SpaceId:  spaceID,
		ObjectId: objectID,
	})

	if resp.Error.Code != pb.RpcObjectOpenResponseError_NULL {
		if resp.Error.Code == pb.RpcObjectOpenResponseError_NOT_FOUND {
			c.JSON(http.StatusNotFound, gin.H{"message": "Object not found", "space_id": spaceID, "object_id": objectID})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve object."})
		return
	}

	object := Object{
		Type:       "object",
		ID:         objectID,
		Name:       resp.ObjectView.Details[0].Details.Fields["name"].GetStringValue(),
		IconEmoji:  resp.ObjectView.Details[0].Details.Fields["iconEmoji"].GetStringValue(),
		ObjectType: resp.ObjectView.Details[0].Details.Fields["type"].GetStringValue(),
		RootID:     resp.ObjectView.RootId,
		// TODO: populate other fields
		Blocks:  []Block{},
		Details: []Detail{},
	}

	c.JSON(http.StatusOK, gin.H{"object": object})
}

// createObjectHandler creates a new object in a specific space
//
//	@Summary	Create a new object in a specific space
//	@Tags		space_objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string				true	"The ID of the space"
//	@Param		object		body		map[string]string	true	"Object details (e.g., name)"
//	@Success	200			{object}	Object				"The created object"
//	@Failure	403			{object}	UnauthorizedError	"Unauthorized"
//	@Failure	502			{object}	ServerError			"Internal server error"
//	@Router		/spaces/{space_id}/objects [post]
func (a *ApiServer) createObjectHandler(c *gin.Context) {
	spaceID := c.Param("space_id")

	request := CreateObjectRequest{}
	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid JSON"})
		return
	}

	resp := a.mw.ObjectCreate(context.Background(), &pb.RpcObjectCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				"name":      {Kind: &types.Value_StringValue{StringValue: request.Name}},
				"iconEmoji": {Kind: &types.Value_StringValue{StringValue: request.IconEmoji}},
			},
		},
		// TODO figure out internal flags
		InternalFlags: []*model.InternalFlag{
			{Value: model.InternalFlagValue(2)},
		},
		TemplateId:          request.TemplateID,
		SpaceId:             spaceID,
		ObjectTypeUniqueKey: request.ObjectTypeUniqueKey,
		WithChat:            request.WithChat,
	})

	if resp.Error.Code != pb.RpcObjectCreateResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to create a new object."})
		return
	}

	object := Object{
		Type:       "object",
		ID:         resp.ObjectId,
		Name:       resp.Details.Fields["name"].GetStringValue(),
		IconEmoji:  resp.Details.Fields["iconEmoji"].GetStringValue(),
		ObjectType: request.ObjectTypeUniqueKey,
		SpaceID:    resp.Details.Fields["spaceId"].GetStringValue(),
		// TODO populate other fields
		// RootID:    resp.RootId,
		// Blocks:    []Block{},
		// Details: []Detail{},
	}

	c.JSON(http.StatusOK, gin.H{"object": object})
}

// updateObjectHandler updates an existing object in a specific space
//
//	@Summary	Update an existing object in a specific space
//	@Tags		space_objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string				true	"The ID of the space"
//	@Param		object_id	path		string				true	"The ID of the object"
//	@Param		object		body		Object				true	"The updated object details"
//	@Success	200			{object}	Object				"The updated object"
//	@Failure	403			{object}	UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	NotFoundError		"Resource not found"
//	@Failure	502			{object}	ServerError			"Internal server error"
//	@Router		/spaces/{space_id}/objects/{object_id} [put]
func (a *ApiServer) updateObjectHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	objectID := c.Param("object_id")
	// TODO: Implement logic to update an existing object
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet", "space_id": spaceID, "object_id": objectID})
}

// getObjectTypesHandler retrieves object types in a specific space
//
//	@Summary	Retrieve object types in a specific space
//	@Tags		types_and_templates
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		offset		query		int						false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int						false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string]ObjectType	"List of object types"
//	@Failure	403			{object}	UnauthorizedError		"Unauthorized"
//	@Failure	404			{object}	NotFoundError			"Resource not found"
//	@Failure	502			{object}	ServerError				"Internal server error"
//	@Router		/spaces/{space_id}/objectTypes [get]
func (a *ApiServer) getObjectTypesHandler(c *gin.Context) {
	spaceID := c.Param("space_id")

	resp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceID,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String("true"),
			},
		},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve object types."})
		return
	}

	objectTypes := make([]ObjectType, 0, len(resp.Records))
	for _, record := range resp.Records {
		objectTypes = append(objectTypes, ObjectType{
			Type:      "object_type",
			ID:        record.Fields["id"].GetStringValue(),
			UniqueKey: record.Fields["uniqueKey"].GetStringValue(),
			Name:      record.Fields["name"].GetStringValue(),
			IconEmoji: record.Fields["iconEmoji"].GetStringValue(),
		})
	}

	c.JSON(http.StatusOK, gin.H{"objectTypes": objectTypes})
}

// getObjectTypeTemplatesHandler retrieves a list of templates for a specific object type in a space
//
//	@Summary	Retrieve a list of templates for a specific object type in a space
//	@Tags		types_and_templates
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string						true	"The ID of the space"
//	@Param		typeId		path		string						true	"The ID of the object type"
//	@Success	200			{object}	map[string][]ObjectTemplate	"List of templates"
//	@Failure	403			{object}	UnauthorizedError			"Unauthorized"
//	@Failure	404			{object}	NotFoundError				"Resource not found"
//	@Failure	502			{object}	ServerError					"Internal server error"
//	@Router		/spaces/{space_id}/objectTypes/{typeId}/templates [get]
func (a *ApiServer) getObjectTypeTemplatesHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	typeID := c.Param("typeId")

	// First, determine the type Id of "ot-template" in the space
	templateTypeIdResp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceID,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("ot-template"),
			},
		},
	})

	if templateTypeIdResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve template type."})
		return
	}

	templateTypeID := templateTypeIdResp.Records[0].Fields["id"].GetStringValue()

	// Then, search all objects of the template type and filter by the target object type
	templateObjectsResp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceID,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(templateTypeID),
			},
		},
	})

	if templateObjectsResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve template objects."})
		return
	}

	templateIds := make([]string, 0)
	for _, record := range templateObjectsResp.Records {
		if record.Fields["targetObjectType"].GetStringValue() == typeID {
			templateIds = append(templateIds, record.Fields["id"].GetStringValue())
		}
	}

	// Finally, open each template and populate the response
	templates := make([]ObjectTemplate, 0, len(templateIds))
	for _, templateId := range templateIds {
		templateResp := a.mw.ObjectOpen(context.Background(), &pb.RpcObjectOpenRequest{
			SpaceId:  spaceID,
			ObjectId: templateId,
		})

		if templateResp.Error.Code != pb.RpcObjectOpenResponseError_NULL {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve template."})
			return
		}

		templates = append(templates, ObjectTemplate{
			Type:      "object_template",
			ID:        templateId,
			Name:      templateResp.ObjectView.Details[0].Details.Fields["name"].GetStringValue(),
			IconEmoji: templateResp.ObjectView.Details[0].Details.Fields["iconEmoji"].GetStringValue(),
		})
	}

	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

// getObjectsHandler searches and retrieves objects across all the spaces
//
//	@Summary	Search and retrieve objects across all the spaces
//	@Tags		search
//	@Accept		json
//	@Produce	json
//	@Param		search		query		string				false	"The search term to filter objects by name"
//	@Param		object_type	query		string				false	"Specify object type for search"
//	@Param		offset		query		int					false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int					false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string][]Object	"List of objects"
//	@Failure	403			{object}	UnauthorizedError	"Unauthorized"
//	@Failure	502			{object}	ServerError			"Internal server error"
//	@Router		/objects [get]
func (a *ApiServer) getObjectsHandler(c *gin.Context) {
	searchTerm := c.Query("search")
	objectType := c.Query("object_type")
	// TODO: implement offset and limit
	// offset := c.DefaultQuery("offset", "0")
	// limit := c.DefaultQuery("limit", "100")

	// First, call ObjectSearch for all objects of type spaceView
	resp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: a.accountInfo.TechSpaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
			},
		},
	})
	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of spaces."})
		return
	}

	// Then, get objects from each space that match the search parameters
	var filters = []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyLayout.String(),
			Condition:   model.BlockContentDataviewFilter_In,
			Value: pbtypes.IntList([]int{
				int(model.ObjectType_basic),
				int(model.ObjectType_note),
				int(model.ObjectType_bookmark),
				int(model.ObjectType_set),
				int(model.ObjectType_collection),
				int(model.ObjectType_participant),
			}...),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.String("true"),
		},
		{
			RelationKey: bundle.RelationKeyName.String(),
			Condition:   model.BlockContentDataviewFilter_Like,
			Value:       pbtypes.String(searchTerm),
		},
	}

	if searchTerm != "" {
		filters = append(filters, &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyName.String(),
			Condition:   model.BlockContentDataviewFilter_Like,
			Value:       pbtypes.String(searchTerm),
		})
	}

	if objectType != "" {
		filters = append(filters, &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyType.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(objectType),
		})
	}

	searchResults := make([]Object, 0)
	for _, spaceRecord := range resp.Records {
		objectSearchResponse := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
			SpaceId: spaceRecord.Fields["targetSpaceId"].GetStringValue(),
			Filters: filters,
			Sorts: []*model.BlockContentDataviewSort{{
				RelationKey: bundle.RelationKeyLastModifiedDate.String(),
				Type:        model.BlockContentDataviewSort_Desc,
			}},
		})

		for _, record := range objectSearchResponse.Records {
			objectTypeName, err := a.resolveTypeToName(spaceRecord.Fields["targetSpaceId"].GetStringValue(), record.Fields["type"].GetStringValue())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to resolve type to name."})
				return
			}

			searchResults = append(searchResults, Object{
				Type:       model.ObjectTypeLayout_name[int32(record.Fields["layout"].GetNumberValue())],
				ID:         record.Fields["id"].GetStringValue(),
				Name:       record.Fields["name"].GetStringValue(),
				IconEmoji:  record.Fields["iconEmoji"].GetStringValue(),
				ObjectType: objectTypeName,
				// TODO: populate other fields
				// RootID:     record.Fields["rootId"].GetStringValue(),
				// Blocks:     []Block{},
				Details: []Detail{
					{
						ID: "lastModifiedDate",
						Details: map[string]interface{}{
							"lastModifiedDate": record.Fields["lastModifiedDate"].GetNumberValue(),
						},
					},
				},
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"objects": searchResults})
}
