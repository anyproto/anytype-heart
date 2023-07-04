package files

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	imageObjectHiddenWidth = 256
)

type Image interface {
	Exif(ctx session.Context) (*mill.ImageExifSchema, error)
	Hash() string
	Details(ctx session.Context) (*types.Struct, error)
	GetFileForWidth(ctx session.Context, wantWidth int) (File, error)
	GetFileForLargestWidth(ctx session.Context) (File, error)
	GetOriginalFile(ctx session.Context) (File, error)
}

var _ Image = (*image)(nil)

type image struct {
	hash            string // directory hash
	variantsByWidth map[int]*storage.FileInfo
	service         *service
}

func (i *image) GetFileForWidth(ctx session.Context, wantWidth int) (File, error) {
	if i.variantsByWidth != nil {
		return i.getFileForWidthFromCache(wantWidth)
	}

	if wantWidth > 1920 {
		fileIndex, err := i.service.fileGetInfoForPath(ctx, "/ipfs/"+i.hash+"/0/original")
		if err == nil {
			return &file{
				hash: fileIndex.Hash,
				info: fileIndex,
				node: i.service,
			}, nil
		}
	}

	sizeName := getSizeForWidth(wantWidth)
	fileIndex, err := i.service.fileGetInfoForPath(ctx, "/ipfs/"+i.hash+"/0/"+sizeName)
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
func (i *image) GetOriginalFile(ctx session.Context) (File, error) {
	sizeName := "original"
	fileIndex, err := i.service.fileGetInfoForPath(ctx, "/ipfs/"+i.hash+"/0/"+sizeName)
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

func (i *image) GetFileForLargestWidth(ctx session.Context) (File, error) {
	if i.variantsByWidth != nil {
		return i.getFileForWidthFromCache(math.MaxInt32)
	}

	// fallback to large size, because older image nodes don't have an original
	sizeName := "large"
	fileIndex, err := i.service.fileGetInfoForPath(ctx, "/ipfs/"+i.hash+"/0/"+sizeName)
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

func (i *image) Exif(ctx session.Context) (*mill.ImageExifSchema, error) {
	fileIndex, err := i.service.fileGetInfoForPath(ctx, "/ipfs/"+i.hash+"/0/exif")
	if err != nil {
		return nil, err
	}

	f := &file{
		hash: fileIndex.Hash,
		info: fileIndex,
		node: i.service,
	}
	r, err := f.Reader(ctx)
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

	return &exif, nil
}

func (i *image) Details(ctx session.Context) (*types.Struct, error) {
	imageExif, err := i.Exif(ctx)
	if err != nil {
		log.Errorf("failed to get exif for image: %s", err.Error())
		imageExif = &mill.ImageExifSchema{}
	}

	commonDetails := calculateCommonDetails(
		i.hash,
		bundle.TypeKeyImage,
		model.ObjectType_image,
		i.extractLastModifiedDate(ctx, imageExif),
	)
	commonDetails[bundle.RelationKeyIconImage.String()] = pbtypes.String(i.hash)

	details := &types.Struct{
		Fields: commonDetails,
	}

	cctx, cancel := context.WithTimeout(ctx.Context(), 1*time.Minute)
	ctx = ctx.WithContext(cctx)
	defer cancel()

	largest, err := i.GetFileForLargestWidth(ctx)
	if err != nil {
		return details, nil
	}

	if v := pbtypes.Get(largest.Info().GetMeta(), "width"); v != nil {
		details.Fields[bundle.RelationKeyWidthInPixels.String()] = v
		if v.GetNumberValue() < imageObjectHiddenWidth {
			details.Fields[bundle.RelationKeyIsHidden.String()] = pbtypes.Bool(true)
		}
	}

	if v := pbtypes.Get(largest.Info().GetMeta(), "height"); v != nil {
		details.Fields[bundle.RelationKeyHeightInPixels.String()] = v
	}

	if largest.Meta() != nil {
		details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(strings.TrimSuffix(largest.Meta().Name, filepath.Ext(largest.Meta().Name)))
		details.Fields[bundle.RelationKeyFileExt.String()] = pbtypes.String(strings.TrimPrefix(filepath.Ext(largest.Meta().Name), "."))
		details.Fields[bundle.RelationKeyFileMimeType.String()] = pbtypes.String(largest.Meta().Media)
		details.Fields[bundle.RelationKeySizeInBytes.String()] = pbtypes.Float64(float64(largest.Meta().Size))
		details.Fields[bundle.RelationKeyAddedDate.String()] = pbtypes.Float64(float64(largest.Meta().Added.Unix()))
	}

	if !imageExif.Created.IsZero() {
		details.Fields[bundle.RelationKeyCreatedDate.String()] = pbtypes.Float64(float64(imageExif.Created.Unix()))
	}
	/*if exif.Latitude != 0.0 {
		details.Fields["latitude"] = pbtypes.Float64(exif.Latitude)
	}
	if exif.Longitude != 0.0 {
		details.Fields["longitude"] = pbtypes.Float64(exif.Longitude)
	}*/
	if imageExif.CameraModel != "" {
		details.Fields[bundle.RelationKeyCamera.String()] = pbtypes.String(imageExif.CameraModel)
	}
	if imageExif.ExposureTime != "" {
		details.Fields[bundle.RelationKeyExposure.String()] = pbtypes.String(imageExif.ExposureTime)
	}
	if imageExif.FNumber != 0 {
		details.Fields[bundle.RelationKeyFocalRatio.String()] = pbtypes.Float64(imageExif.FNumber)
	}
	if imageExif.ISO != 0 {
		details.Fields[bundle.RelationKeyCameraIso.String()] = pbtypes.Float64(float64(imageExif.ISO))
	}
	if imageExif.Description != "" {
		// use non-empty image description as an image name, because it much uglier to use file names for objects
		details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(imageExif.Description)
	}
	if imageExif.Artist != "" {
		artistName, artistURL := unpackArtist(imageExif.Artist)
		details.Fields[bundle.RelationKeyMediaArtistName.String()] = pbtypes.String(artistName)

		if artistURL != "" {
			details.Fields[bundle.RelationKeyMediaArtistURL.String()] = pbtypes.String(artistURL)
		}
	}

	return details, nil
}

// exifArtistWithUrl matches and extracts additional information we store in the Artist field – the URL of the author page.
// We use it within the Unsplash integration
var exifArtistWithUrl = regexp.MustCompile(`(.*?); (http.*)`)

func unpackArtist(packed string) (name, url string) {
	artistParts := exifArtistWithUrl.FindStringSubmatch(packed)
	if len(artistParts) == 3 {
		return artistParts[1], artistParts[2]
	}

	return packed, ""
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

func (i *image) extractLastModifiedDate(ctx session.Context, imageExif *mill.ImageExifSchema) int64 {
	var lastModifiedDate int64
	largest, err := i.GetFileForLargestWidth(ctx)
	if err == nil {
		lastModifiedDate = largest.Meta().LastModifiedDate
	}
	if lastModifiedDate == 0 {
		lastModifiedDate = imageExif.Created.Unix()
	}

	if lastModifiedDate == 0 && err == nil {
		lastModifiedDate = largest.Meta().Added.Unix()
	}

	if lastModifiedDate == 0 {
		lastModifiedDate = time.Now().Unix()
	}

	return lastModifiedDate
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
