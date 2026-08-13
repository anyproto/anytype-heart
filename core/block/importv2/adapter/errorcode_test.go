package adapter

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	notionclient "github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestErrorCode pins the wire mapping the frontend depends on. This is the
// coverage whose absence let P0-2's regression ship: a cancelled run must
// reach the client as IMPORT_IS_CANCELED, never NULL or INTERNAL_ERROR.
func TestErrorCode(t *testing.T) {
	notionReq := &pb.RpcObjectImportRequest{Type: model.Import_Notion}
	mdDirReq := &pb.RpcObjectImportRequest{
		Type: model.Import_Markdown,
		Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
			MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{"/notes"}},
		},
	}
	mdZipReq := &pb.RpcObjectImportRequest{
		Type: model.Import_Markdown,
		Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
			MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{"/export.ZIP"}},
		},
	}

	for _, tc := range []struct {
		name string
		err  error
		req  *pb.RpcObjectImportRequest
		want model.ImportErrorCode
	}{
		{"nil error is NULL", nil, mdDirReq, model.Import_NULL},
		{"user cancel maps to IMPORT_IS_CANCELED",
			importv2.Fatal(importv2.IssueCancelled, context.Canceled), mdDirReq,
			model.Import_IMPORT_IS_CANCELED},
		{"rate-limit issue maps to the notion code",
			importv2.Fatal(importv2.IssueRateLimited, assert.AnError), notionReq,
			model.Import_NOTION_RATE_LIMIT_EXCEEDED},
		{"notion client rate-limit error maps directly",
			fmt.Errorf("crawl: %w", notionclient.ErrRateLimited), notionReq,
			model.Import_NOTION_RATE_LIMIT_EXCEEDED},
		{"auth failure maps to INSUFFICIENT_PERMISSIONS",
			importv2.Fatal(importv2.IssueAuthFailed, assert.AnError), notionReq,
			model.Import_INSUFFICIENT_PERMISSIONS},
		{"no objects, notion", importv2.Fatal(importv2.IssueNoObjects, assert.AnError),
			notionReq, model.Import_NOTION_NO_OBJECTS_IN_INTEGRATION},
		{"no objects, zip path (case-insensitive)",
			importv2.Fatal(importv2.IssueNoObjects, assert.AnError), mdZipReq,
			model.Import_FILE_IMPORT_NO_OBJECTS_IN_ZIP_ARCHIVE},
		{"no objects, directory", importv2.Fatal(importv2.IssueNoObjects, assert.AnError),
			mdDirReq, model.Import_FILE_IMPORT_NO_OBJECTS_IN_DIRECTORY},
		{"file fetch failure maps to FILE_LOAD_ERROR",
			importv2.Fatal(importv2.IssueFileFetchFailed, assert.AnError), mdDirReq,
			model.Import_FILE_LOAD_ERROR},
		{"a store error is INTERNAL_ERROR",
			importv2.Fatal(importv2.IssueStoreError, assert.AnError), mdDirReq,
			model.Import_INTERNAL_ERROR},
		{"an untyped error is INTERNAL_ERROR", assert.AnError, mdDirReq,
			model.Import_INTERNAL_ERROR},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, errorCode(tc.err, tc.req))
		})
	}
}
