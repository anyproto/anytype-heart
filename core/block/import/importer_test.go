package importer

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/import/common/objectid/mock_objectid"
	"github.com/anyproto/anytype-heart/core/block/import/web/parsers/mock_parsers"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/space/mock_space"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/mock_common"
	"github.com/anyproto/anytype-heart/core/block/import/common/objectcreator/mock_objectcreator"
	pbc "github.com/anyproto/anytype-heart/core/block/import/pb"
	"github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/import/web/parsers"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync/mock_filesync"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

func Test_ImportSuccess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		i := Import{}

		converter := mock_common.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
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

		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
		idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().SendImportEvents().Return().Times(1)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		importRequest := &ImportRequest{
			&pb.RpcObjectImportRequest{
				Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
				UpdateExistingObjects: false,
				Type:                  0,
				Mode:                  0,
				SpaceId:               "space1",
			}, objectorigin.Import(model.Import_Notion),
			nil,
			false,
			true}
		res := i.Import(context.Background(), importRequest)

		assert.Nil(t, res.Err)
		assert.Equal(t, int64(1), res.ObjectsCount)
	})
	t.Run("success with notification", func(t *testing.T) {
		// given
		i := Import{}

		converter := mock_common.NewMockConverter(t)
		converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Blocks: []*model.Block{{
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

		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
		idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().SendImportEvents().Return().Times(1)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		notificationsService := mock_notifications.NewMockNotifications(t)

		notification := make(chan *model.Notification)
		notificationsService.EXPECT().CreateAndSend(mock.Anything).Run(func(n *model.Notification) {
			notification <- n
		}).Return(nil)
		notificationProcess := process.NewNotificationProcess(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}}, notificationsService)
		setupProcessService(t, notificationProcess)

		importRequest := &ImportRequest{
			&pb.RpcObjectImportRequest{
				Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
				UpdateExistingObjects: false,
				Type:                  0,
				Mode:                  0,
				SpaceId:               "space1",
			}, objectorigin.Import(model.Import_Notion),
			notificationProcess,
			true,
			true}

		// when
		res := i.Import(context.Background(), importRequest)

		// then
		assert.Nil(t, res.Err)
		assert.Equal(t, int64(1), res.ObjectsCount)
		result := <-notification
		assert.NotEmpty(t, result.Id)
	})
}

func setupProcessService(t *testing.T, notificationProcess process.Progress) {
	s := process.New()
	a := &app.App{}
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Broadcast(mock.Anything).Return()
	sender.EXPECT().BroadcastExceptSessions(mock.Anything, mock.Anything).Return().Maybe()
	a.Register(testutil.PrepareMock(context.Background(), a, sender)).Register(s)
	err := a.Start(context.Background())
	assert.Nil(t, err)
	err = s.Add(notificationProcess)
	assert.Nil(t, err)
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
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  0,
			SpaceId:               "space1",
		},
		objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)

	assert.NotNil(t, res.Err)
	assert.Contains(t, res.Err.Error(), "converter error")
	assert.Equal(t, int64(0), res.ObjectsCount)
}

func Test_ImportErrorFromObjectCreator(t *testing.T) {
	i := Import{}

	converter := mock_common.NewMockConverter(t)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &common.SnapshotModel{
			Data: &common.StateSnapshot{
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
			SbType: smartblock.SmartBlockTypePage,
		},
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, nil).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", errors.New("creator error")).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
	idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	request := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  0,
			SpaceId:               "space1",
		}, objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), request)

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
		Snapshot: &common.SnapshotModel{Data: &common.StateSnapshot{
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
			SbType: smartblock.SmartBlockTypePage,
		},
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
	idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
	idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  1,
			SpaceId:               "space1",
		},
		objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)

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
		Snapshot: &common.SnapshotModel{
			Data: &common.StateSnapshot{
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
			SbType: smartblock.SmartBlockTypePage,
		},
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", errors.New("creator error")).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
	idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
	idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  1,
			SpaceId:               "space1",
		}, objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)

	assert.NotNil(t, res.Err)
	assert.Contains(t, res.Err.Error(), "converter error")
}

func Test_ImportExternalPlugin(t *testing.T) {
	i := Import{}

	i.converters = make(map[string]common.Converter, 0)

	creator := mock_objectcreator.NewMockService(t)
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
	idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
	idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
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
	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                nil,
			Snapshots:             snapshots,
			UpdateExistingObjects: false,
			Type:                  model.Import_External,
			Mode:                  2,
			SpaceId:               "space1",
		}, objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)
	assert.NotNil(t, res)
	assert.Nil(t, res.Err)
}

func Test_ImportExternalPluginError(t *testing.T) {
	i := Import{}

	i.converters = make(map[string]common.Converter, 0)

	creator := mock_objectcreator.NewMockService(t)
	i.oc = creator
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                nil,
			Snapshots:             nil,
			UpdateExistingObjects: false,
			Type:                  model.Import_External,
			Mode:                  2,
			SpaceId:               "space1",
		},
		objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)
	assert.NotNil(t, res)
	assert.Contains(t, res.Err.Error(), common.ErrNoSnapshotToImport.Error())
}

func Test_ListImports(t *testing.T) {
	i := Import{}
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = pbc.New(nil, nil, nil)
	creator := mock_objectcreator.NewMockService(t)
	i.oc = creator
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
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
	i.idProvider = mock_objectid.NewMockIdAndKeyProvider(t)
	_, _, err := i.ImportWeb(context.Background(), &ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: "http://example.com"}},
			UpdateExistingObjects: true,
		},
		Progress: process.NewNoOp(),
	})

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "unknown url format")
}

func Test_ImportWebFailedToParse(t *testing.T) {
	i := Import{}

	i.converters = make(map[string]common.Converter, 0)
	i.converters[web.Name] = web.NewConverter()
	creator := mock_objectcreator.NewMockService(t)
	i.oc = creator
	i.idProvider = mock_objectid.NewMockIdAndKeyProvider(t)
	parser := mock_parsers.NewMockParser(t)
	parser.EXPECT().MatchUrl("http://example.com").Return(true).Times(1)
	parser.EXPECT().ParseUrl("http://example.com").Return(nil, errors.New("failed")).Times(1)

	new := func() parsers.Parser {
		return parser
	}
	parsers.RegisterFunc(new)

	_, _, err := i.ImportWeb(context.Background(), &ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: "http://example.com"}},
			UpdateExistingObjects: true,
		},
		Progress: process.NewNoOp(),
	})

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "failed")
}

func Test_ImportWebSuccess(t *testing.T) {
	i := Import{}
	parsers.Parsers = []parsers.RegisterParser{}

	i.converters = make(map[string]common.Converter, 0)

	i.converters[web.Name] = web.NewConverter()

	creator := mock_objectcreator.NewMockService(t)
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
	idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
	idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter
	parser := mock_parsers.NewMockParser(t)
	parser.EXPECT().MatchUrl("http://example.com").Return(true).Times(1)
	parser.EXPECT().ParseUrl("http://example.com").Return(&common.StateSnapshot{Blocks: []*model.Block{&model.Block{
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

	_, _, err := i.ImportWeb(context.Background(), &ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: "http://example.com"}},
			UpdateExistingObjects: true,
		},
		Progress: process.NewNoOp(),
	})

	assert.Nil(t, err)
}

func Test_ImportWebFailedToCreateObject(t *testing.T) {
	i := Import{}
	parsers.Parsers = []parsers.RegisterParser{}

	i.converters = make(map[string]common.Converter, 0)
	i.converters[web.Name] = web.NewConverter()

	creator := mock_objectcreator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", errors.New("error")).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
	idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
	idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter
	parser := mock_parsers.NewMockParser(t)
	parser.EXPECT().MatchUrl("http://example.com").Return(true).Times(1)
	parser.EXPECT().ParseUrl("http://example.com").Return(&common.StateSnapshot{Blocks: []*model.Block{&model.Block{
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

	_, _, err := i.ImportWeb(context.Background(), &ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: "http://example.com"}},
			UpdateExistingObjects: true,
		},
		Progress: process.NewNoOp(),
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

	res := i.Import(context.Background(), &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
			SpaceId:               "space1",
		},
		objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	})

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrCancel))
}

func Test_ImportNoObjectToImportError(t *testing.T) {
	i := Import{}
	converter := mock_common.NewMockConverter(t)
	e := common.NewFromError(common.ErrNoObjectInIntegration, pb.RpcObjectImportRequest_IGNORE_ERRORS)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: nil}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
			SpaceId:               "space1",
		},
		objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrNoObjectInIntegration))
}

func Test_ImportNoObjectToImportErrorModeAllOrNothing(t *testing.T) {
	i := Import{}
	converter := mock_common.NewMockConverter(t)
	e := common.NewFromError(common.ErrNoObjectInIntegration, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &common.SnapshotModel{
			Data: &common.StateSnapshot{
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

	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
			SpaceId:               "space1",
		},
		objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrNoObjectInIntegration))
}

func Test_ImportNoObjectToImportErrorIgnoreErrorsMode(t *testing.T) {
	i := Import{}
	e := common.NewFromError(common.ErrNoObjectInIntegration, pb.RpcObjectImportRequest_IGNORE_ERRORS)
	converter := mock_common.NewMockConverter(t)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &common.SnapshotModel{
			Data: &common.StateSnapshot{
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
			SbType: smartblock.SmartBlockTypePage,
		},
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter
	creator := mock_objectcreator.NewMockService(t)
	//nolint:lll
	creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
	i.oc = creator
	idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
	idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
	idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
	i.idProvider = idGetter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
			SpaceId:               "space1",
		},
		objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrNoObjectInIntegration))
}

func Test_ImportErrLimitExceeded(t *testing.T) {
	i := Import{}
	converter := mock_common.NewMockConverter(t)
	e := common.NewFromError(common.ErrCsvLimitExceeded, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &common.SnapshotModel{
			Data: &common.StateSnapshot{
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
			SbType: smartblock.SmartBlockTypePage,
		},
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
			SpaceId:               "space1",
		},
		objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrCsvLimitExceeded))
}

func Test_ImportErrLimitExceededIgnoreErrorMode(t *testing.T) {
	i := Import{}
	converter := mock_common.NewMockConverter(t)
	e := common.NewFromError(common.ErrCsvLimitExceeded, pb.RpcObjectImportRequest_ALL_OR_NOTHING)
	converter.EXPECT().GetSnapshots(mock.Anything, mock.Anything, mock.Anything).Return(&common.Response{Snapshots: []*common.Snapshot{{
		Snapshot: &common.SnapshotModel{
			Data: &common.StateSnapshot{
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
			SbType: smartblock.SmartBlockTypePage,
		},
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}}, e).Times(1)
	i.converters = make(map[string]common.Converter, 0)
	i.converters["Notion"] = converter

	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().ClearImportEvents().Return().Times(1)
	i.fileSync = fileSync

	importRequest := &ImportRequest{
		&pb.RpcObjectImportRequest{
			Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"test"}}},
			UpdateExistingObjects: false,
			Type:                  0,
			Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
			SpaceId:               "space1",
		},
		objectorigin.Import(model.Import_Notion),
		nil,
		false,
		true,
	}
	res := i.Import(context.Background(), importRequest)

	assert.NotNil(t, res.Err)
	assert.True(t, errors.Is(res.Err, common.ErrCsvLimitExceeded))
}

func TestImport_replaceRelationKeyWithNew(t *testing.T) {
	t.Run("no matching relation id in oldIDToNew map", func(t *testing.T) {
		// given
		i := Import{}
		option := &common.Snapshot{
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyRelationKey: domain.String("key"),
					}),
				},
				SbType: smartblock.SmartBlockTypeSubObject,
			},
		}
		oldIDToNew := make(map[string]string, 0)

		// when
		i.replaceRelationKeyWithNew(option, oldIDToNew)

		// then
		assert.Equal(t, "key", option.Snapshot.Data.Details.GetString(bundle.RelationKeyRelationKey))
	})
	t.Run("oldIDToNew map have relation id", func(t *testing.T) {
		// given
		i := Import{}
		option := &common.Snapshot{
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyRelationKey: domain.String("key"),
					}),
				},
				SbType: smartblock.SmartBlockTypeSubObject,
			},
		}
		oldIDToNew := map[string]string{"key": "newkey"}

		// when
		i.replaceRelationKeyWithNew(option, oldIDToNew)

		// then
		assert.Equal(t, "newkey", option.Snapshot.Data.Details.GetString(bundle.RelationKeyRelationKey))
	})

	t.Run("no details", func(t *testing.T) {
		// given
		i := Import{}
		option := &common.Snapshot{
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: nil,
				},
				SbType: smartblock.SmartBlockTypeSubObject,
			},
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
					Snapshot: &common.SnapshotModel{
						SbType: smartblock.SmartBlockTypePage,
						Data:   &common.StateSnapshot{},
					},
					Id: originalRootCollectionID,
				},
			},
		}, nil).Times(1)
		i.converters = make(map[string]common.Converter, 0)
		i.converters["Notion"] = converter
		creator := mock_objectcreator.NewMockService(t)
		creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
		i.oc = creator

		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
		idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(expectedRootCollectionID, treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().SendImportEvents().Return().Times(1)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync
		mockSpaceService := mock_space.NewMockService(t)
		mockSpaceService.EXPECT().Get(nil, "space1").Return(nil, fmt.Errorf("not found"))
		i.spaceService = mockSpaceService

		// when
		importRequest := &ImportRequest{
			&pb.RpcObjectImportRequest{
				Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
				UpdateExistingObjects: false,
				Type:                  0,
				Mode:                  0,
				SpaceId:               "space1",
			},
			objectorigin.Import(model.Import_Notion),
			nil,
			false,
			true,
		}
		res := i.Import(context.Background(), importRequest)

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
					Snapshot: &common.SnapshotModel{
						SbType: smartblock.SmartBlockTypePage,
						Data:   &common.StateSnapshot{},
					},
					Id: originalRootCollectionId,
				},
			},
		}, nil).Times(1)
		i.converters = make(map[string]common.Converter, 0)
		i.converters["Notion"] = converter

		creator := mock_objectcreator.NewMockService(t)
		creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", creatorError).Times(1)
		i.oc = creator

		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
		idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		importRequest := &ImportRequest{
			&pb.RpcObjectImportRequest{
				Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
				UpdateExistingObjects: false,
				Type:                  0,
				Mode:                  0,
				SpaceId:               "space1",
			},
			objectorigin.Import(model.Import_Notion),
			nil,
			false,
			true,
		}
		res := i.Import(context.Background(), importRequest)

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
					Snapshot: &common.SnapshotModel{
						SbType: smartblock.SmartBlockTypePage,
					},
					Id: originalRootCollectionId,
				},
			},
		}, converterError).Times(1)
		i.converters = make(map[string]common.Converter, 0)
		i.converters["Notion"] = converter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		importRequest := &ImportRequest{
			&pb.RpcObjectImportRequest{
				Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
				UpdateExistingObjects: false,
				Type:                  0,
				Mode:                  0,
				SpaceId:               "space1",
			},
			objectorigin.Import(model.Import_Notion),
			nil,
			false,
			true,
		}
		res := i.Import(context.Background(), importRequest)

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
					Snapshot: &common.SnapshotModel{
						SbType: smartblock.SmartBlockTypePage,
						Data:   &common.StateSnapshot{},
					},
					Id: originalRootCollectionId,
				},
			},
		}, converterError).Times(1)
		i.converters = make(map[string]common.Converter, 0)
		i.converters["Notion"] = converter

		creator := mock_objectcreator.NewMockService(t)
		creator.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, "", nil).Times(1)
		i.oc = creator

		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
		idGetter.EXPECT().GetIDAndPayload(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("id", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		fileSync := mock_filesync.NewMockFileSync(t)
		fileSync.EXPECT().ClearImportEvents().Return().Times(1)
		i.fileSync = fileSync

		// when
		importRequest := &ImportRequest{
			&pb.RpcObjectImportRequest{
				Params:                &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{"bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}}},
				UpdateExistingObjects: false,
				Type:                  0,
				Mode:                  pb.RpcObjectImportRequest_IGNORE_ERRORS,
				SpaceId:               "space1",
			}, objectorigin.Import(model.Import_Notion), nil, false, true,
		}
		res := i.Import(context.Background(), importRequest)

		// then
		assert.NotNil(t, res.Err)
		assert.Equal(t, expectedRootCollectionId, res.RootCollectionId)
	})
}

func Test_getObjectId(t *testing.T) {
	t.Run("get object new id", func(t *testing.T) {
		// given
		i := Import{}
		oldIDToNew := make(map[string]string, 0)
		createPayloads := make(map[string]treestorage.TreeStorageCreatePayload, 0)
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{},
			},
		}
		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
		idGetter.EXPECT().GetIDAndPayload(context.Background(), "spaceId", sn, mock.Anything, false, objectorigin.Import(model.Import_Pb)).Return("newId", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		// when
		err := i.getObjectID(context.Background(), "spaceId", sn, createPayloads, oldIDToNew, false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newId", oldIDToNew["oldId"])
	})
	t.Run("get object new id and new key", func(t *testing.T) {
		// given
		i := Import{}
		oldIDToNew := make(map[string]string, 0)
		createPayloads := make(map[string]treestorage.TreeStorageCreatePayload, 0)
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Key: "key",
				},
			},
		}
		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("newKey").Times(1)
		idGetter.EXPECT().GetIDAndPayload(context.Background(), "spaceId", sn, mock.Anything, false, objectorigin.Import(model.Import_Pb)).Return("newId", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		// when
		err := i.getObjectID(context.Background(), "spaceId", sn, createPayloads, oldIDToNew, false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newId", oldIDToNew["oldId"])
		assert.Equal(t, "newKey", oldIDToNew["key"])
	})
	t.Run("get object new id and new key", func(t *testing.T) {
		// given
		i := Import{}
		oldIDToNew := make(map[string]string, 0)
		createPayloads := make(map[string]treestorage.TreeStorageCreatePayload, 0)
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyUniqueKey: domain.String("key"),
					}),
				},
			},
		}
		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("newKey").Times(1)
		idGetter.EXPECT().GetIDAndPayload(context.Background(), "spaceId", sn, mock.Anything, false, objectorigin.Import(model.Import_Pb)).Return("newId", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		i.idProvider = idGetter

		// when
		err := i.getObjectID(context.Background(), "spaceId", sn, createPayloads, oldIDToNew, false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newId", oldIDToNew["oldId"])
		assert.Equal(t, "newKey", oldIDToNew["key"])
	})
	t.Run("don't add create payload", func(t *testing.T) {
		// given
		i := Import{}
		oldIDToNew := make(map[string]string, 0)
		createPayloads := make(map[string]treestorage.TreeStorageCreatePayload, 0)
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyUniqueKey: domain.String("key"),
					}),
				},
			},
		}
		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("newKey").Times(1)
		idGetter.EXPECT().GetIDAndPayload(context.Background(), "spaceId", sn, mock.Anything, false, objectorigin.Import(model.Import_Pb)).Return("newId", treestorage.TreeStorageCreatePayload{
			RootRawChange: &treechangeproto.RawTreeChangeWithId{},
		}, nil).Times(1)
		i.idProvider = idGetter

		// when
		err := i.getObjectID(context.Background(), "spaceId", sn, createPayloads, oldIDToNew, false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newId", oldIDToNew["oldId"])
		assert.Equal(t, "newKey", oldIDToNew["key"])
		assert.NotNil(t, createPayloads["newId"])
	})
}
