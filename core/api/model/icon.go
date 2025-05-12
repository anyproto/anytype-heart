package apimodel

import (
	"encoding/json"
	"fmt"
	"unicode"

	"github.com/anyproto/anytype-heart/core/api/util"
)

type IconFormat string

const (
	IconFormatEmoji IconFormat = "emoji"
	IconFormatFile  IconFormat = "file"
	IconFormatIcon  IconFormat = "icon"
)

func (f *IconFormat) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch IconFormat(s) {
	case IconFormatEmoji, IconFormatFile, IconFormatIcon:
		*f = IconFormat(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid icon format: %q", s))
	}
}

type Color string

const (
	ColorGrey   Color = "grey"
	ColorYellow Color = "yellow"
	ColorOrange Color = "orange"
	ColorRed    Color = "red"
	ColorPink   Color = "pink"
	ColorPurple Color = "purple"
	ColorBlue   Color = "blue"
	ColorIce    Color = "ice"
	ColorTeal   Color = "teal"
	ColorLime   Color = "lime"
)

func (c *Color) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch Color(s) {
	case ColorGrey, ColorYellow, ColorOrange, ColorRed, ColorPink, ColorPurple, ColorBlue, ColorIce, ColorTeal, ColorLime:
		*c = Color(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid color: %q", s))
	}
}

var iconOptionToColor = map[float64]Color{
	1:  ColorGrey,
	2:  ColorYellow,
	3:  ColorOrange,
	4:  ColorRed,
	5:  ColorPink,
	6:  ColorPurple,
	7:  ColorBlue,
	8:  ColorIce,
	9:  ColorTeal,
	10: ColorLime,
}

var ColorOptionToColor = map[string]Color{
	"grey":   ColorGrey,
	"yellow": ColorYellow,
	"orange": ColorOrange,
	"red":    ColorRed,
	"pink":   ColorPink,
	"purple": ColorPurple,
	"blue":   ColorBlue,
	"ice":    ColorIce,
	"teal":   ColorTeal,
	"lime":   ColorLime,
}

var ColorToColorOption = map[Color]string{
	ColorGrey:   "grey",
	ColorYellow: "yellow",
	ColorOrange: "orange",
	ColorRed:    "red",
	ColorPink:   "pink",
	ColorPurple: "purple",
	ColorBlue:   "blue",
	ColorIce:    "ice",
	ColorTeal:   "teal",
	ColorLime:   "lime",
}

func StringPtr(s string) *string {
	return &s
}

func ColorPtr(c Color) *Color {
	return &c
}

type Icon struct {
	WrappedIcon `swaggerignore:"true"`
}

func (i Icon) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.WrappedIcon)
}

func (i *Icon) UnmarshalJSON(data []byte) error {
	var raw struct {
		Format IconFormat `json:"format"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch raw.Format {
	case IconFormatEmoji:
		var emojiIcon EmojiIcon
		if err := json.Unmarshal(data, &emojiIcon); err != nil {
			return err
		}
		i.WrappedIcon = emojiIcon
	case IconFormatFile:
		var fileIcon FileIcon
		if err := json.Unmarshal(data, &fileIcon); err != nil {
			return err
		}
		i.WrappedIcon = fileIcon
	case IconFormatIcon:
		var namedIcon NamedIcon
		if err := json.Unmarshal(data, &namedIcon); err != nil {
			return err
		}
		i.WrappedIcon = namedIcon
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid icon format: %q", raw.Format))
	}
	return nil
}

type WrappedIcon interface{ isIcon() }

type EmojiIcon struct {
	Format IconFormat `json:"format" enums:"emoji"` // The format of the icon
	Emoji  string     `json:"emoji" example:"📄"`    // The emoji of the icon
}

func (EmojiIcon) isIcon() {}

type FileIcon struct {
	Format IconFormat `json:"format" enums:"file"`                                                        // The format of the icon
	File   string     `json:"file" example:"bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay"` // The file of the icon
}

func (FileIcon) isIcon() {}

type NamedIcon struct {
	Format IconFormat `json:"format" enums:"icon"`                                                                            // The format of the icon
	Name   string     `json:"name" example:"document"`                                                                        // The name of the icon
	Color  *Color     `json:"color,omitempty" example:"yellow" enums:"grey,yellow,orange,red,pink,purple,blue,ice,teal,lime"` // The color of the icon
}

func (NamedIcon) isIcon() {}

func IsEmoji(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.Is(unicode.Cf, r) || unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r) {
			continue
		} else {
			return false
		}
	}
	return true
}

// GetIcon returns the appropriate Icon implementation.
func GetIcon(gatewayUrl string, iconEmoji string, iconImage string, iconName string, iconOption float64) Icon {
	if iconName != "" {
		return Icon{NamedIcon{
			Format: IconFormatIcon,
			Name:   iconName,
			Color:  ColorPtr(iconOptionToColor[iconOption]),
		}}
	}
	if iconEmoji != "" {
		return Icon{EmojiIcon{
			Format: IconFormatEmoji,
			Emoji:  iconEmoji,
		}}
	}
	if iconImage != "" {
		return Icon{FileIcon{
			Format: IconFormatFile,
			File:   fmt.Sprintf("%s/image/%s", gatewayUrl, iconImage),
		}}
	}

	return Icon{NamedIcon{Format: ""}}
}
