package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Image interface {
	FileId() domain.FileId
	Details(ctx context.Context) (*types.Struct, error)
	GetFileForWidth(wantWidth int) (File, error)
	GetOriginalFile() (File, error)
}

var _ Image = (*image)(nil)

type image struct {
	fileId             domain.FileId
	spaceID            string
	onlyResizeVariants []*storage.FileInfo
	service            *service
}

func selectAndSortResizeVariants(variants []*storage.FileInfo) []*storage.FileInfo {
	onlyResizeVariants := variants[:0]
	for _, variant := range variants {
		if variant.Mill == mill.ImageResizeId || variant.Mill == mill.BlobId {
			onlyResizeVariants = append(onlyResizeVariants, variant)
		}
	}

	// Sort by width
	sort.Slice(onlyResizeVariants, func(i, j int) bool {
		return getVariantWidth(onlyResizeVariants[i]) < getVariantWidth(onlyResizeVariants[j])
	})
	return onlyResizeVariants
}

func (i *image) listResizeVariants() ([]*storage.FileInfo, error) {
	if i.onlyResizeVariants != nil {
		return i.onlyResizeVariants, nil
	}
	variants, err := i.service.fileStore.ListFileVariants(i.fileId)
	if err != nil {
		return nil, fmt.Errorf("get variants: %w", err)
	}
	i.onlyResizeVariants = selectAndSortResizeVariants(variants)
	return i.onlyResizeVariants, nil
}

func (i *image) getLargestVariant() (*storage.FileInfo, error) {
	onlyResizeVariants, err := i.listResizeVariants()
	if err != nil {
		return nil, fmt.Errorf("list resize variants: %w", err)
	}
	if len(onlyResizeVariants) == 0 {
		return nil, errors.New("no resize variants")
	}
	return onlyResizeVariants[len(onlyResizeVariants)-1], nil
}

func (i *image) getVariantForWidth(wantWidth int) (*storage.FileInfo, error) {
	onlyResizeVariants, err := i.listResizeVariants()
	if err != nil {
		return nil, fmt.Errorf("list resize variants: %w", err)
	}

	if len(onlyResizeVariants) == 0 {
		return nil, errors.New("no resize variants")
	}

	for _, variant := range onlyResizeVariants {
		if getVariantWidth(variant) >= wantWidth {
			return variant, nil
		}
	}
	// return largest if no more suitable variant found
	return onlyResizeVariants[len(onlyResizeVariants)-1], nil
}

func getVariantWidth(variantInfo *storage.FileInfo) int {
	return int(pbtypes.GetInt64(variantInfo.Meta, "width"))
}

func (i *image) GetFileForWidth(wantWidth int) (File, error) {
	variant, err := i.getVariantForWidth(wantWidth)
	if err != nil {
		return nil, fmt.Errorf("get variant for width: %w", err)
	}
	return &file{
		spaceID: i.spaceID,
		fileId:  i.fileId,
		info:    variant,
		node:    i.service,
	}, nil
}

// GetOriginalFile doesn't contains Meta
func (i *image) GetOriginalFile() (File, error) {
	variant, err := i.getLargestVariant()
	if err != nil {
		return nil, fmt.Errorf("get largest variant: %w", err)
	}
	return &file{
		spaceID: i.spaceID,
		fileId:  i.fileId,
		info:    variant,
		node:    i.service,
	}, nil
}

func (i *image) FileId() domain.FileId {
	return i.fileId
}

func (i *image) getExif(ctx context.Context) (*mill.ImageExifSchema, error) {
	variants, err := i.service.fileStore.ListFileVariants(i.fileId)
	if err != nil {
		return nil, fmt.Errorf("get variants: %w", err)
	}
	var variant *storage.FileInfo
	for _, v := range variants {
		if v.Mill == mill.ImageExifId {
			variant = v
		}
	}
	if variant == nil {
		return nil, fmt.Errorf("exif variant not found")
	}

	f := &file{
		spaceID: i.spaceID,
		fileId:  i.fileId,
		info:    variant,
		node:    i.service,
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

func (i *image) Details(ctx context.Context) (*types.Struct, error) {
	imageExif, err := i.getExif(ctx)
	if err != nil {
		log.Errorf("failed to get exif for image: %s", err)
		imageExif = &mill.ImageExifSchema{}
	}

	commonDetails := calculateCommonDetails(
		i.fileId,
		model.ObjectType_image,
		i.extractLastModifiedDate(imageExif),
	)

	details := &types.Struct{
		Fields: commonDetails,
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	largest, err := i.getLargestVariant()
	if err != nil {
		return details, nil
	}

	if v := pbtypes.Get(largest.GetMeta(), "width"); v != nil {
		details.Fields[bundle.RelationKeyWidthInPixels.String()] = v
	}

	if v := pbtypes.Get(largest.GetMeta(), "height"); v != nil {
		details.Fields[bundle.RelationKeyHeightInPixels.String()] = v
	}

	if largest.Meta != nil {
		details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(strings.TrimSuffix(largest.Name, filepath.Ext(largest.Name)))
		details.Fields[bundle.RelationKeyFileExt.String()] = pbtypes.String(strings.TrimPrefix(filepath.Ext(largest.Name), "."))
		details.Fields[bundle.RelationKeyFileMimeType.String()] = pbtypes.String(largest.Media)
		details.Fields[bundle.RelationKeySizeInBytes.String()] = pbtypes.Float64(float64(largest.Size_))
		details.Fields[bundle.RelationKeyAddedDate.String()] = pbtypes.Float64(float64(largest.Added))
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

func (i *image) extractLastModifiedDate(imageExif *mill.ImageExifSchema) int64 {
	var lastModifiedDate int64
	largest, err := i.getLargestVariant()
	if err == nil {
		lastModifiedDate = largest.LastModifiedDate
	}
	if lastModifiedDate <= 0 {
		lastModifiedDate = imageExif.Created.Unix()
	}

	if lastModifiedDate <= 0 && err == nil {
		lastModifiedDate = largest.Added
	}

	if lastModifiedDate <= 0 {
		lastModifiedDate = time.Now().Unix()
	}

	return lastModifiedDate
}
