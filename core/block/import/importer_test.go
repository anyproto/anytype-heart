package importer

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	cv "github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	pbc "github.com/anytypeio/go-anytype-middleware/core/block/import/pb"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/web"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/web/parsers"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func Test_ImportSuccess(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)
	converter := cv.NewMockConverter(ctrl)
	converter.EXPECT().GetSnapshots(gomock.Any()).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
		Snapshot: &model.SmartBlockSnapshotBase{
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
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}, Error: nil}).Times(1)
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := NewMockCreator(ctrl)
	creator.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
	i.oc = creator

	idGetter := NewMockIDGetter(ctrl)
	idGetter.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("id", nil).Times(1)
	i.objectIDGetter = idGetter
	err := i.Import(session.NewContext(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a.pb"}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	})

	assert.Nil(t, err)
}

func Test_ImportErrorFromConverter(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)
	converter := cv.NewMockConverter(ctrl)
	e := cv.NewError()
	e.Add("error", fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(gomock.Any()).Return(&cv.Response{
		Error: e,
	}).Times(1)
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := NewMockCreator(ctrl)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	i.objectIDGetter = idGetter
	err := i.Import(session.NewContext(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: "test"}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	})

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "converter error")
}

func Test_ImportErrorFromObjectCreator(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)
	converter := cv.NewMockConverter(ctrl)
	converter.EXPECT().GetSnapshots(gomock.Any()).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
		Snapshot: &model.SmartBlockSnapshotBase{
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
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}, Error: nil}).Times(1)
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := NewMockCreator(ctrl)
	//nolint:lll
	creator.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("creator error")).Times(1)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	idGetter.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("id", nil).Times(1)
	i.objectIDGetter = idGetter
	res := i.Import(session.NewContext(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: "test"}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	})

	assert.NotNil(t, res)
	assert.Contains(t, res.Error(), "creator error")
}

func Test_ImportIgnoreErrorMode(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)
	converter := cv.NewMockConverter(ctrl)
	e := cv.NewError()
	e.Add("error", fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(gomock.Any()).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
		Snapshot: &model.SmartBlockSnapshotBase{
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
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}, Error: e}).Times(1)
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := NewMockCreator(ctrl)
	creator.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	idGetter.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("id", nil).Times(1)
	i.objectIDGetter = idGetter
	res := i.Import(session.NewContext(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: "test"}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
	})

	assert.NotNil(t, res)
	assert.Contains(t, res.Error(), "converter error")
}

func Test_ImportIgnoreErrorModeWithTwoErrorsPerFile(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)
	converter := cv.NewMockConverter(ctrl)
	e := cv.NewError()
	e.Add("error", fmt.Errorf("converter error"))
	converter.EXPECT().GetSnapshots(gomock.Any()).Return(&cv.Response{Snapshots: []*cv.Snapshot{{
		Snapshot: &model.SmartBlockSnapshotBase{
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
		Id: "bafybbbbruo3kqubijrbhr24zonagbz3ksxbrutwjjoczf37axdsusu4a"}}, Error: e}).Times(1)
	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = converter
	creator := NewMockCreator(ctrl)
	//nolint:lll
	creator.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("creator error")).Times(1)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	idGetter.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("id", nil).Times(1)
	i.objectIDGetter = idGetter
	res := i.Import(session.NewContext(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: "test"}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
	})

	assert.NotNil(t, res)
	assert.Contains(t, res.Error(), "converter error")
	assert.Contains(t, res.Error(), "converter error", "creator error")
}

func Test_ImportExternalPlugin(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)

	i.converters = make(map[string]cv.Converter, 0)

	creator := NewMockCreator(ctrl)
	creator.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	idGetter.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("id", nil).Times(1)
	i.objectIDGetter = idGetter
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
	res := i.Import(session.NewContext(), &pb.RpcObjectImportRequest{
		Params:                nil,
		Snapshots:             snapshots,
		UpdateExistingObjects: false,
		Type:                  1,
		Mode:                  2,
	})
	assert.Nil(t, res)
}

func Test_ImportExternalPluginError(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)

	i.converters = make(map[string]cv.Converter, 0)

	creator := NewMockCreator(ctrl)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	i.objectIDGetter = idGetter
	res := i.Import(session.NewContext(), &pb.RpcObjectImportRequest{
		Params:                nil,
		Snapshots:             nil,
		UpdateExistingObjects: false,
		Type:                  1,
		Mode:                  2,
	})
	assert.NotNil(t, res)
	assert.Contains(t, res.Error(), "snapshots are empty")
}

func Test_ListImports(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)

	i.converters = make(map[string]cv.Converter, 0)
	i.converters["Notion"] = pbc.New(nil)
	creator := NewMockCreator(ctrl)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	i.objectIDGetter = idGetter
	res, err := i.ListImports(session.NewContext(), &pb.RpcObjectImportListRequest{})

	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Len(t, res, 1)
	assert.True(t, res[0].Type == pb.RpcObjectImportListImportResponseType(0) || res[1].Type == pb.RpcObjectImportListImportResponseType(0))
}

func Test_ImportWebNoParser(t *testing.T) {
	i := Import{}

	ctrl := gomock.NewController(t)

	i.converters = make(map[string]cv.Converter, 0)
	i.converters[web.Name] = web.NewConverter(nil)

	creator := NewMockCreator(ctrl)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	i.objectIDGetter = idGetter
	_, _, err := i.ImportWeb(session.NewContext(), &pb.RpcObjectImportRequest{
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
	i.converters[web.Name] = web.NewConverter(nil)
	creator := NewMockCreator(ctrl)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	i.objectIDGetter = idGetter
	parser := parsers.NewMockParser(ctrl)
	parser.EXPECT().MatchUrl("http://example.com").Return(true).Times(1)
	parser.EXPECT().ParseUrl("http://example.com").Return(nil, errors.New("failed")).Times(1)

	new := func() parsers.Parser {
		return parser
	}
	parsers.RegisterFunc(new)

	_, _, err := i.ImportWeb(session.NewContext(), &pb.RpcObjectImportRequest{
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

	i.converters[web.Name] = web.NewConverter(nil)

	creator := NewMockCreator(ctrl)
	creator.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	idGetter.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("id", nil).Times(1)
	i.objectIDGetter = idGetter
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

	_, _, err := i.ImportWeb(session.NewContext(), &pb.RpcObjectImportRequest{
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
	i.converters[web.Name] = web.NewConverter(nil)

	creator := NewMockCreator(ctrl)
	//nolint:lll
	creator.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("error")).Times(1)
	i.oc = creator
	idGetter := NewMockIDGetter(ctrl)
	idGetter.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("id", nil).Times(1)
	i.objectIDGetter = idGetter
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

	_, _, err := i.ImportWeb(session.NewContext(), &pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfBookmarksParams{BookmarksParams: &pb.RpcObjectImportRequestBookmarksParams{Url: "http://example.com"}},
		UpdateExistingObjects: true,
	})

	assert.NotNil(t, err)
	assert.Equal(t, "couldn't create objects", err.Error())
}
