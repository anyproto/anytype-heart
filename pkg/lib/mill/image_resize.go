package mill

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/dsoprea/go-exif/v3"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"

	"github.com/anyproto/anytype-heart/pkg/lib/mill/ico"

	// Import for image.DecodeConfig to support .webp format
	_ "golang.org/x/image/webp"
)

// Format enumerates the type of images currently supported
type Format string

func init() {
	image.RegisterFormat("ico", string([]byte{0x00, 0x00, 0x01, 0x00}), ico.Decode, ico.DecodeConfig)
}

const (
	JPEG Format = "jpeg"
	PNG  Format = "png"
	GIF  Format = "gif"
	ICO  Format = "ico"
	WEBP Format = "webp"
	HEIC Format = "heic"
)

func IsImage(mime string) bool {
	parts := strings.SplitN(mime, "/", 2)
	if len(parts) == 1 {
		return false
	}
	mimeType := parts[0]
	mimeSubtype := parts[1]
	return mimeType == "image" && isImageFormatSupported(Format(mimeSubtype))
}

func isImageFormatSupported(format Format) bool {
	switch format {
	case JPEG, PNG, GIF, ICO, WEBP, HEIC:
		return true
	}
	return false
}

var ErrFormatSupportNotEnabled = errors.New("this image format support is not enabled in this build")

type ImageSize struct {
	Width  int
	Height int
}

type ImageResizeOpts struct {
	Width   string `json:"width"`
	Quality string `json:"quality"`
}

type ImageResize struct {
	Opts ImageResizeOpts
}

func (m *ImageResize) ID() string {
	return "/image/resize"
}

func (m *ImageResize) Encrypt() bool {
	return true
}

func (m *ImageResize) Pin() bool {
	return false
}

func (m *ImageResize) AcceptMedia(media string) error {
	return accepts([]string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/x-icon",
		"image/webp",
	}, media)
}

func (m *ImageResize) Options(add map[string]interface{}) (string, error) {
	return hashOpts(m.Opts, add)
}

func (m *ImageResize) Mill(r io.ReadSeeker, name string) (*Result, error) {
	imgConfig, formatStr, err := image.DecodeConfig(r)
	if err != nil {
		return nil, err
	}
	format := Format(formatStr)

	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	switch format {
	case JPEG:
		return m.resizeJPEG(&imgConfig, r)
	case ICO, PNG:
		return m.resizePNG(&imgConfig, r)
	case WEBP:
		return m.resizeWEBP(&imgConfig, r)
	case GIF:
		return m.resizeGIF(&imgConfig, r)
	case HEIC:
		return m.resizeHEIC(&imgConfig, r)
	}

	return nil, fmt.Errorf("unknown format")
}

func (m *ImageResize) resizeJPEG(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	width, err := strconv.Atoi(m.Opts.Width)
	if err != nil {
		return nil, fmt.Errorf("invalid width: " + m.Opts.Width)
	}
	quality, err := strconv.Atoi(m.Opts.Quality)
	if err != nil {
		return nil, fmt.Errorf("invalid quality: " + m.Opts.Quality)
	}

	var exifData []byte
	exifData, err = getExifData(r)
	if err != nil {
		return nil, fmt.Errorf("failed to get exif data %s", err.Error())
	}

	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	var orientation int
	var img image.Image

	if exifData != nil {
		orientation, err = getJpegOrientation(exifData)
		if err != nil {
			return nil, fmt.Errorf("failed to get jpeg orientation: %s", err.Error())
		}
		_, err = r.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
	}

	if orientation > 1 {
		img, err = jpeg.Decode(r)
		if err != nil {
			return nil, err
		}

		img = reverseOrientation(img, orientation)
		if err != nil {
			err = fmt.Errorf("failed to fix img orientation: %s", err.Error())
			return nil, err
		}
		imgConfig.Width, imgConfig.Height = img.Bounds().Max.X, img.Bounds().Max.Y
	}

	var height int
	if imgConfig.Width <= width || width == 0 {
		// we will not do the upscale
		width, height = imgConfig.Width, imgConfig.Height
	}

	if orientation <= 1 && width == imgConfig.Width {
		var r2 io.Reader
		r2, err = patchReaderRemoveExif(r)
		if err != nil {
			return nil, err
		}
		// here is an optimization
		// lets return the original picture in case it has not been resized or normalized
		return &Result{
			File: r2,
			Meta: map[string]interface{}{
				"width":  imgConfig.Width,
				"height": imgConfig.Height,
			},
		}, nil
	}

	if img == nil {
		if img, err = jpeg.Decode(r); err != nil {
			return nil, err
		}
	}

	resized := imaging.Resize(img, width, 0, imaging.Lanczos)
	width, height = resized.Rect.Max.X, resized.Rect.Max.Y

	buff := &bytes.Buffer{}
	if err = jpeg.Encode(buff, resized, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}

	return &Result{
		File: buff,
		Meta: map[string]interface{}{
			"width":  width,
			"height": height,
		},
	}, nil
}

func (m *ImageResize) resizePNG(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	var height int
	width, err := strconv.Atoi(m.Opts.Width)
	if err != nil {
		return nil, fmt.Errorf("invalid width: " + m.Opts.Width)
	}

	if imgConfig.Width <= width || width == 0 {
		// we will not do the upscale
		width, height = imgConfig.Width, imgConfig.Height
	}

	if width == imgConfig.Width {
		// here is an optimization
		// lets return the original picture in case it has not been resized or normalized
		return &Result{
			File: r,
			Meta: map[string]interface{}{
				"width":  imgConfig.Width,
				"height": imgConfig.Height,
			},
		}, nil
	}

	img, err := png.Decode(r)
	if err != nil {
		return nil, err
	}

	resized := imaging.Resize(img, width, 0, imaging.Lanczos)
	width, height = resized.Rect.Max.X, resized.Rect.Max.Y

	buff := &bytes.Buffer{}
	if err = png.Encode(buff, resized); err != nil {
		return nil, err
	}

	return &Result{
		File: buff,
		Meta: map[string]interface{}{
			"width":  width,
			"height": height,
		},
	}, nil
}

func (m *ImageResize) resizeGIF(imgConfig *image.Config, r io.ReadSeeker) (*Result, error) {
	width, err := strconv.Atoi(m.Opts.Width)
	if err != nil {
		return nil, fmt.Errorf("invalid width: " + m.Opts.Width)
	}

	if imgConfig.Width <= width || width == 0 {
		// we will not do the upscale
		width = imgConfig.Width
	}

	if width == imgConfig.Width {
		// here is an optimization
		// lets return the original picture in case it has not been resized or normalized
		return &Result{
			File: r,
			Meta: map[string]interface{}{
				"width":  imgConfig.Width,
				"height": imgConfig.Height,
			},
		}, nil
	}

	gifImg, err := gif.DecodeAll(r)
	if err != nil {
		return nil, err
	}

	rect := image.Rect(0, 0, imgConfig.Width, imgConfig.Height)
	rgba := image.NewRGBA(rect)

	for index, frame := range gifImg.Image {
		bounds := frame.Bounds()
		draw.Draw(rgba, bounds, frame, bounds.Min, draw.Over)
		gifImg.Image[index] = imageToPaletted(imaging.Resize(rgba, width, 0, imaging.Lanczos))
	}
	gifImg.Config.Width, gifImg.Config.Height = gifImg.Image[0].Bounds().Dx(), gifImg.Image[0].Bounds().Dy()

	buff := bytes.NewBuffer(make([]byte, 0))
	if err = gif.EncodeAll(buff, gifImg); err != nil {
		return nil, err
	}

	return &Result{
		File: buff,
		Meta: map[string]interface{}{
			"width":  gifImg.Config.Width,
			"height": gifImg.Config.Height,
		},
	}, nil
}

func getExifData(r io.ReadSeeker) (data []byte, err error) {
	exifData, err := exif.SearchAndExtractExifWithReader(r)
	if err != nil {
		if err == exif.ErrNoExif {
			return nil, nil
		}
		return nil, err
	}

	return exifData, nil
}

func getJpegOrientation(exifData []byte) (int, error) {
	tags, _, err := exif.GetFlatExifData(exifData, nil)
	if err != nil {
		return 0, err
	}
	var orientation int
	for _, tag := range tags {
		if tag.TagId != 274 {
			continue
		}
		if v, ok := tag.Value.([]uint16); ok && len(v) == 1 {
			orientation = int(v[0])
		}
	}

	return orientation, nil
}

// reverseOrientation transforms the given orientation to 1
func reverseOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 1:
		return imaging.Clone(img)
	case 2:
		return imaging.FlipV(img)
	case 3:
		return imaging.Rotate180(img)
	case 4:
		return imaging.Rotate180(imaging.FlipV(img))
	case 5:
		return imaging.Rotate270(imaging.FlipV(img))
	case 6:
		return imaging.Rotate270(img)
	case 7:
		return imaging.Rotate90(imaging.FlipV(img))
	case 8:
		return imaging.Rotate90(img)
	}

	log.Warnf("unknown orientation %s, expected 1-8", orientation)
	return imaging.Clone(img)
}

// imageToPaletted convert Image to Paletted for GIF handling
func imageToPaletted(img image.Image) *image.Paletted {
	b := img.Bounds()
	pm := image.NewPaletted(b, palette.Plan9)
	draw.FloydSteinberg.Draw(pm, b, img, image.ZP)
	return pm
}

func patchReaderRemoveExif(r io.ReadSeeker) (io.Reader, error) {
	jmp := jpegstructure.NewJpegMediaParser()
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	_, _ = r.Seek(0, io.SeekStart)

	buff := bytes.NewBuffer(make([]byte, 0, size))
	intfc, err := jmp.Parse(r, int(size))
	if err != nil {
		return nil, fmt.Errorf("failed to open file to read exif: %s", err.Error())
	}
	sl := intfc.(*jpegstructure.SegmentList)

	_, err = sl.DropExif()
	if err != nil {
		return nil, err
	}

	err = sl.Write(buff)
	if err != nil {
		return nil, err
	}

	return buff, nil
}
