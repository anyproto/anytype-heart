package core

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/mill"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

type Image interface {
	Exif() (*mill.ImageExifSchema, error)
	Hash() string
	Details() (*types.Struct, error)
	GetFileForWidth(ctx context.Context, wantWidth int) (File, error)
	GetFileForLargestWidth(ctx context.Context) (File, error)
	GetOriginalFile(ctx context.Context) (File, error)
}

type image struct {
	hash            string // directory hash
	variantsByWidth map[int]*storage.FileInfo
	service         *files.Service
	artist          string
	Url             string
}

func (i *image) GetFileForWidth(ctx context.Context, wantWidth int) (File, error) {
	if i.variantsByWidth != nil {
		return i.getFileForWidthFromCache(wantWidth)
	}

	if wantWidth > 1920 {
		fileIndex, err := i.service.FileGetInfoForPath("/ipfs/" + i.hash + "/0/original")
		if err == nil {
			return &file{
				hash: fileIndex.Hash,
				info: fileIndex,
				node: i.service,
			}, nil
		}
	}

	sizeName := getSizeForWidth(wantWidth)
	fileIndex, err := i.service.FileGetInfoForPath("/ipfs/" + i.hash + "/0/" + sizeName)
	if err != nil {
		return nil, err
	}

	return &file{
		hash: fileIndex.Hash,
		info: fileIndex,
		node: i.service,
	}, nil
}

// GetOriginalFile doesn't contains Meta
func (i *image) GetOriginalFile(ctx context.Context) (File, error) {
	sizeName := "original"
	fileIndex, err := i.service.FileGetInfoForPath("/ipfs/" + i.hash + "/0/" + sizeName)
	if err == nil {
		return &file{
			hash: fileIndex.Hash,
			info: fileIndex,
			node: i.service,
		}, nil
	}

	// fallback for the old schema without an original
	return i.GetFileForLargestWidth(ctx)
}

func (i *image) GetFileForLargestWidth(ctx context.Context) (File, error) {
	if i.variantsByWidth != nil {
		return i.getFileForWidthFromCache(math.MaxInt32)
	}

	// fallback to large size, because older image nodes don't have an original
	sizeName := "large"
	fileIndex, err := i.service.FileGetInfoForPath("/ipfs/" + i.hash + "/0/" + sizeName)
	if err != nil {
		return nil, err
	}

	return &file{
		hash: fileIndex.Hash,
		info: fileIndex,
		node: i.service,
	}, nil
}

func (i *image) Hash() string {
	return i.hash
}

func (i *image) Exif() (*mill.ImageExifSchema, error) {
	fileIndex, err := i.service.FileGetInfoForPath("/ipfs/" + i.hash + "/0/exif")
	if err != nil {
		return nil, err
	}

	f := &file{
		hash: fileIndex.Hash,
		info: fileIndex,
		node: i.service,
	}
	r, err := f.Reader()
	if err != nil {
		return nil, err
	}

	// todo: there is no timeout for reader
	// pending bug: unmarshal NaN values
	var exif mill.ImageExifSchema
	err = json.NewDecoder(r).Decode(&exif)
	if err != nil {
		return nil, err
	}

	if i.artist != "" {
		exif.Artist = fmt.Sprintf("%s; %s", i.artist, i.Url)
	}

	return &exif, nil
}

func (i *image) Details() (*types.Struct, error) {
	details := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():         pbtypes.String(i.hash),
			bundle.RelationKeyIsReadonly.String(): pbtypes.Bool(true),
			bundle.RelationKeyIconImage.String():  pbtypes.String(i.hash),
			bundle.RelationKeyType.String():       pbtypes.String(bundle.TypeKeyImage.URL()),
			bundle.RelationKeyLayout.String():     pbtypes.Float64(float64(model.ObjectType_image)),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	largest, err := i.GetFileForLargestWidth(ctx)
	if err != nil {
		return details, nil
	}

	if largest.Meta() != nil {
		details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(strings.TrimSuffix(largest.Meta().Name, filepath.Ext(largest.Meta().Name)))
		details.Fields[bundle.RelationKeyFileExt.String()] = pbtypes.String(strings.TrimPrefix(filepath.Ext(largest.Meta().Name), "."))
		details.Fields[bundle.RelationKeyFileMimeType.String()] = pbtypes.String(largest.Meta().Media)
		details.Fields[bundle.RelationKeySizeInBytes.String()] = pbtypes.Float64(float64(largest.Meta().Size))
		details.Fields[bundle.RelationKeyAddedDate.String()] = pbtypes.Float64(float64(largest.Meta().Added.Unix()))

	}
	if v := pbtypes.Get(largest.Info().GetMeta(), "width"); v != nil {
		details.Fields[bundle.RelationKeyWidthInPixels.String()] = v
	}

	if v := pbtypes.Get(largest.Info().GetMeta(), "height"); v != nil {
		details.Fields[bundle.RelationKeyHeightInPixels.String()] = v
	}

	exif, err := i.Exif()
	if err != nil {
		log.Errorf("failed to get exif for image: %s", err.Error())
		exif = &mill.ImageExifSchema{}
	}

	if exif.Width > 0 {
		details.Fields[bundle.RelationKeyWidthInPixels.String()] = pbtypes.Float64(float64(exif.Width))
	}
	if exif.Height > 0 {
		details.Fields[bundle.RelationKeyHeightInPixels.String()] = pbtypes.Float64(float64(exif.Height))
	}
	if !exif.Created.IsZero() {
		details.Fields[bundle.RelationKeyCreatedDate.String()] = pbtypes.Float64(float64(exif.Created.Unix()))
	}
	/*if exif.Latitude != 0.0 {
		details.Fields["latitude"] = pbtypes.Float64(exif.Latitude)
	}
	if exif.Longitude != 0.0 {
		details.Fields["longitude"] = pbtypes.Float64(exif.Longitude)
	}*/
	if exif.CameraModel != "" {
		details.Fields[bundle.RelationKeyCamera.String()] = pbtypes.String(exif.CameraModel)
	}
	if exif.ExposureTime != "" {
		details.Fields[bundle.RelationKeyExposure.String()] = pbtypes.String(exif.ExposureTime)
	}
	if exif.FNumber != 0 {
		details.Fields[bundle.RelationKeyFocalRatio.String()] = pbtypes.Float64(exif.FNumber)
	}
	if exif.ISO != 0 {
		details.Fields[bundle.RelationKeyCameraIso.String()] = pbtypes.Float64(float64(exif.ISO))
	}

	if exif.Artist != "" {
		details.Fields[bundle.RelationKeyArtistPhoto.String()] = pbtypes.String(strings.Split(exif.Artist, "; ")[0])
		details.Fields[bundle.RelationKeyArtistUrl.String()] = pbtypes.String(strings.Split(exif.Artist, "; ")[1])
	}

	return details, nil
}

func (i *image) getFileForWidthFromCache(wantWidth int) (File, error) {
	var maxWidth int
	var maxWidthImage *storage.FileInfo

	var minWidthMatched int
	var minWidthMatchedImage *storage.FileInfo

	for width, fileIndex := range i.variantsByWidth {
		if width >= maxWidth {
			maxWidth = width
			maxWidthImage = fileIndex
		}

		if width > wantWidth &&
			(minWidthMatchedImage == nil || minWidthMatched > width) {
			minWidthMatchedImage = fileIndex
			minWidthMatched = width
		}
	}

	if minWidthMatchedImage != nil {
		return &file{
			hash: minWidthMatchedImage.Hash,
			info: minWidthMatchedImage,
			node: i.service,
		}, nil
	} else if maxWidthImage != nil {
		return &file{
			hash: maxWidthImage.Hash,
			info: maxWidthImage,
			node: i.service,
		}, nil
	}

	return nil, ErrFileNotFound
}

var imageWidthByName = map[string]int{
	"thumb": 100,
	"small": 320,
	"large": 1920,
}

func getSizeForWidth(wantWidth int) string {
	var maxWidth int
	var maxWidthSize string
	for sizeName, width := range imageWidthByName {
		if width >= wantWidth {
			return sizeName
		}
		if width > maxWidth {
			maxWidthSize = sizeName
			maxWidth = width
		}
	}
	return maxWidthSize
}
