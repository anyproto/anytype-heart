package mill

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/textileio/go-textile/mill/testdata"
)

func TestImageExif_Mill(t *testing.T) {
	m := &ImageExif{}

	for _, i := range testdata.Images {
		file, err := os.Open(i.Path)
		if err != nil {
			t.Fatal(err)
		}

		input, err := ioutil.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		file.Close()

		res, err := m.Mill(input, "test")
		if err != nil {
			t.Fatal(err)
		}

		var exif *ImageExifSchema
		if err := json.Unmarshal(res.File, &exif); err != nil {
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
