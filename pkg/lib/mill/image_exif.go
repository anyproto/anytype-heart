package mill

import (
	"bytes"
	"encoding/json"
	"image"
	"path/filepath"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

type ImageExifSchema struct {
	Created   time.Time `json:"created,omitempty"`
	Name      string    `json:"name"`
	Ext       string    `json:"extension"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Format    string    `json:"format"`
	Latitude  float64   `json:"latitude,omitempty"`
	Longitude float64   `json:"longitude,omitempty"`
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

func (m *ImageExif) Mill(input []byte, name string) (*Result, error) {
	conf, formatStr, err := image.DecodeConfig(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	format := Format(formatStr)

	var created time.Time
	var lat, lon float64

	exf, err := exif.Decode(bytes.NewReader(input))
	if err == nil {
		createdTmp, err := exf.DateTime()
		if err == nil {
			created = createdTmp
		}

		latTmp, lonTmp, err := exf.LatLong()
		if err == nil {
			lat, lon = latTmp, lonTmp
		}
	}

	res := &ImageExifSchema{
		Created:   created,
		Name:      name,
		Ext:       strings.ToLower(filepath.Ext(name)),
		Format:    string(format),
		Width:     conf.Width,
		Height:    conf.Height,
		Latitude:  lat,
		Longitude: lon,
	}

	data, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}

	return &Result{File: data}, nil
}
