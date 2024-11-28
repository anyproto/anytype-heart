package api

import (
	"context"
	"crypto/rand"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	httpTimeout     = 1 * time.Second
	paginationLimit = 100
)

type CreateSpaceRequest struct {
	Name string `json:"name"`
}

type CreateObjectRequest struct {
	Name                string `json:"name"`
	IconEmoji           string `json:"icon_emoji"`
	TemplateId          string `json:"template_id"`
	ObjectTypeUniqueKey string `json:"object_type_unique_key"`
	WithChat            bool   `json:"with_chat"`
}

type AddMessageRequest struct {
	Text  string `json:"text"`
	Style string `json:"style"`
}

// authdisplayCodeHandler generates a new challenge and returns the challenge Id
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
//	@Summary	Retrieve an authentication token using a code
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		code		query		string				true	"The code retrieved from Anytype Desktop app"
//	@Param		challengeId	query		string				true	"The challenge ID"
//	@Success	200			{object}	map[string]string	"Access and refresh tokens"
//	@Failure	400			{object}	ValidationError		"Invalid input"
//	@Failure	502			{object}	ServerError			"Internal server error"
//	@Router		/auth/token [get]
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
	})
	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of spaces."})
		return
	}

	spaces := make([]Space, 0, len(resp.Records))
	for _, record := range resp.Records {
		spaceId := record.Fields["targetSpaceId"].GetStringValue()
		workspace := a.getWorkspaceInfo(c, spaceId)
		workspace.Name = record.Fields["name"].GetStringValue()

		// Set space icon to gateway URL
		iconImageId := record.Fields["iconImage"].GetStringValue()
		if iconImageId != "" {
			workspace.Icon = a.getGatewayURLForMedia(iconImageId, true)
		}

		spaces = append(spaces, workspace)
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
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: "name",
				Type:        model.BlockContentDataviewSort_Asc,
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
			Type:       "space_member",
			Id:         record.Fields["id"].GetStringValue(),
			Name:       record.Fields["name"].GetStringValue(),
			Identity:   record.Fields["identity"].GetStringValue(),
			GlobalName: record.Fields["globalName"].GetStringValue(),
			Role:       model.ParticipantPermissions_name[int32(record.Fields["participantPermissions"].GetNumberValue())],
		}

		// Set member icon to gateway URL
		iconImageId := record.Fields["iconImage"].GetStringValue()
		if iconImageId != "" {
			member.Icon = a.getGatewayURLForMedia(iconImageId, true)
		}

		members = append(members, member)
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

// getSpaceObjectsHandler retrieves objects in a specific space
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
func (a *ApiServer) getSpaceObjectsHandler(c *gin.Context) {
	spaceId := c.Param("space_id")
	// TODO: implement offset and limit
	// offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	// limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	resp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
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
		objectTypeName, err := a.resolveTypeToName(spaceId, record.Fields["type"].GetStringValue())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to resolve object type name."})
			return
		}

		object := Object{
			// TODO fix type inconsistency
			Type:       model.ObjectTypeLayout_name[int32(record.Fields["layout"].GetNumberValue())],
			Id:         record.Fields["id"].GetStringValue(),
			Name:       record.Fields["name"].GetStringValue(),
			IconEmoji:  record.Fields["iconEmoji"].GetStringValue(),
			ObjectType: objectTypeName,
			SpaceId:    spaceId,
			// TODO: populate other fields
			// RootId:     record.Fields["rootId"].GetStringValue(),
			// Blocks:  []Block{},
			Details: []Detail{
				{
					Id: "lastModifiedDate",
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
	spaceId := c.Param("space_id")
	objectId := c.Param("object_id")

	resp := a.mw.ObjectOpen(context.Background(), &pb.RpcObjectOpenRequest{
		SpaceId:  spaceId,
		ObjectId: objectId,
	})

	if resp.Error.Code != pb.RpcObjectOpenResponseError_NULL {
		if resp.Error.Code == pb.RpcObjectOpenResponseError_NOT_FOUND {
			c.JSON(http.StatusNotFound, gin.H{"message": "Object not found", "space_id": spaceId, "object_id": objectId})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve object."})
		return
	}

	objectTypeName, err := a.resolveTypeToName(spaceId, resp.ObjectView.Details[0].Details.Fields["type"].GetStringValue())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to resolve object type name."})
		return
	}

	object := Object{
		Type:       "object",
		Id:         objectId,
		Name:       resp.ObjectView.Details[0].Details.Fields["name"].GetStringValue(),
		IconEmoji:  resp.ObjectView.Details[0].Details.Fields["iconEmoji"].GetStringValue(),
		ObjectType: objectTypeName,
		RootId:     resp.ObjectView.RootId,
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
	spaceId := c.Param("space_id")

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
		IconEmoji:  resp.Details.Fields["iconEmoji"].GetStringValue(),
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

	resp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
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
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve object types."})
		return
	}

	objectTypes := make([]ObjectType, 0, len(resp.Records))
	for _, record := range resp.Records {
		objectTypes = append(objectTypes, ObjectType{
			Type:      "object_type",
			Id:        record.Fields["id"].GetStringValue(),
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
	spaceId := c.Param("space_id")
	typeId := c.Param("typeId")

	// First, determine the type Id of "ot-template" in the space
	templateTypeIdResp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
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

	templateTypeId := templateTypeIdResp.Records[0].Fields["id"].GetStringValue()

	// Then, search all objects of the template type and filter by the target object type
	templateObjectsResp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(templateTypeId),
			},
		},
	})

	if templateObjectsResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve template objects."})
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
		templateResp := a.mw.ObjectOpen(context.Background(), &pb.RpcObjectOpenRequest{
			SpaceId:  spaceId,
			ObjectId: templateId,
		})

		if templateResp.Error.Code != pb.RpcObjectOpenResponseError_NULL {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve template."})
			return
		}

		templates = append(templates, ObjectTemplate{
			Type:      "object_template",
			Id:        templateId,
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
	l := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(l)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to parse limit."})
		return
	}

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

	if searchTerm != "" {
		// TODO also include snippet for notes
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
		spaceId := spaceRecord.Fields["targetSpaceId"].GetStringValue()
		objectSearchResponse := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: filters,
			Sorts: []*model.BlockContentDataviewSort{{
				RelationKey: bundle.RelationKeyLastModifiedDate.String(),
				Type:        model.BlockContentDataviewSort_Desc,
			}},
			Keys:  []string{"id", "name", "type", "layout", "iconEmoji", "lastModifiedDate"},
			Limit: 25,
			// FullText: searchTerm,
		})
		for _, record := range objectSearchResponse.Records {
			objectTypeName, err := a.resolveTypeToName(spaceId, record.Fields["type"].GetStringValue())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to resolve type to name."})
				return
			}

			searchResults = append(searchResults, Object{
				Type:       model.ObjectTypeLayout_name[int32(record.Fields["layout"].GetNumberValue())],
				Id:         record.Fields["id"].GetStringValue(),
				Name:       record.Fields["name"].GetStringValue(),
				IconEmoji:  record.Fields["iconEmoji"].GetStringValue(),
				ObjectType: objectTypeName,
				SpaceId:    spaceId,
				// TODO: populate other fields
				// RootId:     record.Fields["rootId"].GetStringValue(),
				// Blocks:     []Block{},
				Details: []Detail{
					{
						Id: "lastModifiedDate",
						Details: map[string]interface{}{
							"lastModifiedDate": record.Fields["lastModifiedDate"].GetNumberValue(),
						},
					},
				},
			})
		}
	}

	if len(searchResults) > limit {
		searchResults = searchResults[:limit]
	}

	c.JSON(http.StatusOK, gin.H{"objects": searchResults})
}

// getChatMessagesHandler retrieves last chat messages
//
//	@Summary	Retrieve last chat messages
//	@Tags		chat
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string						true	"The ID of the space"
//	@Success	200			{object}	map[string][]ChatMessage	"List of chat messages"
//	@Failure	502			{object}	ServerError					"Internal server error"
//	@Router		/v1/spaces/{space_id}/chat/messages [get]
func (a *ApiServer) getChatMessagesHandler(c *gin.Context) {
	spaceId := c.Param("space_id")
	chatId := a.getChatIdForSpace(c, spaceId)

	lastMessages := a.mw.ChatSubscribeLastMessages(context.Background(), &pb.RpcChatSubscribeLastMessagesRequest{
		ChatObjectId: chatId,
		Limit:        paginationLimit,
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

	c.JSON(http.StatusOK, gin.H{"messages": messages})
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

	chatId := a.getChatIdForSpace(c, spaceId)
	resp := a.mw.ChatAddMessage(context.Background(), &pb.RpcChatAddMessageRequest{
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
