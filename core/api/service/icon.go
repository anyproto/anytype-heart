package service

import (
	"fmt"
	"unicode"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
)

// isEmoji checks if the given string is a valid emoji.
func isEmoji(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r) {
			continue
		} else {
			return false
		}
	}
	return true
}

// getIcon returns the appropriate apimodel.Icon based on the provided parameters.
// File-backed icons are served by the API itself via the proxy endpoint
// (GET /v1/spaces/{space_id}/images/{image_id}), so the URL inherits the same
// bearer-token auth as the rest of the API.
func (s *Service) getIcon(spaceId string, iconEmoji string, iconImage string, iconName string, iconOption float64) *apimodel.Icon {
	if iconName != "" {
		return &apimodel.Icon{WrappedIcon: apimodel.NamedIcon{
			Format: apimodel.IconFormatIcon,
			Name:   apimodel.IconName(iconName),
			Color:  apimodel.IconOptionToColor[iconOption],
		}}
	}

	if iconEmoji != "" {
		return &apimodel.Icon{WrappedIcon: apimodel.EmojiIcon{
			Format: apimodel.IconFormatEmoji,
			Emoji:  iconEmoji,
		}}
	}

	if iconImage != "" {
		return &apimodel.Icon{WrappedIcon: apimodel.FileIcon{
			Format: apimodel.IconFormatFile,
			File:   fmt.Sprintf("%s/v1/spaces/%s/files/%s", s.apiBaseUrl, spaceId, iconImage),
		}}
	}

	return nil
}
