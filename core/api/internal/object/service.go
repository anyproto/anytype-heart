package object

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/core/api/apimodel"
)

type Service interface {
	ListObjects(ctx context.Context, spaceId string, offset int, limit int) ([]apimodel.Object, int, bool, error)
	GetObject(ctx context.Context, spaceId string, objectId string) (apimodel.ObjectWithBlocks, error)
	CreateObject(ctx context.Context, spaceId string, request apimodel.CreateObjectRequest) (apimodel.ObjectWithBlocks, error)
	UpdateObject(ctx context.Context, spaceId string, objectId string, request apimodel.UpdateObjectRequest) (apimodel.ObjectWithBlocks, error)
	DeleteObject(ctx context.Context, spaceId string, objectId string) (apimodel.ObjectWithBlocks, error)
	GetObjectExport(ctx context.Context, spaceId string, objectId string, format string) (string, error)

	ListProperties(ctx context.Context, spaceId string, offset int, limit int) ([]apimodel.Property, int, bool, error)
	GetProperty(ctx context.Context, spaceId string, propertyId string) (apimodel.Property, error)
	CreateProperty(ctx context.Context, spaceId string, request apimodel.CreatePropertyRequest) (apimodel.Property, error)
	UpdateProperty(ctx context.Context, spaceId string, propertyId string, request apimodel.UpdatePropertyRequest) (apimodel.Property, error)
	DeleteProperty(ctx context.Context, spaceId string, propertyId string) (apimodel.Property, error)

	ListTags(ctx context.Context, spaceId string, propertyId string, offset int, limit int) ([]apimodel.Tag, int, bool, error)
	GetTag(ctx context.Context, spaceId string, propertyId string, tagId string) (apimodel.Tag, error)
	CreateTag(ctx context.Context, spaceId string, propertyId string, request apimodel.CreateTagRequest) (apimodel.Tag, error)
	UpdateTag(ctx context.Context, spaceId string, propertyId string, tagId string, request apimodel.UpdateTagRequest) (apimodel.Tag, error)
	DeleteTag(ctx context.Context, spaceId string, propertyId string, tagId string) (apimodel.Tag, error)

	ListTypes(ctx context.Context, spaceId string, offset int, limit int) ([]apimodel.Type, int, bool, error)
	GetType(ctx context.Context, spaceId string, typeId string) (apimodel.Type, error)
	CreateType(ctx context.Context, spaceId string, request apimodel.CreateTypeRequest) (apimodel.Type, error)
	UpdateType(ctx context.Context, spaceId string, typeId string, request apimodel.UpdateTypeRequest) (apimodel.Type, error)
	DeleteType(ctx context.Context, spaceId string, typeId string) (apimodel.Type, error)

	ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) ([]apimodel.Object, int, bool, error)
	GetTemplate(ctx context.Context, spaceId string, typeId string, templateId string) (apimodel.ObjectWithBlocks, error)

	GetObjectFromStruct(details *types.Struct, propertyMap map[string]apimodel.Property, typeMap map[string]apimodel.Type, tagMap map[string]apimodel.Tag) apimodel.Object
	GetPropertyMapFromStore(spaceId string) (map[string]apimodel.Property, error)
	GetPropertyMapsFromStore(spaceIds []string) (map[string]map[string]apimodel.Property, error)
	GetTypeMapFromStore(spaceId string, propertyMap map[string]apimodel.Property) (map[string]apimodel.Type, error)
	GetTypeMapsFromStore(spaceIds []string, propertyMap map[string]map[string]apimodel.Property) (map[string]map[string]apimodel.Type, error)
	GetTagMapFromStore(spaceId string) (map[string]apimodel.Tag, error)
	GetTagMapsFromStore(spaceIds []string) (map[string]map[string]apimodel.Tag, error)
}

type service struct {
	mw            apicore.ClientCommands
	gatewayUrl    string
	exportService apicore.ExportService
}

func NewService(mw apicore.ClientCommands, exportService apicore.ExportService, gatewayUrl string) Service {
	return &service{mw: mw, exportService: exportService, gatewayUrl: gatewayUrl}
}
