package report_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/export/report"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestConcurrentReportSnapshots(t *testing.T) {
	var c report.Collector
	assert.Equal(t, model.ExportReport_SUCCESS, c.Snapshot(nil).Status)
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Succeeded()
			c.ObjectFailed()
			c.FileFailed()
			c.Add(model.ExportReportIssue{ObjectId: fmt.Sprintf("object-%02d", i), Code: "test", Message: "warning"})
			_ = c.Snapshot(nil)
		}()
	}
	wg.Wait()
	r := c.Snapshot(nil)
	assert.Equal(t, model.ExportReport_PARTIAL, r.Status)
	assert.EqualValues(t, 64, r.Succeed)
	assert.EqualValues(t, 64, r.ObjectErrors)
	assert.EqualValues(t, 64, r.FileErrors)
	require.Len(t, r.Issues, 64)
	for i, issue := range r.Issues {
		assert.Equal(t, fmt.Sprintf("object-%02d", i), issue.ObjectId)
	}
	r.Issues[0].Message = "mutated by caller"
	assert.Equal(t, "warning", c.Snapshot(nil).Issues[0].Message)
}

func TestFatalAndCanceledReports(t *testing.T) {
	var inner, outer report.Collector
	err := fmt.Errorf("disk full")
	inner.Succeeded()
	outer.Merge(inner.Snapshot(err))
	r := outer.Snapshot(fmt.Errorf("write bundle: %w", err))
	assert.Equal(t, model.ExportReport_FAILED, r.Status)
	require.Len(t, r.Issues, 1)
	assert.Equal(t, "write bundle: disk full", r.Issues[0].Message)
	assert.EqualValues(t, 1, r.Succeed)

	canceled := inner.Snapshot(fmt.Errorf("emit: %w", context.Canceled))
	assert.Equal(t, model.ExportReport_CANCELED, canceled.Status)
	assert.Empty(t, canceled.Issues)
}

func TestReportSurvivesRPCAndNotificationWireFormat(t *testing.T) {
	var c report.Collector
	c.Succeeded()
	c.FileFailed()
	c.Add(model.ExportReportIssue{ObjectId: "file-id", Severity: model.ExportReportIssue_ERROR, Code: "file_export_failed", Path: "files/file.bin", Message: "file unavailable"})
	r := c.Snapshot(nil)
	for _, message := range []proto.Message{
		&pb.RpcObjectListExportResponse{Report: r},
		&pb.RpcObjectExportResponse{Report: r},
		&model.Notification{Payload: &model.NotificationPayloadOfExport{Export: &model.NotificationExport{Report: r}}},
	} {
		data, err := proto.Marshal(message)
		require.NoError(t, err)
		restored := proto.Clone(message)
		restored.Reset()
		require.NoError(t, proto.Unmarshal(data, restored))
		assert.True(t, proto.Equal(message, restored), "%T", message)
	}
}

func TestWrappedTypeIdentityFailure(t *testing.T) {
	var inner, outer report.Collector
	cause := &anyblockjson.TypeIdentityMismatchError{ObjectID: "typeid-test", InternalKey: "test", DocumentID: "type-test", ReferenceID: "typeid-test"}
	innerErr := fmt.Errorf("build path plan: %w", cause)
	outer.Merge(inner.Snapshot(innerErr))
	err := fmt.Errorf("export by format: %w", innerErr)
	r := outer.Snapshot(err)
	require.Equal(t, model.ExportReport_FAILED, r.Status)
	require.Len(t, r.Issues, 1)
	assert.Equal(t, "type_identity_mismatch", r.Issues[0].Code)
	assert.Equal(t, "typeid-test", r.Issues[0].ObjectId)
	assert.Equal(t, model.ExportReportIssue_ERROR, r.Issues[0].Severity)
	assert.Equal(t, err.Error(), r.Issues[0].Message)
	for _, message := range []proto.Message{
		&pb.RpcObjectListExportResponse{Report: r}, &pb.RpcObjectExportResponse{Report: r},
		&model.Notification{Payload: &model.NotificationPayloadOfExport{Export: &model.NotificationExport{Report: r}}},
	} {
		data, marshalErr := proto.Marshal(message)
		require.NoError(t, marshalErr)
		restored := proto.Clone(message)
		restored.Reset()
		require.NoError(t, proto.Unmarshal(data, restored))
		assert.True(t, proto.Equal(message, restored))
	}
}

func TestOnlyErrorsMakeCompletedExportPartial(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		severity                 model.ExportReportIssueSeverity
		objectErrors, fileErrors bool
		want                     model.ExportReportStatus
	}{
		{"warning", model.ExportReportIssue_WARNING, false, false, model.ExportReport_SUCCESS},
		{"information", model.ExportReportIssue_INFO, false, false, model.ExportReport_SUCCESS},
		{"error issue without counter", model.ExportReportIssue_ERROR, false, false, model.ExportReport_PARTIAL},
		{"object error", model.ExportReportIssue_WARNING, true, false, model.ExportReport_PARTIAL},
		{"file error", model.ExportReportIssue_WARNING, false, true, model.ExportReport_PARTIAL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var inner, outer report.Collector
			inner.Add(model.ExportReportIssue{Severity: tc.severity, Code: "test", Message: "preserved diagnostic"})
			if tc.objectErrors {
				inner.ObjectFailed()
			}
			if tc.fileErrors {
				inner.FileFailed()
			}
			outer.Merge(inner.Snapshot(nil))
			r := outer.Snapshot(nil)
			assert.Equal(t, tc.want, r.Status)
			require.Len(t, r.Issues, 1)
			assert.Equal(t, tc.severity, r.Issues[0].Severity)
		})
	}
}
