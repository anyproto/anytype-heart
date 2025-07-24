package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pb"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFileNotFound    = errors.New("file not found")
	ErrFailedListFiles = errors.New("failed to retrieve list of files")
	ErrFailedGetFile   = errors.New("failed to retrieve file")
)

// ListFiles retrieves a paginated list of file objects in a specific space.
func (s *Service) ListFiles(ctx context.Context, spaceId string, additionalFilters []*model.BlockContentDataviewFilter, offset int, limit int) (files []apimodel.File, total int, hasMore bool, err error) {
	filters := append([]*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyResolvedLayout.String(),
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       pbtypes.IntList(util.LayoutsToIntArgs(util.FileLayouts)...),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.Bool(true),
		},
	}, additionalFilters...)

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: filters,
		Sorts: []*model.BlockContentDataviewSort{{
			RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
			Type:           model.BlockContentDataviewSort_Desc,
			Format:         model.RelationFormat_longtext,
			IncludeTime:    true,
			EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
		}},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedListFiles
	}

	total = len(resp.Records)
	paginatedFiles, hasMore := pagination.Paginate(resp.Records, offset, limit)
	files = make([]apimodel.File, 0, len(paginatedFiles))

	// pre-fetch properties, types and tags to fill the files
	propertyMap, err := s.getPropertyMapFromStore(ctx, spaceId, true)
	if err != nil {
		return nil, 0, false, err
	}
	typeMap, err := s.getTypeMapFromStore(ctx, spaceId, propertyMap, false)
	if err != nil {
		return nil, 0, false, err
	}

	tagMap, err := s.getTagMapFromStore(ctx, spaceId)
	if err != nil {
		return nil, 0, false, err
	}

	for _, record := range paginatedFiles {
		files = append(files, s.getFileFromStruct(record, propertyMap, typeMap, tagMap))
	}
	return files, total, hasMore, nil
}

// GetFile retrieves a single file object by its ID in a specific space.
func (s *Service) GetFile(ctx context.Context, spaceId string, fileId string) (apimodel.File, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: fileId,
	})

	if resp.Error != nil {
		switch resp.Error.Code {
		case pb.RpcObjectShowResponseError_NOT_FOUND:
			return apimodel.File{}, ErrFileNotFound
		default:
			return apimodel.File{}, ErrFailedGetFile
		}
	}

	// Check if the object is a file type
	layoutValue := pbtypes.GetInt64(resp.ObjectView.Details[0].Details, bundle.RelationKeyResolvedLayout.String())
	layout := model.ObjectTypeLayout(layoutValue)
	if !util.IsFileLayout(layout) {
		return apimodel.File{}, ErrFileNotFound
	}

	// pre-fetch properties and types
	propertyMap, err := s.getPropertyMapFromStore(ctx, spaceId, true)
	if err != nil {
		return apimodel.File{}, err
	}
	typeMap, err := s.getTypeMapFromStore(ctx, spaceId, propertyMap, false)
	if err != nil {
		return apimodel.File{}, err
	}
	tagMap, err := s.getTagMapFromStore(ctx, spaceId)
	if err != nil {
		return apimodel.File{}, err
	}

	return s.getFileFromStruct(resp.ObjectView.Details[0].Details, propertyMap, typeMap, tagMap), nil
}

func (s *Service) getFileFromStruct(details *types.Struct, propertyMap map[string]*apimodel.Property, typeMap map[string]*apimodel.Type, tagMap map[string]apimodel.Tag) apimodel.File {
	fileID := pbtypes.GetString(details, bundle.RelationKeyId.String())
	return apimodel.File{
		Object:     "file",
		Id:         fileID,
		Name:       pbtypes.GetString(details, bundle.RelationKeyName.String()),
		SpaceId:    pbtypes.GetString(details, bundle.RelationKeySpaceId.String()),
		Layout:     s.fileLayoutToString(model.ObjectTypeLayout(pbtypes.GetInt64(details, bundle.RelationKeyResolvedLayout.String()))),
		Type:       s.getTypeFromMap(details, typeMap),
		URL:        fmt.Sprintf("%s/image/%s", s.gatewayUrl, fileID),
		Properties: s.getPropertiesFromStruct(details, propertyMap, tagMap),
	}
}

func (s *Service) fileLayoutToString(layout model.ObjectTypeLayout) string {
	switch layout {
	case model.ObjectType_image:
		return string(apimodel.ObjectLayoutImage)
	case model.ObjectType_pdf, model.ObjectType_file, model.ObjectType_audio, model.ObjectType_video:
		return string(apimodel.ObjectLayoutFile)
	default:
		return string(apimodel.ObjectLayoutFile)
	}
}
