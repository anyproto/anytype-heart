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

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Image interface {
	FileId() domain.FileId
	Details(ctx context.Context) (*domain.Details, error)
	GetFileForWidth(wantWidth int) (File, error)
	GetOriginalFile() (File, error)
}

var _ Image = (*image)(nil)

type image struct {
	fileId             domain.FileId
	spaceID            string
	onlyResizeVariants []*storage.FileInfo
	exifVariant        *storage.FileInfo
	fileService        Service
}

func NewImage(fileService Service, id domain.FullFileId, variants []*storage.FileInfo) Image {
	var exifVariant *storage.FileInfo
	for _, variant := range variants {
		if variant.Mill == mill.ImageExifId {
			exifVariant = variant
		}
	}
	return &image{
		fileId:             id.FileId,
		spaceID:            id.SpaceId,
		onlyResizeVariants: selectAndSortResizeVariants(variants),
		exifVariant:        exifVariant,
		fileService:        fileService,
	}
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
		varI, varJ := onlyResizeVariants[i], onlyResizeVariants[j]
		widthI, widthJ := getVariantWidth(varI), getVariantWidth(varJ)
		// Sort by width first
		if widthI != widthJ {
			return widthI < widthJ
		}
		// Then by size
		return varI.Size_ < varJ.Size_
	})
	return onlyResizeVariants
}

func (i *image) getLargestVariant() (*storage.FileInfo, error) {
	if len(i.onlyResizeVariants) == 0 {
		return nil, errors.New("no resize variants")
	}
	return i.onlyResizeVariants[len(i.onlyResizeVariants)-1], nil
}

func (i *image) getVariantForWidth(wantWidth int) (*storage.FileInfo, error) {
	if len(i.onlyResizeVariants) == 0 {
		return nil, errors.New("no resize variants")
	}

	for _, variant := range i.onlyResizeVariants {
		if getVariantWidth(variant) >= wantWidth {
			return variant, nil
		}
	}
	// return largest if no more suitable variant found
	return i.onlyResizeVariants[len(i.onlyResizeVariants)-1], nil
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
		spaceID:     i.spaceID,
		fileId:      i.fileId,
		info:        variant,
		fileService: i.fileService,
	}, nil
}

// GetOriginalFile doesn't contains Meta
func (i *image) GetOriginalFile() (File, error) {
	variant, err := i.getLargestVariant()
	if err != nil {
		return nil, fmt.Errorf("get largest variant: %w", err)
	}
	return &file{
		spaceID:     i.spaceID,
		fileId:      i.fileId,
		info:        variant,
		fileService: i.fileService,
	}, nil
}

func (i *image) FileId() domain.FileId {
	return i.fileId
}

func (i *image) getExif(ctx context.Context) (*mill.ImageExifSchema, error) {
	if i.exifVariant == nil {
		return nil, fmt.Errorf("exif variant not found")
	}

	f := &file{
		spaceID:     i.spaceID,
		fileId:      i.fileId,
		info:        i.exifVariant,
		fileService: i.fileService,
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

func (i *image) Details(ctx context.Context) (*domain.Details, error) {
	imageExif, err := i.getExif(ctx)
	if err != nil {
		log.Errorf("failed to get exif for image: %s", err)
		imageExif = &mill.ImageExifSchema{}
	}

	details := calculateCommonDetails(
		i.fileId,
		model.ObjectType_image,
		i.extractLastModifiedDate(imageExif),
	)

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	largest, err := i.getLargestVariant()
	if err != nil {
		return details, nil
	}

	if v := pbtypes.Get(largest.GetMeta(), "width"); v != nil {
		details.SetFloat64(bundle.RelationKeyWidthInPixels, v.GetNumberValue())
	}

	if v := pbtypes.Get(largest.GetMeta(), "height"); v != nil {
		details.SetFloat64(bundle.RelationKeyHeightInPixels, v.GetNumberValue())
	}

	if largest.Meta != nil {
		details.SetString(bundle.RelationKeyName, strings.TrimSuffix(largest.Name, filepath.Ext(largest.Name)))
		details.SetString(bundle.RelationKeyFileExt, strings.TrimPrefix(filepath.Ext(largest.Name), "."))
		details.SetString(bundle.RelationKeyFileMimeType, largest.Media)
		details.SetFloat64(bundle.RelationKeySizeInBytes, float64(largest.Size_))
		details.SetFloat64(bundle.RelationKeyAddedDate, float64(largest.Added))
	}

	if !imageExif.Created.IsZero() {
		details.SetFloat64(bundle.RelationKeyCreatedDate, float64(imageExif.Created.Unix()))
	}
	/*if exif.Latitude != 0.0 {
		details.Set("latitude",  pbtypes.Float64(exif.Latitude))
	}
	if exif.Longitude != 0.0 {
		details.Set("longitude",  pbtypes.Float64(exif.Longitude))
	}*/
	if imageExif.CameraModel != "" {
		details.SetString(bundle.RelationKeyCamera, imageExif.CameraModel)
	}
	if imageExif.ExposureTime != "" {
		details.SetString(bundle.RelationKeyExposure, imageExif.ExposureTime)
	}
	if imageExif.FNumber != 0 {
		details.SetFloat64(bundle.RelationKeyFocalRatio, imageExif.FNumber)
	}
	if imageExif.ISO != 0 {
		details.SetFloat64(bundle.RelationKeyCameraIso, float64(imageExif.ISO))
	}
	if imageExif.Description != "" {
		// use non-empty image description as an image name, because it much uglier to use file names for objects
		details.SetString(bundle.RelationKeyName, imageExif.Description)
	}
	if imageExif.Artist != "" {
		artistName, artistURL := unpackArtist(imageExif.Artist)
		details.SetString(bundle.RelationKeyMediaArtistName, artistName)

		if artistURL != "" {
			details.SetString(bundle.RelationKeyMediaArtistURL, artistURL)
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
