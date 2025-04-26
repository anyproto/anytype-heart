package object

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type Service interface {
	ListObjects(ctx context.Context, spaceId string, offset int, limit int) ([]Object, int, bool, error)
	GetObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error)
	CreateObject(ctx context.Context, spaceId string, request CreateObjectRequest) (ObjectWithBlocks, error)
	UpdateObject(ctx context.Context, spaceId string, objectId string, request UpdateObjectRequest) (ObjectWithBlocks, error)
	DeleteObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error)
	GetObjectExport(ctx context.Context, spaceId string, objectId string, format string) (string, error)

	ListProperties(ctx context.Context, spaceId string, offset int, limit int) ([]Property, int, bool, error)
	GetProperty(ctx context.Context, spaceId string, propertyId string) (Property, error)
	CreateProperty(ctx context.Context, spaceId string, request CreatePropertyRequest) (Property, error)
	UpdateProperty(ctx context.Context, spaceId string, propertyId string, request UpdatePropertyRequest) (Property, error)
	DeleteProperty(ctx context.Context, spaceId string, propertyId string) (Property, error)

	ListTags(ctx context.Context, spaceId string, propertyId string, offset int, limit int) ([]Tag, int, bool, error)
	GetTag(ctx context.Context, spaceId string, propertyId string, tagId string) (Tag, error)
	CreateTag(ctx context.Context, spaceId string, propertyId string, request CreateTagRequest) (Tag, error)
	UpdateTag(ctx context.Context, spaceId string, propertyId string, tagId string, request UpdateTagRequest) (Tag, error)
	DeleteTag(ctx context.Context, spaceId string, propertyId string, tagId string) (Tag, error)

	ListTypes(ctx context.Context, spaceId string, offset int, limit int) ([]Type, int, bool, error)
	GetType(ctx context.Context, spaceId string, typeId string) (Type, error)
	ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) ([]Template, int, bool, error)
	GetTemplate(ctx context.Context, spaceId string, typeId string, templateId string) (Template, error)

	GetObjectFromStruct(details *types.Struct, propertyMap map[string]Property, typeMap map[string]Type, tagMap map[string]Tag) Object
	GetPropertyMapFromStore(spaceId string) (map[string]Property, error)
	GetPropertyMapsFromStore(spaceIds []string) (map[string]map[string]Property, error)
	GetTypeMapFromStore(spaceId string, propertyMap map[string]Property) (map[string]Type, error)
	GetTypeMapsFromStore(spaceIds []string, propertyMap map[string]map[string]Property) (map[string]map[string]Type, error)
	GetTypeFromDetails(details []*model.ObjectViewDetailsSet, typeId string, propertyMap map[string]Property) Type
	GetTagMapFromStore(spaceId string) (map[string]Tag, error)
	GetTagMapsFromStore(spaceIds []string) (map[string]map[string]Tag, error)
}

type service struct {
	mw            apicore.ClientCommands
	gatewayUrl    string
	exportService apicore.ExportService
}

func NewService(mw apicore.ClientCommands, exportService apicore.ExportService, gatewayUrl string) Service {
	return &service{mw: mw, exportService: exportService, gatewayUrl: gatewayUrl}
}
