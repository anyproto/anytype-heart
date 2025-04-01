package export

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/api/apicore/mock_apicore"
)

const (
	spaceID            = "space-123"
	objectID           = "obj-456"
	exportFormat       = "markdown"
	unrecognizedFormat = "unrecognized"
	exportPath         = "/some/dir/myexport"
)

type fixture struct {
	*ExportService
	exportMock *mock_apicore.MockExportService
	mwMock     *mock_apicore.MockClientCommands
}

func newFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	exportMock := mock_apicore.NewMockExportService(t)
	exportService := NewService(mwMock, exportMock)

	return &fixture{
		ExportService: exportService,
		exportMock:    exportMock,
		mwMock:        mwMock,
	}
}

func TestExportService_GetObjectExport(t *testing.T) {
	// TODO: revive tests once export is finalized
	// t.Run("successful export to markdown", func(t *testing.T) {
	// 	// Given
	// 	ctx := context.Background()
	// 	fx := newFixture(t)
	//
	// 	// Mock the ObjectListExport call
	// 	fx.mwMock.
	// 		On("ObjectListExport", mock.Anything, &pb.RpcObjectListExportRequest{
	// 			SpaceId:         spaceID,
	// 			Path:            exportPath,
	// 			ObjectIds:       []string{objectID},
	// 			Format:          model.Export_Markdown,
	// 			Zip:             false,
	// 			IncludeNested:   false,
	// 			IncludeFiles:    true,
	// 			IsJson:          false,
	// 			IncludeArchived: false,
	// 			NoProgress:      true,
	// 		}).
	// 		Return(&pb.RpcObjectListExportResponse{
	// 			Path: exportPath,
	// 			Error: &pb.RpcObjectListExportResponseError{
	// 				Code: pb.RpcObjectListExportResponseError_NULL,
	// 			},
	// 		}).
	// 		Once()
	//
	// 	// When
	// 	gotPath, err := fx.GetObjectExport(ctx, spaceID, objectID, exportFormat)
	//
	// 	// Then
	// 	require.NoError(t, err)
	// 	require.Equal(t, exportPath, gotPath)
	// 	fx.mwMock.AssertExpectations(t)
	// })
	//
	// t.Run("failed export returns error", func(t *testing.T) {
	// 	// Given
	// 	ctx := context.Background()
	// 	fx := newFixture(t)
	//
	// 	// Mock the ObjectListExport call to return an error code
	// 	fx.mwMock.
	// 		On("ObjectListExport", mock.Anything, mock.Anything).
	// 		Return(&pb.RpcObjectListExportResponse{
	// 			Path: "",
	// 			Error: &pb.RpcObjectListExportResponseError{
	// 				Code: pb.RpcObjectListExportResponseError_UNKNOWN_ERROR,
	// 			},
	// 		}).
	// 		Once()
	//
	// 	// When
	// 	gotPath, err := fx.GetObjectExport(ctx, spaceID, objectID, exportFormat)
	//
	// 	// Then
	// 	require.Error(t, err)
	// 	require.Empty(t, gotPath)
	// 	require.ErrorIs(t, err, ErrFailedExportObjectAsMarkdown)
	// 	fx.mwMock.AssertExpectations(t)
	// })
	//
	// t.Run("unrecognized format defaults to markdown", func(t *testing.T) {
	// 	ctx := context.Background()
	// 	fx := newFixture(t)
	//
	// 	fx.mwMock.
	// 		On("ObjectListExport", mock.Anything, &pb.RpcObjectListExportRequest{
	// 			SpaceId:         spaceID,
	// 			Path:            exportPath,
	// 			ObjectIds:       []string{objectID},
	// 			Format:          model.Export_Markdown, // fallback
	// 			Zip:             false,
	// 			IncludeNested:   false,
	// 			IncludeFiles:    true,
	// 			IsJson:          false,
	// 			IncludeArchived: false,
	// 			NoProgress:      true,
	// 		}).
	// 		Return(&pb.RpcObjectListExportResponse{
	// 			Path: exportPath,
	// 			Error: &pb.RpcObjectListExportResponseError{
	// 				Code: pb.RpcObjectListExportResponseError_NULL,
	// 			},
	// 		}).
	// 		Once()
	//
	// 	// When
	// 	gotPath, err := fx.GetObjectExport(ctx, spaceID, objectID, unrecognizedFormat)
	//
	// 	// Then
	// 	require.NoError(t, err)
	// 	require.Equal(t, exportPath, gotPath)
	// 	fx.mwMock.AssertExpectations(t)
	// })
}
