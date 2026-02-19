package mill

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	exif2 "github.com/dsoprea/go-exif/v3"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/mill/testdata"
)

var errFailedToFindExifMarker = fmt.Errorf("exif: failed to find exif intro marker")

func TestImageResize_Mill_ShouldRotateAndRemoveExif(t *testing.T) {
	configs := []*ImageResize{
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
	for i, cfg := range configs {
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

		err = res.File.Close()
		require.NoError(t, err)

		img, err := jpeg.Decode(bytes.NewReader(b))
		require.NoError(t, err)
		require.Equal(t, 680, img.Bounds().Max.X)
		require.Equal(t, 518, img.Bounds().Max.Y)

		// For original
		if i == 0 {
			d, err := exif2.SearchAndExtractExif(b)
			require.NoError(t, err)
			assert.NotNil(t, d)
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				assert.Equal(t, origImgDump, spew.Sdump(*(img.(*image.YCbCr))))
			})
			// For rest sizes
		} else {
			_, err := exif2.SearchAndExtractExif(b)
			require.Error(t, err, exif2.ErrNoExif)
		}
	}
}

func TestImageResize_Mill(t *testing.T) {
	m := &ImageResize{
		Opts: ImageResizeOpts{
			Width:   "100",
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

		if res.Meta["width"] != 100 {
			t.Errorf("wrong width")
		}

		// ensure exif was removed
		_, err = exif.Decode(res.File)
		if err == nil || (err != io.EOF && err.Error() != errFailedToFindExifMarker.Error()) {
			t.Errorf("exif data was not removed")
		}
		file.Close()
		err = res.File.Close()
		require.NoError(t, err)
	}
}
