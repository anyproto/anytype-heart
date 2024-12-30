package utils

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// GetIconFromEmojiOrImage returns the icon to use for the object, which can be either an emoji or an image url
func GetIconFromEmojiOrImage(accountInfo *model.AccountInfo, iconEmoji string, iconImage string) string {
	if iconEmoji != "" {
		return iconEmoji
	}

	if iconImage != "" {
		return GetGatewayURLForMedia(accountInfo, iconImage, true)
	}

	return ""
}

// GetGatewayURLForMedia returns the URL of file gateway for the media object with the given ID
func GetGatewayURLForMedia(accountInfo *model.AccountInfo, objectId string, isIcon bool) string {
	widthParam := ""
	if isIcon {
		widthParam = "?width=100"
	}
	return fmt.Sprintf("%s/image/%s%s", accountInfo.GatewayUrl, objectId, widthParam)
}
