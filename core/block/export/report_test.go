package export

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestAnyBlockWarningsReachReportAndNotification(t *testing.T) {
	fx := newFixture(t)
	anyblockSpace(t, fx)
	// This scenario must contain warnings only: give the shared fixture's
	// property snapshot the identity and format its store record already has.
	property, err := fx.picker.GetObject(context.Background(), propertyKey.String())
	require.NoError(t, err)
	propertyObject := property.(*smarttest.SmartTest)
	propertyState := propertyObject.NewState()
	propertyState.SetUniqueKeyInternal(propertyKey.String())
	propertyState.SetDetail(bundle.RelationKeyRelationKey, domain.String(propertyKey))
	propertyState.SetDetail(bundle.RelationKeyRelationFormat, domain.Int64(int64(model.RelationFormat_tag)))
	propertyObject.Doc = propertyState
	object, err := fx.picker.GetObject(context.Background(), pageId)
	require.NoError(t, err)
	page := object.(*smarttest.SmartTest)
	page.AddBlock(simple.New(&model.Block{Id: pageId, ChildrenIds: []string{"div-warning"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
	page.AddBlock(simple.New(&model.Block{Id: "div-warning", BackgroundColor: "red", ChildrenIds: []string{"text"}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Div}}}))
	page.AddBlock(simple.New(&model.Block{Id: "text", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "hello"}}}))
	fx.picker.EXPECT().TryRemoveFromCache(mock.Anything, mock.Anything).Return(true, nil)
	fx.sender.EXPECT().Broadcast(mock.Anything).Return()
	notifications := make(chan *model.Notification, 1)
	fx.notifications.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(n *model.Notification) error {
		notifications <- n
		return nil
	})

	_, singleReport, err := fx.ExportSingleInMemory(context.Background(), spaceId, pageId, model.Export_AnyBlockV2)
	require.NoError(t, err)
	assertContainerWarning(t, singleReport)
	exportDir := t.TempDir()
	_, bundleReport, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{SpaceId: spaceId, Path: exportDir, Format: model.Export_AnyBlockV2, IncludeArchived: true})
	require.NoError(t, err)
	assertContainerWarning(t, bundleReport)
	select {
	case n := <-notifications:
		require.NotNil(t, n.GetExport())
		assert.Equal(t, model.NotificationExport_NULL, n.GetExport().ErrorCode)
		assert.Equal(t, bundleReport, n.GetExport().Report)
		assert.Equal(t, exportDir, n.GetExport().Path)
	case <-time.After(5 * time.Second):
		t.Fatal("export completion notification was not sent")
	}
}

func assertContainerWarning(t *testing.T, r *model.ExportReport) {
	t.Helper()
	require.NotNil(t, r)
	assert.Equal(t, model.ExportReport_SUCCESS, r.Status)
	for _, issue := range r.Issues {
		if issue.ObjectId == pageId && issue.Code == "codec_warning" && issue.Path == "/blocks" {
			assert.Equal(t, model.ExportReportIssue_WARNING, issue.Severity)
			assert.Contains(t, issue.Message, "div-warning")
			return
		}
	}
	t.Fatalf("container warning missing from report: %v", r)
}
