package mill

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/mill/testdata"
)

func TestImageExif_Mill(t *testing.T) {
	m := &ImageExif{}

	for _, i := range testdata.Images {
		file, err := os.Open(i.Path)
		if err != nil {
			t.Fatal(err)
		}

		res, err := m.Mill(file, "test")
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
