package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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
func (a *ApiServer) resolveTypeToName(spaceId string, typeId string) (typeName string, statusCode int, errorMessage string) {
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
		return "", http.StatusInternalServerError, "Failed to search for type."
	}

	if len(resp.Records) == 0 {
		return "", http.StatusNotFound, "Type not found."
	}

	return resp.Records[0].Fields["name"].GetStringValue(), http.StatusOK, ""
}

// getChatIdForSpace returns the chat ID for the space with the given ID
func (a *ApiServer) getChatIdForSpace(spaceId string) (chatId string, statusCode int, errorMessage string) {
	workspace, statusCode, errorMessage := a.getWorkspaceInfo(spaceId)
	if statusCode != http.StatusOK {
		return "", statusCode, errorMessage
	}

	resp := a.mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: workspace.WorkspaceObjectId,
	})

	if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return "", http.StatusInternalServerError, "Failed to open workspace object."
	}

	if !resp.ObjectView.Details[0].Details.Fields["hasChat"].GetBoolValue() {
		return "", http.StatusNotFound, "Chat not found."
	}

	return resp.ObjectView.Details[0].Details.Fields["chatId"].GetStringValue(), http.StatusOK, ""
}

// getWorkspaceInfo returns the workspace info for the space with the given ID
func (a *ApiServer) getWorkspaceInfo(spaceId string) (space Space, statusCode int, errorMessage string) {
	workspaceResponse := a.mw.WorkspaceOpen(context.Background(), &pb.RpcWorkspaceOpenRequest{
		SpaceId:  spaceId,
		WithChat: true,
	})

	if workspaceResponse.Error.Code != pb.RpcWorkspaceOpenResponseError_NULL {
		return Space{}, http.StatusInternalServerError, "Failed to open workspace."
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
	}, http.StatusOK, ""
}

// getIconFromEmojiOrImage returns the icon to use for the object, which can be either an emoji or an image url
func (a *ApiServer) getIconFromEmojiOrImage(iconEmoji string, iconImage string) string {
	if iconEmoji != "" {
		return iconEmoji
	}

	if iconImage != "" {
		return a.getGatewayURLForMedia(iconImage, true)
	}

	return ""
}

// getTags returns the list of tags from the object details
func (a *ApiServer) getTags(resp *pb.RpcObjectShowResponse) []Tag {
	tags := []Tag{}
	for _, tagId := range resp.ObjectView.Details[0].Details.Fields["tag"].GetListValue().Values {
		id := tagId.GetStringValue()
		for _, detail := range resp.ObjectView.Details {
			if detail.Id == id {
				tags = append(tags, Tag{
					Id:    id,
					Name:  detail.Details.Fields["name"].GetStringValue(),
					Color: detail.Details.Fields["relationOptionColor"].GetStringValue(),
				})
				break
			}
		}
	}
	return tags
}
