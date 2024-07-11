package mill

import (
	"bytes"
	"image"
	"io"
	"math/big"
	"path/filepath"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"

	"github.com/anyproto/anytype-heart/util/jsonutil"
)

type ImageExifSchema struct {
	Created      time.Time `json:"created,omitempty"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Ext          string    `json:"extension"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	Format       string    `json:"format"`
	CameraModel  string    `json:"model,omitempty"`
	ISO          int       `json:"iso"`
	ExposureTime string    `json:"exposure_time"`
	FNumber      float64   `json:"f_number"`

	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`

	Artist string `json:"artist,omitempty"`
}

type ImageExif struct{}

const ImageExifId = "/image/exif"

func (m *ImageExif) ID() string {
	return "/image/exif"
}

func (m *ImageExif) Pin() bool {
	return false
}

func (m *ImageExif) AcceptMedia(media string) error {
	return accepts([]string{
		"image/jpeg",
		"image/png",
		"image/gif",
	}, media)
}

func (m *ImageExif) Options(add map[string]interface{}) (string, error) {
	return hashOpts(make(map[string]string), add)
}

func (m *ImageExif) Mill(r io.ReadSeeker, name string) (*Result, error) {
	conf, formatStr, err := image.DecodeConfig(r)
	if err != nil {
		return nil, err
	}
	format := Format(formatStr)

	var created time.Time
	var model, exposureTime, artist, description string
	var lat, lon, fNumber float64
	var iso int

	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}
	exf, err := exif.Decode(r)
	if err == nil {
		createdTmp, err := exf.DateTime()
		if err == nil {
			created = createdTmp
		}

		latTmp, lonTmp, err := exf.LatLong()
		if err == nil {
			lat, lon = latTmp, lonTmp
		}
		tag, err := exf.Get(exif.Model)
		if tag != nil {
			model, _ = tag.StringVal()
		}

		tag, err = exf.Get(exif.ExposureTime)
		if tag != nil {
			num, denom, err := tag.Rat2(0)
			if err == nil {
				if denom == 0 {
					exposureTime = "inf"
				} else {
					exposureTime = big.NewRat(num, denom).String()
				}
			}
		}

		tag, err = exf.Get(exif.FNumber)
		if tag != nil {
			num, denom, err := tag.Rat2(0)
			if err == nil {
				if denom != 0 {
					fNumber, _ = big.NewRat(num, denom).Float64()
				}
			}
		}
		tag, err = exf.Get(exif.ISOSpeedRatings)
		if tag != nil {
			iso, _ = tag.Int(0)
		}
		tag, err = exf.Get(exif.Artist)
		if tag != nil {
			artist, _ = tag.StringVal()
		}
		tag, err = exf.Get(exif.ImageDescription)
		if tag != nil {
			description, _ = tag.StringVal()
		}
	}

	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	res := &ImageExifSchema{
		Created:      created,
		Name:         name,
		Ext:          strings.ToLower(filepath.Ext(name)),
		Format:       string(format),
		CameraModel:  model,
		ISO:          iso,
		ExposureTime: exposureTime,
		FNumber:      fNumber,
		Width:        conf.Width,
		Height:       conf.Height,
		Latitude:     lat,
		Longitude:    lon,
		Artist:       artist,
		Description:  description,
	}

	b, err := jsonutil.MarshalSafely(res)
	if err != nil {
		return nil, err
	}

	return &Result{File: noopCloser(bytes.NewReader(b))}, nil
}
