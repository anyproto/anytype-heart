package mill

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/textileio/go-textile/mill/testdata"
)

var errFailedToFindExifMarker = fmt.Errorf("exif: failed to find exif intro marker")

func TestImageResize_Mill(t *testing.T) {
	m := &ImageResize{
		Opts: ImageResizeOpts{
			Width:   "200",
			Quality: "80",
		},
	}

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

		if res.Meta["width"] != 200 {
			t.Errorf("wrong width")
		}

		// ensure exif was removed
		_, err = exif.Decode(bytes.NewReader(res.File))
		if err == nil || (err != io.EOF && err.Error() != errFailedToFindExifMarker.Error()) {
			t.Errorf("exif data was not removed")
		}
	}
}
