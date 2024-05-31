package mill

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/mill/testdata"
)

func TestImageExif_Mill(t *testing.T) {
	m := &ImageExif{}

	for _, i := range testdata.Images {
		file, err := os.Open(i.Path)
		if err != nil {
			t.Fatal(err)
		}

		res, err := m.Mill(file, "test", "")
		if err != nil {
			t.Fatal(err)
		}

		var exif *ImageExifSchema

		if err := json.NewDecoder(res.File).Decode(&exif); err != nil {
			t.Fatal(err)
		}

		if exif.Width != i.Width {
			t.Errorf("wrong width")
		}
		if exif.Height != i.Height {
			t.Errorf("wrong height")
		}
		if exif.Format != i.Format {
			t.Errorf("wrong format")
		}
	}
}

func TestImageExif_Mill_Checksum(t *testing.T) {
	m := &ImageExif{}

	file, err := os.Open("testdata/Landscape_8.jpg")
	require.NoError(t, err)
	defer file.Close()

	res, err := m.Mill(file, "test", "FOO")
	require.NoError(t, err)

	raw1, err := io.ReadAll(res.File)
	require.NoError(t, err)

	res, err = m.Mill(file, "test", "BAR")
	require.NoError(t, err)

	raw2, err := io.ReadAll(res.File)
	require.NoError(t, err)

	// Different checksums produce different results
	require.NotEqual(t, raw1, raw2)
}
