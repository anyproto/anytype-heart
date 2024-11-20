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

type NameRequest struct {
	Name string `json:"name"`
}

// /v1/auth/displayCode [POST]
func (a *ApiServer) authDisplayCodeHandler(c *gin.Context) {
	// Call AccountLocalLinkNewChallenge to display code modal
	ctx := context.Background()
	resp := a.mw.AccountLocalLinkNewChallenge(ctx, &pb.RpcAccountLocalLinkNewChallengeRequest{AppName: "api-test"})

	if resp.Error.Code != pb.RpcAccountLocalLinkNewChallengeResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to generate a new challenge."})
	}

	c.JSON(http.StatusOK, gin.H{"challengeId": resp.ChallengeId})
}

// /v1/auth/token [GET]
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

// /v1/spaces [GET]
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

	// Convert the response to a list of spaces with their details: type, id, homeObjectID, archiveObjectID, profileObjectID, marketplaceWorkspaceID, deviceID, accountSpaceID, widgetsID, spaceViewID, techSpaceID, timezone, networkID
	spaces := make([]Space, 0, len(resp.Records))
	for _, record := range resp.Records {
		typeName, err := a.resolveTypeToName(record.Fields["targetSpaceId"].GetStringValue(), "ot-space")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to resolve type to name."})
			return
		}

		// TODO: Populate missing fields
		space := Space{
			Type:         typeName,
			ID:           record.Fields["id"].GetStringValue(),
			Name:         record.Fields["name"].GetStringValue(),
			HomeObjectID: record.Fields["spaceDashboardId"].GetStringValue(),
			// ArchiveObjectID:        record.Fields["archive_object_id"].GetStringValue(),
			// ProfileObjectID:        record.Fields["profile_object_id"].GetStringValue(),
			// MarketplaceWorkspaceID: record.Fields["marketplace_workspace_id"].GetStringValue(),
			// DeviceID:               record.Fields["device_id"].GetStringValue(),
			// AccountSpaceID:         record.Fields["account_space_id"].GetStringValue(),
			// WidgetsID:              record.Fields["widgets_id"].GetStringValue(),
			// SpaceViewID:            record.Fields["space_view_id"].GetStringValue(),
			TechSpaceID: a.accountInfo.TechSpaceId,
			// Timezone:               record.Fields["timezone"].GetStringValue(),
			// NetworkID:              record.Fields["network_id"].GetStringValue(),
		}
		spaces = append(spaces, space)
	}

	c.JSON(http.StatusOK, gin.H{"spaces": spaces})
}

// /v1/spaces [POST]
func (a *ApiServer) createSpaceHandler(c *gin.Context) {
	// Create new workspace with a random icon and import default usecase
	nameRequest := NameRequest{}
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

// /v1/spaces/:space_id/members [GET]
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

	// Convert the response to a slice of SpaceMember structs with their details: type, identity, name, role
	members := make([]SpaceMember, 0, len(resp.Records))
	for _, record := range resp.Records {
		typeName, err := a.resolveTypeToName(spaceId, record.Fields["type"].GetStringValue())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to resolve type to name."})
			return
		}

		member := SpaceMember{
			Type: typeName,
			ID:   record.Fields["identity"].GetStringValue(),
			Name: record.Fields["name"].GetStringValue(),
			Role: model.ParticipantPermissions_name[int32(record.Fields["participantPermissions"].GetNumberValue())],
		}

		members = append(members, member)
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

// /v1/spaces/:space_id/objects [GET]
func (a *ApiServer) getSpaceObjectsHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	// TODO: Implement logic to retrieve objects in a space
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet", "space_id": spaceID})
}

// /v1/spaces/:space_id/objects/:object_id [GET]
func (a *ApiServer) getObjectHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	objectID := c.Param("object_id")
	// TODO: Implement logic to retrieve a specific object
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet", "space_id": spaceID, "object_id": objectID})
}

// /v1/spaces/:space_id/objects/:object_id [POST]
func (a *ApiServer) createObjectHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	objectID := c.Param("object_id")
	// TODO: Implement logic to create a new object
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet", "space_id": spaceID, "object_id": objectID})
}

// /v1/spaces/:space_id/objects/:object_id [PUT]
func (a *ApiServer) updateObjectHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	objectID := c.Param("object_id")
	// TODO: Implement logic to update an existing object
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet", "space_id": spaceID, "object_id": objectID})
}

// /v1/spaces/:space_id/objectTypes [GET]
func (a *ApiServer) getObjectTypesHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	// TODO: Implement logic to retrieve object types in a space
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet", "space_id": spaceID})
}

// /v1/spaces/:space_id/objectTypes/:typeId/templates [GET]
func (a *ApiServer) getObjectTypeTemplatesHandler(c *gin.Context) {
	spaceID := c.Param("space_id")
	typeID := c.Param("typeId")
	// TODO: Implement logic to retrieve templates for an object type
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet", "space_id": spaceID, "typeId": typeID})
}

// /v1/objects [GET]
func (a *ApiServer) getObjectsHandler(c *gin.Context) {
	// TODO: Implement logic to search and retrieve objects across all spaces
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet"})
}
