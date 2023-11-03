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

	cv "github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/converter/mock_converter"
	"github.com/anyproto/anytype-heart/core/block/import/creator/mock_creator"
	"github.com/anyproto/anytype-heart/core/block/import/objectid/mock_objectid"
	pbc "github.com/anyproto/anytype-heart/core/block/import/pb"
	"github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/import/web/parsers"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync/mock_filesync"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_ImportSuccess(t *testing.T) {
	i := Import{}

	converter := mock_converter.NewMockConverter(t)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
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
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_creator.NewMockService(t)
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator

	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().SendImportEvents().Return().Times(1)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, err := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.Nil(t, err)
}

func Test_ImportErrorFromConverter(t *testing.T) {
	i := Import{}

	converter := mock_converter.NewMockConverter(t)
	e := cv.NewError(0)
	e.Add(fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(nil, e).Times(1)
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_creator.NewMockService(t)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, err := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "converter error")
}

func Test_ImportErrorFromObjectCreator(t *testing.T) {
	i := Import{}

	converter := mock_converter.NewMockConverter(t)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
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
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_creator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", errors.New("creator error")).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, res)
	// assert.Contains(t, res.Error(), "creator error")
}

func Test_ImportIgnoreErrorMode(t *testing.T) {
	i := Import{}

	converter := mock_converter.NewMockConverter(t)
	e := cv.NewError(0)
	e.Add(fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
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
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_creator.NewMockService(t)
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, res)
	assert.Contains(t, res.Error(), "converter error")
}

func Test_ImportIgnoreErrorModeWithTwoErrorsPerFile(t *testing.T) {
	i := Import{}

	converter := mock_converter.NewMockConverter(t)
	e := cv.NewError(0)
	e.Add(fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
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
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_creator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", errors.New("creator error")).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, res)
	assert.Contains(t, res.Error(), "converter error")
	assert.Contains(t, res.Error(), "converter error", "creator error")
}

func Test_ImportExternalPlugin(t *testing.T) {
	i := Import{}

	i.converters = make(map[string]cv.Converter, 0)

	creator := mock_creator.NewMockService(t)
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
	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                nil,
		Snapshots:             snapshots,
		UpdateExistingObjects: false,
		Type:                  pb.RpcObjectImportRequest_External,
		Mode:                  2,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)
	assert.Nil(t, res)
}

func Test_ImportExternalPluginError(t *testing.T) {
	i := Import{}

	i.converters = make(map[string]cv.Converter, 0)

	creator := mock_creator.NewMockService(t)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                nil,
		Snapshots:             nil,
		UpdateExistingObjects: false,
		Type:                  pb.RpcObjectImportRequest_External,
		Mode:                  2,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)
	assert.NotNil(t, res)
	assert.Contains(t, res.Error(), cv.ErrNoObjectsToImport.Error())
}

func Test_ListImports(t *testing.T) {
	i := Import{}
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = pbc.New(nil, nil)
	creator := mock_creator.NewMockService(t)
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
	i.converters = make(map[string]cv.Converter, 0)
	i.converters[web.Name] = web.NewConverter()

	creator := mock_creator.NewMockService(t)
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

	i.converters = make(map[string]cv.Converter, 0)
	i.converters[web.Name] = web.NewConverter()
	creator := mock_creator.NewMockService(t)
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

	i.converters = make(map[string]cv.Converter, 0)

	i.converters[web.Name] = web.NewConverter()

	creator := mock_creator.NewMockService(t)
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

	i.converters = make(map[string]cv.Converter, 0)
	i.converters[web.Name] = web.NewConverter()

	creator := mock_creator.NewMockService(t)
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
	converter := mock_converter.NewMockConverter(t)
	e := cv.NewCancelError(fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: nil}, e).Times(1)
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, res)
	assert.True(t, errors.Is(res, cv.ErrCancel))
}

func Test_ImportNoObjectToImportError(t *testing.T) {
	i := Import{}
	converter := mock_converter.NewMockConverter(t)
	e := cv.NewFromError(cv.ErrNoObjectsToImport, pb.RpcObjectImportRequest_IGNORE_ERRORS)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: nil}, e).Times(1)
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, res)
	assert.True(t, errors.Is(res, cv.ErrNoObjectsToImport))
}

func Test_ImportNoObjectToImportErrorModeAllOrNothing(t *testing.T) {
	i := Import{}
	converter := mock_converter.NewMockConverter(t)
	e := cv.NewFromError(cv.ErrNoObjectsToImport, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
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
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, res)
	assert.True(t, errors.Is(res, cv.ErrNoObjectsToImport))
}

func Test_ImportNoObjectToImportErrorIgnoreErrorsMode(t *testing.T) {
	i := Import{}
	e := cv.NewFromError(cv.ErrNoObjectsToImport, pb.RpcObjectImportRequest_IGNORE_ERRORS)
	converter := mock_converter.NewMockConverter(t)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
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
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_creator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIDGetter(t)
	idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, res)
	assert.True(t, errors.Is(res, cv.ErrNoObjectsToImport))
}

func Test_ImportErrLimitExceeded(t *testing.T) {
	i := Import{}
	converter := mock_converter.NewMockConverter(t)
	e := cv.NewFromError(cv.ErrLimitExceeded, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
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
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, res)
	assert.True(t, errors.Is(res, cv.ErrLimitExceeded))
}

func Test_ImportErrLimitExceededIgnoreErrorMode(t *testing.T) {
	i := Import{}
	converter := mock_converter.NewMockConverter(t)
	e := cv.NewFromError(cv.ErrLimitExceeded, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
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
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	_, res := i.Import(context.Background(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
		SpaceId:               "space1",
	}, model.ObjectOrigin_import)

	assert.NotNil(t, res)
	assert.True(t, errors.Is(res, cv.ErrLimitExceeded))
}

func TestImport_replaceRelationKeyWithNew(t *testing.T) {
	t.Run("no matching relation id in oldIDToNew map", func(t *testing.T) {
		// given
		i := Import{}
		option := &cv.Snapshot{
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
		option := &cv.Snapshot{
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
		oldIDToNew := map[string]string{"rel-key": "rel-newkey"}

		// when
		i.replaceRelationKeyWithNew(option, oldIDToNew)

		// then
		assert.Equal(t, "newkey", pbtypes.GetString(option.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String()))
	})

	t.Run("no details", func(t *testing.T) {
		// given
		i := Import{}
		option := &cv.Snapshot{
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
		originalRootCollectionID := "rootCollectionID"

		converter := mock_converter.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{RootCollectionID: originalRootCollectionID,
			Snapshots: []*cv.Snapshot{
				{
					Snapshot: &pb.ChangeSnapshot{},
					Id:       originalRootCollectionID,
					SbType:   smartblock.SmartBlockTypePage,
				},
			},
		}, nil).Times(1)
		i.converters = make(map[string]cv.Converter, 0)
		i.converters["Notion"] = converter
		creator := mock_creator.NewMockService(t)
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
		rootCollectionID, err := i.Import(context.Background(), &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  0,
			SpaceId:               "space1",
		}, model.ObjectOrigin_import)

		// then
		assert.Nil(t, err)
		assert.Equal(t, expectedRootCollectionID, rootCollectionID)
	})

	t.Run("return empty root collection id in case of error", func(t *testing.T) {
		// given
		i := Import{}
		expectedRootCollectionID := ""
		originalRootCollectionID := "rootCollectionID"
		creatorError := errors.New("creator error")

		converter := mock_converter.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{RootCollectionID: originalRootCollectionID,
			Snapshots: []*cv.Snapshot{
				{
					Snapshot: &pb.ChangeSnapshot{},
					Id:       originalRootCollectionID,
					SbType:   smartblock.SmartBlockTypePage,
				},
			},
		}, nil).Times(1)
		i.converters = make(map[string]cv.Converter, 0)
		i.converters["Notion"] = converter

		creator := mock_creator.NewMockService(t)
		creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", creatorError).Times(1)
		i.oc = creator

		idGetter := mock_objectid.NewMockIDGetter(t)
		idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		rootCollectionID, err := i.Import(context.Background(), &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  0,
			SpaceId:               "space1",
		}, model.ObjectOrigin_import)

		// then
		assert.NotNil(t, err)
		assert.Equal(t, expectedRootCollectionID, rootCollectionID)
	})

	t.Run("return empty root collection id in case of error from import converter", func(t *testing.T) {
		// given
		i := Import{}
		expectedRootCollectionID := ""
		originalRootCollectionID := "rootCollectionID"
		converterError := cv.NewFromError(errors.New("converter error"), pb.RpcObjectImportRequest_ALL_OR_NOTHING)

		converter := mock_converter.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{RootCollectionID: originalRootCollectionID,
			Snapshots: []*cv.Snapshot{
				{
					Snapshot: &pb.ChangeSnapshot{},
					Id:       originalRootCollectionID,
					SbType:   smartblock.SmartBlockTypePage,
				},
			},
		}, converterError).Times(1)
		i.converters = make(map[string]cv.Converter, 0)
		i.converters["Notion"] = converter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		rootCollectionID, err := i.Import(context.Background(), &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  0,
			SpaceId:               "space1",
		}, model.ObjectOrigin_import)

		// then
		assert.NotNil(t, err)
		assert.Equal(t, expectedRootCollectionID, rootCollectionID)
	})

	t.Run("return empty root collection id in case of error with Ignore_Error mode", func(t *testing.T) {
		// given
		i := Import{}
		expectedRootCollectionID := ""
		originalRootCollectionID := "rootCollectionID"
		converterError := cv.NewFromError(errors.New("converter error"), pb.RpcObjectImportRequest_ALL_OR_NOTHING)

		converter := mock_converter.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&cv.Response{RootCollectionID: originalRootCollectionID,
			Snapshots: []*cv.Snapshot{
				{
					Snapshot: &pb.ChangeSnapshot{},
					Id:       originalRootCollectionID,
					SbType:   smartblock.SmartBlockTypePage,
				},
			},
		}, converterError).Times(1)
		i.converters = make(map[string]cv.Converter, 0)
		i.converters["Notion"] = converter

		creator := mock_creator.NewMockService(t)
		creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
		i.oc = creator

		idGetter := mock_objectid.NewMockIDGetter(t)
		idGetter.EXPECT().GetID(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		rootCollectionID, err := i.Import(context.Background(), &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
			SpaceId:               "space1",
		}, model.ObjectOrigin_import)

		// then
		assert.NotNil(t, err)
		assert.Equal(t, expectedRootCollectionID, rootCollectionID)
	})
}
