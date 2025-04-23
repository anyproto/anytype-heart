package object

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

type Icon struct {
	Format IconFormat `json:"format" enums:"emoji,file,icon" example:"emoji"`                                                                    // The type of the icon
	Emoji  *string    `json:"emoji,omitempty" example:"📄"`                                                                                       // The emoji of the icon
	File   *string    `json:"file,omitempty" example:"http://127.0.0.1:31006/image/bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay"` // The file of the icon
	Name   *string    `json:"name,omitempty" example:"document"`                                                                                 // The name of the icon
	Color  *Color     `json:"color,omitempty" example:"yellow" enums:"grey,yellow,orange,red,pink,purple,blue,ice,teal,lime"`                    // The color of the icon
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

// isEmoji returns true if every rune in s is in the Unicode 'Symbol, Other' category.
func IsEmoji(s string) bool {
	for _, r := range s {
		if !unicode.Is(unicode.So, r) {
			return false
		}
	}
	return true
}

// GetIcon returns the icon to use for the object, which can be builtin icon, emoji or file
func GetIcon(gatewayUrl string, iconEmoji string, iconImage string, iconName string, iconOption float64) Icon {
	if iconName != "" {
		return Icon{
			Format: IconFormatIcon,
			Name:   &iconName,
			Color:  ColorPtr(iconOptionToColor[iconOption]),
		}
	}

	if iconEmoji != "" {
		return Icon{
			Format: IconFormatEmoji,
			Emoji:  &iconEmoji,
		}
	}

	if iconImage != "" {
		return Icon{
			Format: IconFormatFile,
			File:   StringPtr(fmt.Sprintf("%s/image/%s", gatewayUrl, iconImage)),
		}
	}

	return Icon{}
}
