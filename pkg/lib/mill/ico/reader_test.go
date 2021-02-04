package ico

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Decode(t *testing.T) {
	f, err := os.OpenFile("./favicon.ico", os.O_RDONLY, 0555)
	require.NoError(t, err)
	i, err := Decode(f)
	require.NoError(t, err)
	require.Equal(t, i.Bounds().Max.Y, 32)
}

func Test_DecodeConfig(t *testing.T) {
	f, err := os.OpenFile("./favicon.ico", os.O_RDONLY, 0555)
	require.NoError(t, err)
	ic, err := DecodeConfig(f)
	require.NoError(t, err)
	require.Equal(t, ic.Height, 32)
}
