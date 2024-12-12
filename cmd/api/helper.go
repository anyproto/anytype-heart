package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// getGatewayURLForMedia returns the URL of file gateway for the media object with the given ID
func (a *ApiServer) getGatewayURLForMedia(objectId string, isIcon bool) string {
	widthParam := ""
	if isIcon {
		widthParam = "?width=100"
	}
	return fmt.Sprintf("%s/image/%s%s", a.accountInfo.GatewayUrl, objectId, widthParam)
}

// resolveTypeToName resolves the type ID to the name of the type, e.g. "ot-page" to "Page" or "bafyreigyb6l5szohs32ts26ku2j42yd65e6hqy2u3gtzgdwqv6hzftsetu" to "Custom Type"
func (a *ApiServer) resolveTypeToName(spaceId string, typeId string) (string, *pb.RpcObjectSearchResponseError) {
	// Can't look up preinstalled types based on relation key, therefore need to use unique key
	relKey := bundle.RelationKeyId.String()
	if strings.Contains(typeId, "ot-") {
		relKey = bundle.RelationKeyUniqueKey.String()
	}

	// Call ObjectSearch for object of specified type and return the name
	resp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: relKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(typeId),
			},
		},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return "", resp.Error
	}

	if len(resp.Records) == 0 {
		return "", &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_BAD_INPUT, Description: "Type not found"}
	}

	return resp.Records[0].Fields["name"].GetStringValue(), nil
}

// getChatIdForSpace returns the chat ID for the space with the given ID
func (a *ApiServer) getChatIdForSpace(c *gin.Context, spaceId string) string {
	workspace := a.getWorkspaceInfo(c, spaceId)

	resp := a.mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: workspace.WorkspaceObjectId,
	})

	if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to open workspace object."})
		return ""
	}

	if !resp.ObjectView.Details[0].Details.Fields["hasChat"].GetBoolValue() {
		c.JSON(http.StatusNotFound, gin.H{"message": "Chat not found"})
		return ""
	}

	return resp.ObjectView.Details[0].Details.Fields["chatId"].GetStringValue()
}

// getWorkspaceInfo returns the workspace info for the space with the given ID
func (a *ApiServer) getWorkspaceInfo(c *gin.Context, spaceId string) Space {
	workspaceResponse := a.mw.WorkspaceOpen(context.Background(), &pb.RpcWorkspaceOpenRequest{
		SpaceId:  spaceId,
		WithChat: true,
	})

	if workspaceResponse.Error.Code != pb.RpcWorkspaceOpenResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to open workspace."})
		return Space{}
	}

	return Space{
		Type:                   "space",
		Id:                     spaceId,
		HomeObjectId:           workspaceResponse.Info.HomeObjectId,
		ArchiveObjectId:        workspaceResponse.Info.ArchiveObjectId,
		ProfileObjectId:        workspaceResponse.Info.ProfileObjectId,
		MarketplaceWorkspaceId: workspaceResponse.Info.MarketplaceWorkspaceId,
		WorkspaceObjectId:      workspaceResponse.Info.WorkspaceObjectId,
		DeviceId:               workspaceResponse.Info.DeviceId,
		AccountSpaceId:         workspaceResponse.Info.AccountSpaceId,
		WidgetsId:              workspaceResponse.Info.WidgetsId,
		SpaceViewId:            workspaceResponse.Info.SpaceViewId,
		TechSpaceId:            workspaceResponse.Info.TechSpaceId,
		Timezone:               workspaceResponse.Info.TimeZone,
		NetworkId:              workspaceResponse.Info.NetworkId,
	}
}

// getIconFromEmojiOrImage returns the icon to use for the object, which can be either an emoji or an image url
func (a *ApiServer) getIconFromEmojiOrImage(c *gin.Context, iconEmoji string, iconImage string) string {
	if iconEmoji != "" {
		return iconEmoji
	}

	if iconImage != "" {
		return a.getGatewayURLForMedia(iconImage, true)
	}

	return ""
}
