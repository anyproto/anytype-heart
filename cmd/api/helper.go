package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type IconSpace struct {
	Text string
	Bg   map[string]string
	List []string
}

var iconSpace = IconSpace{
	Text: "#fff",
	Bg: map[string]string{
		"grey":   "#949494",
		"yellow": "#ecd91b",
		"orange": "#ffb522",
		"red":    "#f55522",
		"pink":   "#e51ca0",
		"purple": "#ab50cc",
		"blue":   "#3e58eb",
		"ice":    "#2aa7ee",
		"teal":   "#0fc8ba",
		"lime":   "#5dd400",
	},
	List: []string{"grey", "yellow", "orange", "red", "pink", "purple", "blue", "ice", "teal", "lime"},
}

func (a *ApiServer) spaceSvg(option int, size int, iconName string) string {
	if option < 1 || option > len(iconSpace.List) {
		return ""
	}
	bgColor := iconSpace.Bg[iconSpace.List[option-1]]

	fontWeight := func(size int) string {
		if size > 50 {
			return "bold"
		}
		return "normal"
	}

	fontSize := func(size int) int {
		return size / 2
	}

	text := fmt.Sprintf(`<text x="50%%" y="50%%" text-anchor="middle" dominant-baseline="central" fill="%s" font-family="Inter, Helvetica" font-weight="%s" font-size="%dpx">%s</text>`,
		iconSpace.Text, fontWeight(size), fontSize(size), iconName)

	svg := fmt.Sprintf(`
		<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.1" id="Layer_1" x="0px" y="0px" viewBox="0 0 %d %d" xml:space="preserve" height="%dpx" width="%dpx">
			<rect width="%d" height="%d" fill="%s"/>
			%s
		</svg>`, size, size, size, size, size, size, bgColor, text)

	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func validateURL(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ""
	}
	return url
}

func (a *ApiServer) imageToBase64(imagePath string) (string, error) {
	client := &http.Client{
		Timeout: httpTimeout,
	}
	resp, err := client.Get(validateURL(imagePath))
	if err != nil {
		// don't return error if image is not found
		return "", nil
	}
	defer resp.Body.Close()

	fileBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(fileBytes)
	return encoded, nil
}

func (a *ApiServer) getGatewayURLForMedia(objectId string, isIcon bool) string {
	widthParam := ""
	if isIcon {
		widthParam = "?width=100"
	}
	return fmt.Sprintf("%s/image/%s%s", a.accountInfo.GatewayUrl, objectId, widthParam)
}

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

func (a *ApiServer) getChatIdForSpace(c *gin.Context, spaceId string) string {
	workspace := a.getWorkspaceInfo(c, spaceId)

	resp := a.mw.ObjectOpen(context.Background(), &pb.RpcObjectOpenRequest{
		SpaceId:  spaceId,
		ObjectId: workspace.WorkspaceObjectId,
	})

	if resp.Error.Code != pb.RpcObjectOpenResponseError_NULL {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to open workspace object."})
		return ""
	}

	if !resp.ObjectView.Details[0].Details.Fields["hasChat"].GetBoolValue() {
		c.JSON(http.StatusNotFound, gin.H{"message": "Chat not found"})
		return ""
	}

	return resp.ObjectView.Details[0].Details.Fields["chatId"].GetStringValue()
}

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
