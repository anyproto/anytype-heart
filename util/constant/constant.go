package constant

import "math/rand"

const ProfileFile = "profile"

const SvgExt = ".svg"

type OptionColor string

func (c OptionColor) String() string {
	return string(c)
}

const (
	ColorGrey   OptionColor = "grey"
	ColorYellow OptionColor = "yellow"
	ColorOrange OptionColor = "orange"
	ColorRed    OptionColor = "red"
	ColorPink   OptionColor = "pink"
	ColorPurple OptionColor = "purple"
	ColorBlue   OptionColor = "blue"
	ColorIce    OptionColor = "ice"
	ColorTeal   OptionColor = "teal"
	ColorLime   OptionColor = "lime"
)

var colors = []OptionColor{
	ColorGrey, ColorYellow, ColorOrange, ColorRed,
	ColorPink, ColorPurple, ColorBlue, ColorIce,
	ColorTeal, ColorLime,
}

func RandomOptionColor() OptionColor {
	return colors[rand.Intn(len(colors))]
}
