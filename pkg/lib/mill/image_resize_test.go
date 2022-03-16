package mill

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	exif2 "github.com/dsoprea/go-exif/v3"
	"github.com/stretchr/testify/require"
	"image"
	"image/jpeg"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/rwcarlsen/goexif/exif"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/mill/testdata"
)

var errFailedToFindExifMarker = fmt.Errorf("exif: failed to find exif intro marker")

func TestImageResize_Mill_ShouldRotateAndRemoveExif(t *testing.T) {
	configs := []*ImageResize{
		{
			Opts: ImageResizeOpts{
				Width:   "0",
				Quality: "100",
			},
		},
		{
			Opts: ImageResizeOpts{
				Width:   "2200",
				Quality: "100",
			},
		},
		{
			Opts: ImageResizeOpts{
				Width:   "1800",
				Quality: "100",
			},
		},
	}

	for _, cfg := range configs {

		file, err := os.Open(testdata.Images[0].Path)
		if err != nil {
			t.Fatal(err)
		}
		imgCfg, err := jpeg.DecodeConfig(file)

		// the picture is rotated 90 degrees
		require.Equal(t, 1200, imgCfg.Width)
		require.Equal(t, 1800, imgCfg.Height)

		file.Seek(0, io.SeekStart)

		res, err := cfg.Mill(file, "test")
		if err != nil {
			t.Fatal(err)
		}

		b, err := ioutil.ReadAll(res.File)
		if err != nil {
			t.Fatal(err)
		}

		imgCfg, err = jpeg.DecodeConfig(bytes.NewReader(b))
		require.NoError(t, err)
		require.Equal(t, 1800, imgCfg.Width)
		require.Equal(t, 1200, imgCfg.Height)

		d, err := exif2.SearchAndExtractExif(b)
		require.Error(t, exif2.ErrNoExif, err)
		require.Nil(t, d)
	}
}

func TestImageResize_Mill_ShouldNotBeReencoded(t *testing.T) {
	configs := []*ImageResize{
		{
			Opts: ImageResizeOpts{
				Width:   "0",
				Quality: "100",
			},
		},
		{
			Opts: ImageResizeOpts{
				Width:   "680", // same
				Quality: "80",
			},
		},
		{
			Opts: ImageResizeOpts{
				Width:   "1000", // larger
				Quality: "100",
			},
		},
	}

	file, err := os.Open(testdata.Images[1].Path)
	if err != nil {
		t.Fatal(err)
	}
	origImg, err := jpeg.Decode(file)
	origImgDump := spew.Sdump(*(origImg.(*image.YCbCr)))
	for _, cfg := range configs {
		file, err := os.Open(testdata.Images[1].Path)
		if err != nil {
			t.Fatal(err)
		}
		imgCfg, err := jpeg.DecodeConfig(file)

		// the picture is rotated 90 degrees
		require.Equal(t, 680, imgCfg.Width)
		require.Equal(t, 518, imgCfg.Height)

		file.Seek(0, io.SeekStart)

		res, err := cfg.Mill(file, "test")
		if err != nil {
			t.Fatal(err)
		}

		b, err := ioutil.ReadAll(res.File)
		if err != nil {
			t.Fatal(err)
		}

		img, err := jpeg.Decode(bytes.NewReader(b))
		require.NoError(t, err)
		require.Equal(t, 680, img.Bounds().Max.X)
		require.Equal(t, 518, img.Bounds().Max.Y)

		d, err := exif2.SearchAndExtractExif(b)
		require.Error(t, exif2.ErrNoExif, err)
		require.Nil(t, d)
		require.Equal(t, origImgDump, spew.Sdump(*(img.(*image.YCbCr))))
	}
}

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

		res, err := m.Mill(file, "test")
		if err != nil {
			t.Fatal(err)
		}

		if res.Meta["width"] != 200 {
			t.Errorf("wrong width")
		}

		// ensure exif was removed
		_, err = exif.Decode(res.File)
		if err == nil || (err != io.EOF && err.Error() != errFailedToFindExifMarker.Error()) {
			t.Errorf("exif data was not removed")
		}
		file.Close()
	}
}

func Test_patchReaderRemoveExif(t *testing.T) {
	f, err := os.Open(testdata.Images[0].Path)
	s, _ := f.Stat()
	fmt.Println(s.Size())
	require.NoError(t, err)
	_, err = getExifData(f)
	require.NoError(t, err)
	f.Seek(0, io.SeekStart)

	clean, err := patchReaderRemoveExif(f)
	require.NoError(t, err)

	b, err := ioutil.ReadAll(clean)
	require.NoError(t, err)
	_, _, err = image.Decode(bytes.NewReader(b))
	require.NoError(t, err)
}
