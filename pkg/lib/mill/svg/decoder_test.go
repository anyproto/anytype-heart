package svg

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Decode(t *testing.T) {
	resp, err := http.Get("https://www.notion.so/icons/archive_gray.svg")
	if err != nil {
		return
	}
	require.NoError(t, err)
	defer resp.Body.Close()
	i, err := Decode(resp.Body)
	require.NoError(t, err)
	require.Equal(t, i.Bounds().Max.Y, 64)
	require.Equal(t, i.Bounds().Max.X, 64)
}

func Test_DecodeConfig(t *testing.T) {
	resp, err := http.Get("https://www.notion.so/icons/archive_gray.svg")
	if err != nil {
		return
	}
	require.NoError(t, err)
	defer resp.Body.Close()
	c, err := DecodeConfig(resp.Body)
	require.NoError(t, err)
	require.Equal(t, c.Width, 64)
	require.Equal(t, c.Height, 64)
}
