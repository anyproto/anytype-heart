package constant

import (
	"math/rand"
	"slices"
)

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

// OptionColors is the palette in canonical order. Callers that assign colors
// deliberately rather than at random — an AnyBlock JSON bundle declaring a
// select vocabulary, say — cycle it so a vocabulary that names no colors
// still ends up with distinct ones.
func OptionColors() []OptionColor {
	return slices.Clone(colors)
}
