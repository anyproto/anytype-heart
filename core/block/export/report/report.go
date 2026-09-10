// Package report collects diagnostics independently of the export format.
package report

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Collector is safe for concurrent export workers. Its zero value is ready to use.
type Collector struct {
	mu   sync.Mutex
	data model.ExportReport
}

func (c *Collector) Add(issue model.ExportReportIssue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.Issues = append(c.data.Issues, &issue)
}

func (c *Collector) ObjectFailed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.ObjectErrors++
}

func (c *Collector) FileFailed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.FileErrors++
}

func (c *Collector) Succeeded() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.Succeed++
}

func (c *Collector) SetSucceed(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.Succeed = int32(n)
}

func (c *Collector) Merge(r *model.ExportReport) {
	if r == nil {
		return
	}
	r = proto.Clone(r).(*model.ExportReport)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.Succeed += r.Succeed
	c.data.ObjectErrors += r.ObjectErrors
	c.data.FileErrors += r.FileErrors
	c.data.Issues = append(c.data.Issues, r.Issues...)
}

// Snapshot returns an independent, deterministically ordered report. Fatal
// errors remain RPC errors too; recoverable errors make a completed export partial. Warnings do not affect status.
func (c *Collector) Snapshot(err error) *model.ExportReport {
	c.mu.Lock()
	r := proto.Clone(&c.data).(*model.ExportReport)
	c.mu.Unlock()
	switch {
	case errors.Is(err, context.Canceled):
		r.Status = model.ExportReport_CANCELED
	case err != nil:
		r.Status = model.ExportReport_FAILED
		// A nested exporter may already have reported the same fatal error.
		// Keep one entry, with the outer context added by the caller.
		diagnostic, known := KnownErrorIssue(err)
		if !known {
			diagnostic = model.ExportReportIssue{Severity: model.ExportReportIssue_ERROR, Code: "export_failed", Message: err.Error()}
		}
		var fatal *model.ExportReportIssue
		for _, issue := range r.Issues {
			if issue.Code == diagnostic.Code && issue.ObjectId == diagnostic.ObjectId && issue.Severity == diagnostic.Severity {
				fatal = issue
				break
			}
		}
		if fatal == nil {
			fatal = &diagnostic
			r.Issues = append(r.Issues, fatal)
		}
		fatal.Message = err.Error()
	case r.ObjectErrors > 0 || r.FileErrors > 0 || hasErrors(r.Issues):
		r.Status = model.ExportReport_PARTIAL
	default:
		r.Status = model.ExportReport_SUCCESS
	}
	sort.Slice(r.Issues, func(i, j int) bool {
		a, b := r.Issues[i], r.Issues[j]
		if a.ObjectId != b.ObjectId {
			return a.ObjectId < b.ObjectId
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		return a.Message < b.Message
	})
	return r
}

// KnownErrorIssue preserves structured export failures through wrapping. Message
// retains diagnostic context; clients translate Code for user-facing text.
func KnownErrorIssue(err error) (model.ExportReportIssue, bool) {
	var mismatch *anyblockjson.TypeIdentityMismatchError
	if errors.As(err, &mismatch) {
		return model.ExportReportIssue{ObjectId: mismatch.ObjectID, Severity: model.ExportReportIssue_ERROR,
			Code: string(anyblockjson.IssueCodeTypeIdentityMismatch), Message: err.Error()}, true
	}
	return model.ExportReportIssue{}, false
}

func hasErrors(issues []*model.ExportReportIssue) bool {
	for _, issue := range issues {
		if issue.Severity == model.ExportReportIssue_ERROR {
			return true
		}
	}
	return false
}
