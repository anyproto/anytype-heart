package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsEmoji(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// valid single-code-point emoji
		{"GrinningFace", "😀", true},
		// yin-yang with variation selector
		{"YinYangWithVS", "☯️", true},
		// emoji + skin tone modifier
		{"ThumbsUpMediumSkinTone", "👍🏽", true},
		// ZWJ sequence (couple kissing)
		{"CoupleKissingZWJ", "👩‍❤️‍💋‍👨", true},
		// string of emojis
		{"MultipleEmojis", "😀😃😄", true},

		// invalid: letters only
		{"Letters", "abc", false},
		// invalid: mixed emoji + letter
		{"EmojiPlusLetter", "😀a", false},
		// invalid: digits
		{"Digit", "1", false},
		// invalid: punctuation
		{"Punctuation", "!", false},
		// invalid: whitespace
		{"Whitespace", " ", false},
		// invalid: empty string
		{"EmptyString", "", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := isEmoji(tc.input)
			require.Equal(t, tc.want, got, "isEmoji(%q)", tc.input)
		})
	}
}
