package api

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"sort"

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
	Icon                string `json:"icon"`
	TemplateId          string `json:"template_id"`
	ObjectTypeUniqueKey string `json:"object_type_unique_key"`
	WithChat            bool   `json:"with_chat"`
}

type AddMessageRequest struct {
	Text  string `json:"text"`
	Style string `json:"style"`
}

// authDisplayCodeHandler generates a new challenge and returns the challenge ID
//
//	@Summary	Open a modal window with a code in Anytype Desktop app
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Success	200	{object}	AuthDisplayCodeResponse	"Challenge ID"
//	@Failure	502	{object}	ServerError				"Internal server error"
//	@Router		/auth/displayCode [post]
func (a *ApiServer) authDisplayCodeHandler(c *gin.Context) {
	resp := a.mw.AccountLocalLinkNewChallenge(c.Request.Context(), &pb.RpcAccountLocalLinkNewChallengeRequest{AppName: "api-test"})

	if resp.Error.Code != pb.RpcAccountLocalLinkNewChallengeResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to generate a new challenge."})
	}

	c.JSON(http.StatusOK, AuthDisplayCodeResponse{ChallengeId: resp.ChallengeId})
}

// authTokenHandler retrieves an authentication token using a code and challenge ID
//
//	@Summary	Retrieve an authentication token using a code
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		code			query		string				true	"The code retrieved from Anytype Desktop app"
//	@Param		challenge_id	query		string				true	"The challenge ID"
//	@Success	200				{object}	AuthTokenResponse	"Authentication token"
//	@Failure	400				{object}	ValidationError		"Invalid input"
//	@Failure	502				{object}	ServerError			"Internal server error"
//	@Router		/auth/token [get]
func (a *ApiServer) authTokenHandler(c *gin.Context) {
	if c.Query("challenge_id") == "" || c.Query("code") == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid input"})
		return
	}

	// Call AccountLocalLinkSolveChallenge to retrieve session token and app key
	resp := a.mw.AccountLocalLinkSolveChallenge(c.Request.Context(), &pb.RpcAccountLocalLinkSolveChallengeRequest{
		ChallengeId: c.Query("challenge_id"),
		Answer:      c.Query("code"),
	})

	if resp.Error.Code != pb.RpcAccountLocalLinkSolveChallengeResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to authenticate user."})
		return
	}

	c.JSON(http.StatusOK, AuthTokenResponse{
		SessionToken: resp.SessionToken,
		AppKey:       resp.AppKey,
	})
}

// getSpacesHandler retrieves a list of spaces
//
//	@Summary	Retrieve a list of spaces
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		offset	query		int							false	"The number of items to skip before starting to collect the result set"
//	@Param		limit	query		int							false	"The number of items to return"	default(100)
//	@Success	200		{object}	PaginatedResponse[Space]	"List of spaces"
//	@Failure	403		{object}	UnauthorizedError			"Unauthorized"
//	@Failure	404		{object}	NotFoundError				"Resource not found"
//	@Failure	502		{object}	ServerError					"Internal server error"
//	@Router		/spaces [get]
func (a *ApiServer) getSpacesHandler(c *gin.Context) {
	offset := c.GetInt("offset")
	limit := c.GetInt("limit")

	// Call ObjectSearch for all objects of type spaceView
	resp := a.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
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
			{
				RelationKey: bundle.RelationKeySpaceRemoteStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: "name",
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys:   []string{"targetSpaceId", "name", "iconEmoji", "iconImage"},
		Offset: int32(offset),
		Limit:  int32(limit + 1),
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of spaces."})
		return
	}

	if len(resp.Records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No spaces found."})
		return
	}

	spaces := make([]Space, 0, len(resp.Records))
	for _, record := range resp.Records {
		workspace, statusCode, errorMessage := a.getWorkspaceInfo(record.Fields["targetSpaceId"].GetStringValue())
		if statusCode != http.StatusOK {
			c.JSON(statusCode, gin.H{"message": errorMessage})
			return
		}

		workspace.Name = record.Fields["name"].GetStringValue()
		workspace.Icon = a.getIconFromEmojiOrImage(record.Fields["iconEmoji"].GetStringValue(), record.Fields["iconImage"].GetStringValue())

		spaces = append(spaces, workspace)
	}

	hasNext := false
	if len(spaces) > limit {
		hasNext = true
		spaces = spaces[:limit]
	}

	respondWithPagination(c, http.StatusOK, spaces, len(spaces), offset, limit, hasNext)
}

// createSpaceHandler creates a new space
//
//	@Summary	Create a new Space
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		name	body		string				true	"Space Name"
//	@Success	200		{object}	CreateSpaceResponse	"Space created successfully"
//	@Failure	403		{object}	UnauthorizedError	"Unauthorized"
//	@Failure	502		{object}	ServerError			"Internal server error"
//	@Router		/spaces [post]
func (a *ApiServer) createSpaceHandler(c *gin.Context) {
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

	// Create new workspace with a random icon and import default use case
	resp := a.mw.WorkspaceCreate(c.Request.Context(), &pb.RpcWorkspaceCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				"iconOption": {Kind: &types.Value_NumberValue{NumberValue: float64(iconOption.Int64())}},
				"name":       {Kind: &types.Value_StringValue{StringValue: name}},
				"spaceDashboardId": {Kind: &types.Value_StringValue{
					StringValue: "lastOpened",
				}},
			},
		},
		UseCase:  pb.RpcObjectImportUseCaseRequest_GET_STARTED,
		WithChat: true,
	})

	if resp.Error.Code != pb.RpcWorkspaceCreateResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to create a new space."})
		return
	}

	c.JSON(http.StatusOK, CreateSpaceResponse{SpaceId: resp.SpaceId, Name: name})
}

// getMembersHandler retrieves a list of members for the specified space
//
//	@Summary	Retrieve a list of members for the specified Space
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string						true	"The ID of the space"
//	@Param		offset		query		int							false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int							false	"The number of items to return"	default(100)
//	@Success	200			{object}	PaginatedResponse[Member]	"List of members"
//	@Failure	403			{object}	UnauthorizedError			"Unauthorized"
//	@Failure	404			{object}	NotFoundError				"Resource not found"
//	@Failure	502			{object}	ServerError					"Internal server error"
//	@Router		/spaces/{space_id}/members [get]
func (a *ApiServer) getMembersHandler(c *gin.Context) {
	spaceId := c.Param("space_id")
	offset := c.GetInt("offset")
	limit := c.GetInt("limit")

	// Call ObjectSearch for all objects of type participant
	resp := a.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_participant)),
			},
			{
				RelationKey: bundle.RelationKeyParticipantStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ParticipantStatus_Active)),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: "name",
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys:   []string{"id", "name", "iconEmoji", "iconImage", "identity", "globalName", "participantPermissions"},
		Offset: int32(offset),
		Limit:  int32(limit + 1),
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of members."})
		return
	}

	if len(resp.Records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No members found."})
		return
	}

	members := make([]Member, 0, len(resp.Records))
	for _, record := range resp.Records {
		icon := a.getIconFromEmojiOrImage(record.Fields["iconEmoji"].GetStringValue(), record.Fields["iconImage"].GetStringValue())

		member := Member{
			Type:       "space_member",
			Id:         record.Fields["id"].GetStringValue(),
			Name:       record.Fields["name"].GetStringValue(),
			Icon:       icon,
			Identity:   record.Fields["identity"].GetStringValue(),
			GlobalName: record.Fields["globalName"].GetStringValue(),
			Role:       model.ParticipantPermissions_name[int32(record.Fields["participantPermissions"].GetNumberValue())],
		}

		members = append(members, member)
	}

	hasNext := false
	if len(members) > limit {
		hasNext = true
		members = members[:limit]
	}

	respondWithPagination(c, http.StatusOK, members, len(members), offset, limit, hasNext)
}

// getObjectsHandler retrieves objects in a specific space
//
//	@Summary	Retrieve objects in a specific space
//	@Tags		space_objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string				true	"The ID of the space"
//	@Param		offset		query		int					false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int					false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string][]Object	"List of objects"
//	@Failure	403			{object}	UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	NotFoundError		"Resource not found"
//	@Failure	502			{object}	ServerError			"Internal server error"
//	@Router		/spaces/{space_id}/objects [get]
func (a *ApiServer) getObjectsHandler(c *gin.Context) {
	spaceId := c.Param("space_id")
	offset := c.GetInt("offset")
	limit := c.GetInt("limit")

	resp := a.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
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
		Keys:   []string{"id", "name", "type", "layout", "iconEmoji", "iconImage"},
		Offset: int32(offset),
		Limit:  int32(limit + 1),
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of objects."})
		return
	}

	if len(resp.Records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No objects found."})
		return
	}

	objects := make([]Object, 0, len(resp.Records))
	for _, record := range resp.Records {
		icon := a.getIconFromEmojiOrImage(record.Fields["iconEmoji"].GetStringValue(), record.Fields["iconImage"].GetStringValue())
		objectTypeName, statusCode, errorMessage := a.resolveTypeToName(spaceId, record.Fields["type"].GetStringValue())
		if statusCode != http.StatusOK {
			c.JSON(statusCode, gin.H{"message": errorMessage})
			return
		}

		objectShowResp := a.mw.ObjectShow(c.Request.Context(), &pb.RpcObjectShowRequest{
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
			Blocks:     a.getBlocks(objectShowResp),
			Details:    a.getDetails(objectShowResp),
		}

		objects = append(objects, object)
	}

	hasNext := false
	if len(objects) > limit {
		hasNext = true
		objects = objects[:limit]
	}

	respondWithPagination(c, http.StatusOK, objects, len(objects), offset, limit, hasNext)
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
	spaceId := c.Param("space_id")
	objectId := c.Param("object_id")

	resp := a.mw.ObjectShow(c.Request.Context(), &pb.RpcObjectShowRequest{
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

	objectTypeName, statusCode, errorMessage := a.resolveTypeToName(spaceId, resp.ObjectView.Details[0].Details.Fields["type"].GetStringValue())
	if statusCode != http.StatusOK {
		c.JSON(statusCode, gin.H{"message": errorMessage})
		return
	}

	object := Object{
		Type:       "object",
		Id:         objectId,
		Name:       resp.ObjectView.Details[0].Details.Fields["name"].GetStringValue(),
		Icon:       resp.ObjectView.Details[0].Details.Fields["iconEmoji"].GetStringValue(),
		ObjectType: objectTypeName,
		RootId:     resp.ObjectView.RootId,
		Blocks:     a.getBlocks(resp),
		Details:    a.getDetails(resp),
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
	spaceId := c.Param("space_id")

	request := CreateObjectRequest{}
	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid JSON"})
		return
	}

	resp := a.mw.ObjectCreate(c.Request.Context(), &pb.RpcObjectCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				"name":      {Kind: &types.Value_StringValue{StringValue: request.Name}},
				"iconEmoji": {Kind: &types.Value_StringValue{StringValue: request.Icon}},
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
	spaceId := c.Param("space_id")
	objectId := c.Param("object_id")
	// TODO: Implement logic to update an existing object
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet", "space_id": spaceId, "object_id": objectId})
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
	spaceId := c.Param("space_id")
	offset := c.GetInt("offset")
	limit := c.GetInt("limit")

	resp := a.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
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
		Keys:   []string{"id", "uniqueKey", "name", "iconEmoji"},
		Offset: int32(offset),
		Limit:  int32(limit + 1),
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve object types."})
		return
	}

	if len(resp.Records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No object types found."})
		return
	}

	objectTypes := make([]ObjectType, 0, len(resp.Records))
	for _, record := range resp.Records {
		objectTypes = append(objectTypes, ObjectType{
			Type:      "object_type",
			Id:        record.Fields["id"].GetStringValue(),
			UniqueKey: record.Fields["uniqueKey"].GetStringValue(),
			Name:      record.Fields["name"].GetStringValue(),
			Icon:      record.Fields["iconEmoji"].GetStringValue(),
		})
	}

	hasNext := false
	if len(objectTypes) > limit {
		hasNext = true
		objectTypes = objectTypes[:limit]
	}

	respondWithPagination(c, http.StatusOK, objectTypes, len(objectTypes), offset, limit, hasNext)
}

// getObjectTypeTemplatesHandler retrieves a list of templates for a specific object type in a space
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
//	@Failure	403			{object}	UnauthorizedError			"Unauthorized"
//	@Failure	404			{object}	NotFoundError				"Resource not found"
//	@Failure	502			{object}	ServerError					"Internal server error"
//	@Router		/spaces/{space_id}/objectTypes/{typeId}/templates [get]
func (a *ApiServer) getObjectTypeTemplatesHandler(c *gin.Context) {
	spaceId := c.Param("space_id")
	typeId := c.Param("typeId")
	offset := c.GetInt("offset")
	limit := c.GetInt("limit")

	// First, determine the type ID of "ot-template" in the space
	templateTypeIdResp := a.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
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
	templateObjectsResp := a.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(templateTypeId),
			},
		},
		Keys:   []string{"id", "targetObjectType", "name", "iconEmoji"},
		Offset: int32(offset),
		Limit:  int32(limit + 1),
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
	templates := make([]ObjectTemplate, 0, len(templateIds))
	for _, templateId := range templateIds {
		templateResp := a.mw.ObjectShow(c.Request.Context(), &pb.RpcObjectShowRequest{
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

	hasNext := false
	if len(templates) > limit {
		hasNext = true
		templates = templates[:limit]
	}

	respondWithPagination(c, http.StatusOK, templates, len(templates), offset, limit, hasNext)
}

// searchHandler searches and retrieves objects across all the spaces
//
//	@Summary	Search and retrieve objects across all the spaces
//	@Tags		search
//	@Accept		json
//	@Produce	json
//	@Param		query		query		string				false	"The search term to filter objects by name"
//	@Param		object_type	query		string				false	"Specify object type for search"
//	@Param		offset		query		int					false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int					false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string][]Object	"List of objects"
//	@Failure	403			{object}	UnauthorizedError	"Unauthorized"
//	@Failure	502			{object}	ServerError			"Internal server error"
//	@Router		/search [get]
func (a *ApiServer) searchHandler(c *gin.Context) {
	searchQuery := c.Query("query")
	objectType := c.Query("object_type")
	offset := c.GetInt("offset")
	limit := c.GetInt("limit")

	// First, call ObjectSearch for all objects of type spaceView
	spaceResp := a.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
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
			{
				RelationKey: bundle.RelationKeySpaceRemoteStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
			},
		},
		Keys: []string{"targetSpaceId"},
	})

	if spaceResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of spaces."})
		return
	}

	if len(spaceResp.Records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No spaces found."})
		return
	}

	// Then, get objects from each space that match the search parameters
	var filters = []*model.BlockContentDataviewFilter{
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
		{
			RelationKey: bundle.RelationKeyIsHidden.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.Bool(true),
		},
	}

	if searchQuery != "" {
		// TODO also include snippet for notes
		filters = append(filters, &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyName.String(),
			Condition:   model.BlockContentDataviewFilter_Like,
			Value:       pbtypes.String(searchQuery),
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
	for _, spaceRecord := range spaceResp.Records {
		spaceId := spaceRecord.Fields["targetSpaceId"].GetStringValue()
		objectResp := a.mw.ObjectSearch(c.Request.Context(), &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: filters,
			Sorts: []*model.BlockContentDataviewSort{{
				RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
				Type:           model.BlockContentDataviewSort_Desc,
				Format:         model.RelationFormat_longtext,
				IncludeTime:    true,
				EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
			}},
			Keys:   []string{"id", "name", "type", "layout", "iconEmoji", "iconImage"},
			Offset: int32(offset),
			Limit:  int32(limit + 1),
			// TODO split limit between spaces
			// Limit: paginationLimitPerSpace,
			// FullText: searchTerm,
		})

		if objectResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of objects."})
			return
		}

		if len(objectResp.Records) == 0 {
			continue
		}

		for _, record := range objectResp.Records {
			icon := a.getIconFromEmojiOrImage(record.Fields["iconEmoji"].GetStringValue(), record.Fields["iconImage"].GetStringValue())
			objectTypeName, statusCode, errorMessage := a.resolveTypeToName(spaceId, record.Fields["type"].GetStringValue())
			if statusCode != http.StatusOK {
				c.JSON(statusCode, gin.H{"message": errorMessage})
				return
			}

			objectShowResp := a.mw.ObjectShow(c.Request.Context(), &pb.RpcObjectShowRequest{
				SpaceId:  spaceId,
				ObjectId: record.Fields["id"].GetStringValue(),
			})

			searchResults = append(searchResults, Object{
				Type:       model.ObjectTypeLayout_name[int32(record.Fields["layout"].GetNumberValue())],
				Id:         record.Fields["id"].GetStringValue(),
				Name:       record.Fields["name"].GetStringValue(),
				Icon:       icon,
				ObjectType: objectTypeName,
				SpaceId:    spaceId,
				RootId:     objectShowResp.ObjectView.RootId,
				Blocks:     a.getBlocks(objectShowResp),
				Details:    a.getDetails(objectShowResp),
			})
		}
	}

	// sort after lastModifiedDate to achieve descending sort order across all spaces
	sort.Slice(searchResults, func(i, j int) bool {
		return searchResults[i].Details[0].Details["lastModifiedDate"].(float64) > searchResults[j].Details[0].Details["lastModifiedDate"].(float64)
	})

	// TODO: solve global pagination vs per space pagination
	hasNext := false
	if len(searchResults) > limit {
		hasNext = true
		searchResults = searchResults[:limit]
	}

	respondWithPagination(c, http.StatusOK, searchResults, len(searchResults), offset, limit, hasNext)
}

// getChatMessagesHandler retrieves last chat messages
//
//	@Summary	Retrieve last chat messages
//	@Tags		chat
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string						true	"The ID of the space"
//	@Param		offset		query		int							false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int							false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string][]ChatMessage	"List of chat messages"
//	@Failure	502			{object}	ServerError					"Internal server error"
//	@Router		/v1/spaces/{space_id}/chat/messages [get]
func (a *ApiServer) getChatMessagesHandler(c *gin.Context) {
	spaceId := c.Param("space_id")
	// TODO: implement offset
	// offset := c.GetInt("offset")
	limit := c.GetInt("limit")

	chatId, statusCode, errorMessage := a.getChatIdForSpace(spaceId)
	if statusCode != http.StatusOK {
		c.JSON(statusCode, gin.H{"message": errorMessage})
		return
	}

	lastMessages := a.mw.ChatSubscribeLastMessages(c.Request.Context(), &pb.RpcChatSubscribeLastMessagesRequest{
		ChatObjectId: chatId,
		Limit:        int32(limit),
	})

	if lastMessages.Error.Code != pb.RpcChatSubscribeLastMessagesResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve last messages."})
	}

	messages := make([]ChatMessage, 0, len(lastMessages.Messages))
	for _, message := range lastMessages.Messages {

		attachments := make([]Attachment, 0, len(message.Attachments))
		for _, attachment := range message.Attachments {
			target := attachment.Target
			if attachment.Type != model.ChatMessageAttachment_LINK {
				target = a.getGatewayURLForMedia(attachment.Target, false)
			}
			attachments = append(attachments, Attachment{
				Target: target,
				Type:   model.ChatMessageAttachmentAttachmentType_name[int32(attachment.Type)],
			})
		}

		messages = append(messages, ChatMessage{
			Type:             "chat_message",
			Id:               message.Id,
			Creator:          message.Creator,
			CreatedAt:        message.CreatedAt,
			ReplyToMessageId: message.ReplyToMessageId,
			Message: MessageContent{
				Text: message.Message.Text,
				// TODO: params
				// Style: nil,
				// Marks: nil,
			},
			Attachments: attachments,
			Reactions: Reactions{
				ReactionsMap: func() map[string]IdentityList {
					reactionsMap := make(map[string]IdentityList)
					for emoji, ids := range message.Reactions.Reactions {
						reactionsMap[emoji] = IdentityList{Ids: ids.Ids}
					}
					return reactionsMap
				}(),
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{"chatId": chatId, "messages": messages})
}

// getChatMessageHandler retrieves a specific chat message by message_id
//
//	@Summary	Retrieve a specific chat message
//	@Tags		chat
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string			true	"The ID of the space"
//	@Param		message_id	path		string			true	"Message ID"
//	@Success	200			{object}	ChatMessage		"Chat message"
//	@Failure	404			{object}	NotFoundError	"Message not found"
//	@Failure	502			{object}	ServerError		"Internal server error"
//	@Router		/v1/spaces/{space_id}/chat/messages/{message_id} [get]
func (a *ApiServer) getChatMessageHandler(c *gin.Context) {
	// TODO: Implement logic to retrieve a specific chat message by message_id

	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet"})
}

// addChatMessageHandler adds a new chat message to chat
//
//	@Summary	Add a new chat message
//	@Tags		chat
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string			true	"The ID of the space"
//	@Param		message		body		ChatMessage		true	"Chat message"
//	@Success	201			{object}	ChatMessage		"Created chat message"
//	@Failure	400			{object}	ValidationError	"Invalid input"
//	@Failure	502			{object}	ServerError		"Internal server error"
//	@Router		/v1/spaces/{space_id}/chat/messages [post]
func (a *ApiServer) addChatMessageHandler(c *gin.Context) {
	spaceId := c.Param("space_id")

	request := AddMessageRequest{}
	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid JSON"})
		return
	}

	chatId, statusCode, errorMessage := a.getChatIdForSpace(spaceId)
	if statusCode != http.StatusOK {
		c.JSON(statusCode, gin.H{"message": errorMessage})
		return
	}

	resp := a.mw.ChatAddMessage(c.Request.Context(), &pb.RpcChatAddMessageRequest{
		ChatObjectId: chatId,
		Message: &model.ChatMessage{
			Id:               "",
			OrderId:          "",
			Creator:          "",
			CreatedAt:        0,
			ModifiedAt:       0,
			ReplyToMessageId: "",
			Message: &model.ChatMessageMessageContent{
				Text: request.Text,
				// TODO: param
				// Style: request.Style,
			},
		},
	})

	if resp.Error.Code != pb.RpcChatAddMessageResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to create message."})
	}

	c.JSON(http.StatusOK, gin.H{"messageId": resp.MessageId})
}

// updateChatMessageHandler updates an existing chat message by message_id
//
//	@Summary	Update an existing chat message
//	@Tags		chat
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string			true	"The ID of the space"
//	@Param		message_id	path		string			true	"Message ID"
//	@Param		message		body		ChatMessage		true	"Chat message"
//	@Success	200			{object}	ChatMessage		"Updated chat message"
//	@Failure	400			{object}	ValidationError	"Invalid input"
//	@Failure	404			{object}	NotFoundError	"Message not found"
//	@Failure	502			{object}	ServerError		"Internal server error"
//	@Router		/v1/spaces/{space_id}/chat/messages/{message_id} [put]
func (a *ApiServer) updateChatMessageHandler(c *gin.Context) {
	// TODO: Implement logic to update an existing chat message by message_id

	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet"})
}

// deleteChatMessageHandler deletes a chat message by message_id
//
//	@Summary	Delete a chat message
//	@Tags		chat
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path	string	true	"The ID of the space"
//	@Param		message_id	path	string	true	"Message ID"
//	@Success	204			"Message deleted successfully"
//	@Failure	404			{object}	NotFoundError	"Message not found"
//	@Failure	502			{object}	ServerError		"Internal server error"
//	@Router		/v1/spaces/{space_id}/chat/messages/{message_id} [delete]
func (a *ApiServer) deleteChatMessageHandler(c *gin.Context) {
	// TODO: Implement logic to delete a chat message by message_id

	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet"})
}
