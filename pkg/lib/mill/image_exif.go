package mill

import (
	"bytes"
	"encoding/json"
	"image"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

type ImageExifSchema struct {
	Created      time.Time `json:"created,omitempty"`
	Name         string    `json:"name"`
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
}

type ImageExif struct{}

func (m *ImageExif) ID() string {
	return "/image/exif"
}

func (m *ImageExif) Encrypt() bool {
	return true
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
	var model, exposureTime string
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
			exposureTimeRat, _ := tag.Rat(0)
			if exposureTimeRat != nil {
				exposureTime = exposureTimeRat.String()
			}
		}

		tag, err = exf.Get(exif.FNumber)
		if tag != nil {
			fNumberRat, _ := tag.Rat(0)
			if fNumberRat != nil {
				fNumber, _ = fNumberRat.Float64()
			}
		}
		tag, err = exf.Get(exif.ISOSpeedRatings)
		if tag != nil {
			iso, _ = tag.Int(0)
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
	}

	b, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}

	return &Result{File: bytes.NewReader(b)}, nil
}
