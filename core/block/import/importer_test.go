package importer

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/mock_common"
	"github.com/anyproto/anytype-heart/core/block/import/common/objectcreator/mock_objectcreator"
	"github.com/anyproto/anytype-heart/core/block/import/common/objectid/mock_objectid"
	pbc "github.com/anyproto/anytype-heart/core/block/import/pb"
	"github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/import/web/parsers"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync/mock_filesync"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_ImportSuccess(t *testing.T) {
	i := Import{}

	converter := mock_common.NewMockConverter(t)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{&model.Block{
					Id: "1",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{
							Text:  "test",
							Style: model.BlockContentText_Numbered,
						},
					},
				},
				},
			},
		},
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, nil).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator

	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().SendImportEvents().Return().Times(1)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.Nil(t, res.Err)
	assert.Equal(t, int64(1), res.ObjectsCount)
}

func Test_ImportErrorFromConverter(t *testing.T) {
	i := Import{}

	converter := mock_common.NewMockConverter(t)
	e := common.NewError(0)
	e.Add(fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(nil, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.Contains(t, res.Err.Error(), "converter error")
	assert.Equal(t, int64(0), res.ObjectsCount)
}

func Test_ImportErrorFromObjectCreator(t *testing.T) {
	i := Import{}

	converter := mock_common.NewMockConverter(t)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{&model.Block{
					Id: "1",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{
							Text:  "test",
							Style: model.BlockContentText_Numbered,
						},
					},
				},
				},
			},
		},
		SbType: smartblock.SmartBlockTypePage,
		Id:     "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, nil).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", errors.New("creator error")).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.Equal(t, int64(0), res.ObjectsCount)
	// assert.Contains(t, res.Err.Error(), "creator error")
}

func Test_ImportIgnoreErrorMode(t *testing.T) {
	i := Import{}

	converter := mock_common.NewMockConverter(t)
	e := common.NewError(0)
	e.Add(fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{&model.Block{
				Id: "1",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "test",
						Style: model.BlockContentText_Numbered,
					},
				},
			},
			},
		},
		},
		SbType: smartblock.SmartBlockTypePage,
		Id:     "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.Equal(t, int64(1), res.ObjectsCount)
	assert.Contains(t, res.Err.Error(), "converter error")
}

func Test_ImportIgnoreErrorModeWithTwoErrorsPerFile(t *testing.T) {
	i := Import{}

	converter := mock_common.NewMockConverter(t)
	e := common.NewError(0)
	e.Add(fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{&model.Block{
					Id: "1",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{
							Text:  "test",
							Style: model.BlockContentText_Numbered,
						},
					},
				},
				},
			},
		},
		SbType: smartblock.SmartBlockTypePage,
		Id:     "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", errors.New("creator error")).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.Contains(t, res.Err.Error(), "converter error")
}

func Test_ImportExternalPlugin(t *testing.T) {
	i := Import{}

	i.converters = make(map[string]common.Converter, 0)

	creator := mock_objectcreator.NewMockService(t)
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().SendImportEvents().Return().Times(1)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	snapshots := make([]*pb.RpcObjectImportRequestSnapshot, 0)
	snapshots = append(snapshots, &pb.RpcObjectImportRequestSnapshot{
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a",
		Snapshot: &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{{
				Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:  "test",
						Style: model.BlockContentText_Numbered,
					},
				},
			},
			},
			Details:        nil,
			FileKeys:       nil,
			ExtraRelations: []*model.Relation{},
			ObjectTypes:    []string{},
			Collections:    nil,
		},
	})
	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                nil,
		Snapshots:             snapshots,
		UpdateExistingObjects: false,
		Type:                  model.Import_External,
		Mode:                  2,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)
	assert.Nil(t, res.Err)
	assert.Equal(t, int64(1), res.ObjectsCount)
}

func Test_ImportExternalPluginError(t *testing.T) {
	i := Import{}

	i.converters = make(map[string]common.Converter, 0)

	creator := mock_objectcreator.NewMockService(t)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                nil,
		Snapshots:             nil,
		UpdateExistingObjects: false,
		Type:                  model.Import_External,
		Mode:                  2,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)
	assert.NotNil(t, res.Err)
	assert.Contains(t, res.Err.Error(), common.ErrNoObjectsToImport.Error())
	assert.Equal(t, int64(0), res.ObjectsCount)
}

func Test_ListImports(t *testing.T) {
	i := Import{}
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = pbc.New(nil, nil, nil)
	creator := mock_objectcreator.NewMockService(t)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	i.idProvider = idGetter
	res, err := i.ListImports(&pb.RpcObjectImportListRequest{})

	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Len(t, res, 1)
	assert.True(t, res[0].Type == pb.RpcObjectImportListImportResponseType(0) || res[1].Type == pb.RpcObjectImportListImportResponseType(0))
}

func Test_ImportWebNoParser(t *testing.T) {
	i := Import{}
	i.converters = make(map[string]common.Converter, 0)
	i.converters[web.Name] = web.NewConverter()

	creator := mock_objectcreator.NewMockService(t)
	i.oc = creator
	i.idProvider = mock_objectid.NewMockIDGetter(t)
	_, _, err := i.ImportWeb(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: "http://example.com"}},
		UpdateExistingObjects: true,
	})

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "unknown url format")
}

func Test_ImportWebFailedToParse(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)

	i.converters = make(map[string]common.Converter, 0)
	i.converters[web.Name] = web.NewConverter()
	creator := mock_objectcreator.NewMockService(t)
	i.oc = creator
	i.idProvider = mock_objectid.NewMockIDGetter(t)
	parser := parsers.NewMockParser(ctrl)
	parser.EXPECT().MatchUrl("http://example.com").Return(true).Times(1)
	parser.EXPECT().ParseUrl("http://example.com").Return(nil, errors.New("failed")).Times(1)

	new := func() parsers.Parser {
		return parser
	}
	parsers.RegisterFunc(new)

	_, _, err := i.ImportWeb(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: "http://example.com"}},
		UpdateExistingObjects: true,
	})

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "failed")
}

func Test_ImportWebSuccess(t *testing.T) {
	i := Import{}
	parsers.Parsers = []parsers.RegisterParser{}
	ctrl := gomock.NewController(t)

	i.converters = make(map[string]common.Converter, 0)

	i.converters[web.Name] = web.NewConverter()

	creator := mock_objectcreator.NewMockService(t)
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter
	parser := parsers.NewMockParser(ctrl)
	parser.EXPECT().MatchUrl("http://example.com").Return(true).Times(1)
	parser.EXPECT().ParseUrl("http://example.com").Return(&model.SmartBlockSnapshotBase{Blocks: []*model.Block{&model.Block{
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "test",
				Style: model.BlockContentText_Numbered,
			},
		},
	}}}, nil).Times(1)

	new := func() parsers.Parser {
		return parser
	}
	parsers.RegisterFunc(new)

	_, _, err := i.ImportWeb(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: "http://example.com"}},
		UpdateExistingObjects: true,
	})

	assert.Nil(t, err)
}

func Test_ImportWebFailedToCreateObject(t *testing.T) {
	i := Import{}
	parsers.Parsers = []parsers.RegisterParser{}

	ctrl := gomock.NewController(t)

	i.converters = make(map[string]common.Converter, 0)
	i.converters[web.Name] = web.NewConverter()

	creator := mock_objectcreator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", errors.New("error")).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter
	parser := parsers.NewMockParser(ctrl)
	parser.EXPECT().MatchUrl("http://example.com").Return(true).Times(1)
	parser.EXPECT().ParseUrl("http://example.com").Return(&model.SmartBlockSnapshotBase{Blocks: []*model.Block{&model.Block{
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "test",
				Style: model.BlockContentText_Numbered,
			},
		},
	}}}, nil).Times(1)

	new := func() parsers.Parser {
		return parser
	}
	parsers.RegisterFunc(new)

	_, _, err := i.ImportWeb(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: "http://example.com"}},
		UpdateExistingObjects: true,
	})

	assert.NotNil(t, err)
	assert.Equal(t, "couldn't create objects", err.Error())
}

func Test_ImportCancelError(t *testing.T) {
	i := Import{}
	converter := mock_common.NewMockConverter(t)
	e := common.NewCancelError(fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: nil}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrCancel))
}

func Test_ImportNoObjectToImportError(t *testing.T) {
	i := Import{}
	converter := mock_common.NewMockConverter(t)
	e := common.NewFromError(common.ErrNoObjectsToImport, pb.RpcObjectImportRequest_IGNORE_ERRORS)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: nil}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrNoObjectsToImport))
}

func Test_ImportNoObjectToImportErrorModeAllOrNothing(t *testing.T) {
	i := Import{}
	converter := mock_common.NewMockConverter(t)
	e := common.NewFromError(common.ErrNoObjectsToImport, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{&model.Block{
					Id: "1",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{
							Text:  "test",
							Style: model.BlockContentText_Numbered,
						},
					},
				},
				},
			},
		},
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrNoObjectsToImport))
}

func Test_ImportNoObjectToImportErrorIgnoreErrorsMode(t *testing.T) {
	i := Import{}
	e := common.NewFromError(common.ErrNoObjectsToImport, pb.RpcObjectImportRequest_IGNORE_ERRORS)
	converter := mock_common.NewMockConverter(t)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{&model.Block{
					Id: "1",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{
							Text:  "test",
							Style: model.BlockContentText_Numbered,
						},
					},
				},
				},
			},
		},
		SbType: smartblock.SmartBlockTypePage,
		Id:     "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrNoObjectsToImport))
}

func Test_ImportErrLimitExceeded(t *testing.T) {
	i := Import{}
	converter := mock_common.NewMockConverter(t)
	e := common.NewFromError(common.ErrLimitExceeded, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{&model.Block{
					Id: "1",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{
							Text:  "test",
							Style: model.BlockContentText_Numbered,
						},
					},
				},
				},
			},
		},
		SbType: smartblock.SmartBlockTypePage,
		Id:     "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrLimitExceeded))
}

func Test_ImportErrLimitExceededIgnoreErrorMode(t *testing.T) {
	i := Import{}
	converter := mock_common.NewMockConverter(t)
	e := common.NewFromError(common.ErrLimitExceeded, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{&model.Block{
					Id: "1",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{
							Text:  "test",
							Style: model.BlockContentText_Numbered,
						},
					},
				},
				},
			},
		},
		SbType: smartblock.SmartBlockTypePage,
		Id:     "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
		SpaceId:               "space1",
	}, objectorigin.Import(model.Import_Notion), nil)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrLimitExceeded))
}

func TestImport_replaceRelationKeyWithNew(t *testing.T) {
	t.Run("no matching relation id in oldIDToNew map", func(t *testing.T) {
		// given
		i := Import{}
		option := &common.Snapshot{
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details: &types.Struct{
						Fields: map[string]*types.Value{
							bundle.RelationKeyRelationKey.String(): pbtypes.String("key"),
						},
					},
				},
			},
			SbType: smartblock.SmartBlockTypeSubObject,
		}
		oldIDToNew := make(map[string]string, 0)

		// when
		i.replaceRelationKeyWithNew(option, oldIDToNew)

		// then
		assert.Equal(t, "key", pbtypes.GetString(option.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String()))
	})
	t.Run("oldIDToNew map have relation id", func(t *testing.T) {
		// given
		i := Import{}
		option := &common.Snapshot{
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details: &types.Struct{
						Fields: map[string]*types.Value{
							bundle.RelationKeyRelationKey.String(): pbtypes.String("key"),
						},
					},
				},
			},
			SbType: smartblock.SmartBlockTypeSubObject,
		}
		oldIDToNew := map[string]string{"key": "newkey"}

		// when
		i.replaceRelationKeyWithNew(option, oldIDToNew)

		// then
		assert.Equal(t, "newkey", pbtypes.GetString(option.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String()))
	})

	t.Run("no details", func(t *testing.T) {
		// given
		i := Import{}
		option := &common.Snapshot{
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details: nil,
				},
			},
			SbType: smartblock.SmartBlockTypeSubObject,
		}
		oldIDToNew := map[string]string{"rel-key": "rel-newkey"}

		// when
		i.replaceRelationKeyWithNew(option, oldIDToNew)

		// then
		assert.Nil(t, option.Snapshot.Data.Details)
	})
}

func Test_ImportRootCollectionInResponse(t *testing.T) {
	t.Run("return root collection id in case of error", func(t *testing.T) {
		// given
		i := Import{}
		expectedRootCollectionID := "id"
		originalRootCollectionID := "rootCollectionId"

		converter := mock_common.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{RootCollectionID: originalRootCollectionID,
			Snapshots: []*common.Snapshot{
				{
					Snapshot: &pb.ChangeSnapshot{},
					Id:       originalRootCollectionID,
					SbType:   smartblock.SmartBlockTypePage,
				},
			},
		}, nil).Times(1)
		i.converters = make(map[string]common.Converter, 0)
		i.converters["Notion"] = converter
		creator := mock_objectcreator.NewMockService(t)
		creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
		i.oc = creator

		idGetter := mock_objectid.NewMockIDGetter(t)
		idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(expectedRootCollectionID, treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().SendImportEvents().Return().Times(1)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  0,
			SpaceId:               "space1",
		}, objectorigin.Import(model.Import_Notion), nil)

		// then
		assert.Nil(t, res.Err)
		assert.Equal(t, expectedRootCollectionID, res.RootCollectionId)
		assert.Equal(t, int64(0), res.ObjectsCount) // doesn't count root collection
	})

	t.Run("return empty root collection id in case of error", func(t *testing.T) {
		// given
		i := Import{}
		expectedRootCollectionId := ""
		originalRootCollectionId := "rootCollectionId"
		creatorError := errors.New("creator error")

		converter := mock_common.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{RootCollectionID: originalRootCollectionId,
			Snapshots: []*common.Snapshot{
				{
					Snapshot: &pb.ChangeSnapshot{},
					Id:       originalRootCollectionId,
					SbType:   smartblock.SmartBlockTypePage,
				},
			},
		}, nil).Times(1)
		i.converters = make(map[string]common.Converter, 0)
		i.converters["Notion"] = converter

		creator := mock_objectcreator.NewMockService(t)
		creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", creatorError).Times(1)
		i.oc = creator

		idGetter := mock_objectid.NewMockIDGetter(t)
		idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  0,
			SpaceId:               "space1",
		}, objectorigin.Import(model.Import_Notion), nil)

		// then
		assert.NotNil(t, res.Err)
		assert.Equal(t, expectedRootCollectionId, res.RootCollectionId)
	})

	t.Run("return empty root collection id in case of error from import converter", func(t *testing.T) {
		// given
		i := Import{}
		expectedRootCollectionId := ""
		originalRootCollectionId := "rootCollectionId"
		converterError := common.NewFromError(errors.New("converter error"), pb.RpcObjectImportRequest_ALL_OR_NOTHING)

		converter := mock_common.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{RootCollectionID: originalRootCollectionId,
			Snapshots: []*common.Snapshot{
				{
					Snapshot: &pb.ChangeSnapshot{},
					Id:       originalRootCollectionId,
					SbType:   smartblock.SmartBlockTypePage,
				},
			},
		}, converterError).Times(1)
		i.converters = make(map[string]common.Converter, 0)
		i.converters["Notion"] = converter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  0,
			SpaceId:               "space1",
		}, objectorigin.Import(model.Import_Notion), nil)

		// then
		assert.NotNil(t, res.Err)
		assert.Equal(t, expectedRootCollectionId, res.RootCollectionId)
	})

	t.Run("return empty root collection id in case of error with Ignore_Error mode", func(t *testing.T) {
		// given
		i := Import{}
		expectedRootCollectionId := ""
		originalRootCollectionId := "rootCollectionId"
		converterError := common.NewFromError(errors.New("converter error"), pb.RpcObjectImportRequest_ALL_OR_NOTHING)

		converter := mock_common.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{RootCollectionID: originalRootCollectionId,
			Snapshots: []*common.Snapshot{
				{
					Snapshot: &pb.ChangeSnapshot{},
					Id:       originalRootCollectionId,
					SbType:   smartblock.SmartBlockTypePage,
				},
			},
		}, converterError).Times(1)
		i.converters = make(map[string]common.Converter, 0)
		i.converters["Notion"] = converter

		creator := mock_objectcreator.NewMockService(t)
		creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
		i.oc = creator

		idGetter := mock_objectid.NewMockIDGetter(t)
		idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
			SpaceId:               "space1",
		}, objectorigin.Import(model.Import_Notion), nil)

		// then
		assert.NotNil(t, res.Err)
		assert.Equal(t, expectedRootCollectionId, res.RootCollectionId)
	})
}
