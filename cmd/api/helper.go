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

// getBlocks returns the blocks of the object
func (a *ApiServer) getBlocks(resp *pb.RpcObjectShowResponse) []Block {
	blocks := []Block{}

	for _, block := range resp.ObjectView.Blocks {
		var text *Text
		var file *File

		switch content := block.Content.(type) {
		case *model.BlockContentOfText:
			text = &Text{
				Text:    content.Text.Text,
				Style:   model.BlockContentTextStyle_name[int32(content.Text.Style)],
				Checked: content.Text.Checked,
				Color:   content.Text.Color,
				Icon:    a.getIconFromEmojiOrImage(content.Text.IconEmoji, content.Text.IconImage),
			}
		case *model.BlockContentOfFile:
			file = &File{
				Hash:           content.File.Hash,
				Name:           content.File.Name,
				Type:           model.BlockContentFileType_name[int32(content.File.Type)],
				Mime:           content.File.Mime,
				Size:           content.File.Size(),
				AddedAt:        int(content.File.AddedAt),
				TargetObjectId: content.File.TargetObjectId,
				State:          model.BlockContentFileState_name[int32(content.File.State)],
				Style:          model.BlockContentFileStyle_name[int32(content.File.Style)],
			}
			// TODO: other content types?
		}

		blocks = append(blocks, Block{
			Id:              block.Id,
			ChildrenIds:     block.ChildrenIds,
			BackgroundColor: block.BackgroundColor,
			Align:           mapAlign(block.Align),
			VerticalAlign:   mapVerticalAlign(block.VerticalAlign),
			Text:            text,
			File:            file,
		})
	}

	return blocks
}

func mapAlign(align model.BlockAlign) string {
	switch align {
	case model.Block_AlignLeft:
		return "left"
	case model.Block_AlignCenter:
		return "center"
	case model.Block_AlignRight:
		return "right"
	case model.Block_AlignJustify:
		return "justify"
	default:
		return "unknown"
	}
}

func mapVerticalAlign(align model.BlockVerticalAlign) string {
	switch align {
	case model.Block_VerticalAlignTop:
		return "top"
	case model.Block_VerticalAlignMiddle:
		return "middle"
	case model.Block_VerticalAlignBottom:
		return "bottom"
	default:
		return "unknown"
	}
}

// getDetails returns the details of the object
func (a *ApiServer) getDetails(resp *pb.RpcObjectShowResponse) []Detail {
	return []Detail{
		{
			Id: "lastModifiedDate",
			Details: map[string]interface{}{
				"lastModifiedDate": resp.ObjectView.Details[0].Details.Fields["lastModifiedDate"].GetNumberValue(),
			},
		},
		{
			Id: "createdDate",
			Details: map[string]interface{}{
				"createdDate": resp.ObjectView.Details[0].Details.Fields["createdDate"].GetNumberValue(),
			},
		},
		{
			Id: "tags",
			Details: map[string]interface{}{
				"tags": a.getTags(resp),
			},
		},
	}
}

// getTags returns the list of tags from the object details
func (a *ApiServer) getTags(resp *pb.RpcObjectShowResponse) []Tag {
	tags := []Tag{}

	tagField, ok := resp.ObjectView.Details[0].Details.Fields["tag"]
	if !ok {
		return tags
	}

	for _, tagId := range tagField.GetListValue().Values {
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

// respondWithPagination returns a json response with the paginated data and corresponding metadata
func respondWithPagination[T any](c *gin.Context, statusCode int, data []T, total, offset, limit int, hasMore bool) {
	c.JSON(statusCode, PaginatedResponse[T]{
		Data: data,
		Pagination: PaginationMeta{
			Total:   total,
			Offset:  offset,
			Limit:   limit,
			HasMore: hasMore,
		},
	})
}

// paginate paginates the given records based on the offset and limit
func paginate[T any](records []T, offset, limit int) ([]T, bool) {
	total := len(records)
	start := offset
	end := offset + limit

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginated := records[start:end]
	hasMore := end < total
	return paginated, hasMore
}
