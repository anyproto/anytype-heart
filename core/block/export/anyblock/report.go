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

// unresolvedGrader maps a dangling index target to its report grade from
// the composer's classification (compose.Stats.UnresolvedDeleted / Omitted).
func unresolvedGrader(stats compose.Stats) func(target string) (model.ExportReportIssueSeverity, string, string) {
	deleted := map[string]struct{}{}
	for _, id := range stats.UnresolvedDeleted {
		deleted[id] = struct{}{}
	}
	omitted := map[string]struct{}{}
	for _, id := range stats.UnresolvedOmitted {
		omitted[id] = struct{}{}
	}
	return func(target string) (model.ExportReportIssueSeverity, string, string) {
		if _, ok := deleted[target]; ok {
			return model.ExportReportIssue_INFO, string(anyblockjson.IssueCodeDeletedTarget),
				fmt.Sprintf("Referenced object %q was deleted from the space; the reference is kept by design", target)
		}
		if _, ok := omitted[target]; ok {
			return model.ExportReportIssue_WARNING, string(anyblockjson.IssueCodeOmittedTarget),
				fmt.Sprintf("Referenced object %q exists in the space but was not included in this export", target)
		}
		return model.ExportReportIssue_WARNING, string(anyblockjson.IssueCodeUnresolvedTarget),
			fmt.Sprintf("Referenced object %q is unresolved: the space has no row for it, so it was not synced or never existed here", target)
	}
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
	// Types the documents name that no document carries and the source
	// space never held (SPEC §2c, unresolved.types): one warning per
	// reference, attributed to the document and the slot, because each such
	// object restores as a Page.
	if len(stats.UnresolvedTypeReferences) > 0 {
		for _, ref := range stats.UnresolvedTypeReferences {
			path := referenceSourcePath(ref.ObjectID, ref.Path)
			if path == "" {
				path = anyblockjson.IndexFileName
			}
			r.Add(model.ExportReportIssue{ObjectId: ref.ObjectID, Severity: model.ExportReportIssue_WARNING, Code: string(anyblockjson.IssueCodeUnresolvedType),
				Path:    path,
				Message: fmt.Sprintf("Referenced type %q has no declaration in the bundle and the source space never held it; the object is imported as a Page", ref.TargetObjectID)})
		}
	} else {
		for _, target := range stats.UnresolvedTypes {
			r.Add(model.ExportReportIssue{Severity: model.ExportReportIssue_WARNING, Code: string(anyblockjson.IssueCodeUnresolvedType), Path: anyblockjson.IndexFileName,
				Message: fmt.Sprintf("Referenced type %q has no declaration in the bundle and the source space never held it; its objects are imported as Pages", target)})
		}
	}
	// The index's dangling targets, graded by the class the composer gave
	// them (SPEC §2c): a tombstone is by-design state and is info, an
	// object the space holds that this export did not write is a warning,
	// and an id the space had no row for — absent, most likely unsynced —
	// keeps the unresolved_target warning. Only the last is a loss.
	grade := unresolvedGrader(stats)
	if len(stats.UnresolvedReferences) > 0 {
		for _, ref := range stats.UnresolvedReferences {
			path := referenceSourcePath(ref.ObjectID, ref.SourcePath)
			if path == "" {
				path = anyblockjson.IndexFileName + "#" + ref.Path
			}
			severity, code, message := grade(ref.TargetObjectID)
			r.Add(model.ExportReportIssue{ObjectId: ref.ObjectID, Severity: severity, Code: code, Path: path, Message: message})
		}
	} else {
		for _, target := range stats.UnresolvedTargets {
			severity, code, message := grade(target)
			r.Add(model.ExportReportIssue{Severity: severity, Code: code, Path: anyblockjson.IndexFileName, Message: message})
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
