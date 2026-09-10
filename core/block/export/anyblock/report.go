package anyblock

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/export/report"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/compose"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func codecIssue(objectId string, issue anyblockjson.Issue, severity model.ExportReportIssueSeverity) model.ExportReportIssue {
	code := string(issue.Code)
	if code == "" {
		code = "codec_warning"
		if severity == model.ExportReportIssue_ERROR {
			code = "validation_failed"
		}
	}
	path := issue.Path
	if code == "unresolved_target" {
		path = referenceSourcePath(objectId, path)
	}
	return model.ExportReportIssue{ObjectId: objectId, Severity: severity, Code: code, Path: path, Message: issue.Message}
}

func warningSink(r *report.Collector, objectId string) func(anyblockjson.Issue) {
	return func(issue anyblockjson.Issue) {
		r.Add(codecIssue(objectId, issue, model.ExportReportIssue_WARNING))
	}
}

// Preserve structured fields through wrapped errors; never parse Error() text.
func recordError(r *report.Collector, objectId, code, path string, err error) {
	if issue, ok := report.KnownErrorIssue(err); ok {
		issue.Path = path
		r.Add(issue)
		return
	}
	var validation *anyblockjson.ValidationError
	if errors.As(err, &validation) && len(validation.Issues) > 0 {
		for _, issue := range validation.Issues {
			r.Add(codecIssue(objectId, issue, model.ExportReportIssue_ERROR))
		}
		return
	}
	r.Add(model.ExportReportIssue{ObjectId: objectId, Severity: model.ExportReportIssue_ERROR, Code: code, Path: path, Message: err.Error()})
}

func recordStats(r *report.Collector, stats compose.Stats) {
	for _, key := range stats.UnusedPropertyKeys {
		// Unused built-ins are expected in every space and can be supplied
		// by the application. Keep notes for omitted custom definitions.
		if bundle.HasRelation(domain.RelationKey(key)) {
			continue
		}
		r.Add(model.ExportReportIssue{Severity: model.ExportReportIssue_INFO, Code: "unused_property", Path: anyblockjson.PropertiesFileName, Message: fmt.Sprintf("Unused property %q was omitted from the dictionary", key)})
	}
	for _, key := range stats.OrphanUsedKeys {
		r.Add(model.ExportReportIssue{Severity: model.ExportReportIssue_WARNING, Code: "undefined_property", Path: anyblockjson.PropertiesFileName, Message: fmt.Sprintf("Referenced property %q has no definition", key)})
	}
	if len(stats.UnresolvedReferences) > 0 {
		for _, ref := range stats.UnresolvedReferences {
			path := referenceSourcePath(ref.ObjectID, ref.SourcePath)
			if path == "" {
				path = anyblockjson.IndexFileName + "#" + ref.Path
			}
			r.Add(model.ExportReportIssue{ObjectId: ref.ObjectID, Severity: model.ExportReportIssue_WARNING, Code: "unresolved_target",
				Path:    path,
				Message: fmt.Sprintf("Referenced object %q is unresolved", ref.TargetObjectID)})
		}
	} else {
		for _, target := range stats.UnresolvedTargets {
			r.Add(model.ExportReportIssue{Severity: model.ExportReportIssue_WARNING, Code: "unresolved_target", Path: anyblockjson.IndexFileName, Message: fmt.Sprintf("Referenced object %q is unresolved", target)})
		}
	}
	for _, reason := range stats.RefusedOptions {
		r.Add(model.ExportReportIssue{Severity: model.ExportReportIssue_ERROR, Code: "refused_options", Path: anyblockjson.PropertiesFileName, Message: reason})
	}
}

func recordCompositionIssue(r *report.Collector, objectId string, issue compose.Issue) {
	severity := model.ExportReportIssue_ERROR
	switch issue.Category {
	case compose.IssueOptionDescriptionOmitted:
		severity = model.ExportReportIssue_INFO
	case compose.IssueOptionContentOmitted:
		severity = model.ExportReportIssue_WARNING
	}
	r.Add(model.ExportReportIssue{ObjectId: objectId, Severity: severity, Code: string(issue.Category), Message: issue.Detail})
}

// referenceSourcePath makes source locations self-contained while leaving
// bundle-level references without an owning object to their index path.
func referenceSourcePath(objectId, path string) string {
	if objectId == "" || path == "" {
		return path
	}
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(objectId) + "/" + strings.TrimPrefix(path, "/")
}
