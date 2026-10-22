package anyblock

import (
	"fmt"
	"strings"
	"testing"

	codec "github.com/anyproto/any-block/codec/anyblockjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/export/report"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/compose"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestWrappedValidationIssuesPreserveFields(t *testing.T) {
	var c report.Collector
	err := fmt.Errorf("marshal: %w", &anyblockjson.ValidationError{Issues: []anyblockjson.Issue{
		{Path: "/blocks/0", Code: "invalid_block", Message: "invalid block"},
		{Path: "/properties/name", Message: "invalid name"},
	}})
	recordError(&c, "object", "object_export_failed", "objects/object.anyblock.json", err)
	r := c.Snapshot(nil)
	require.Len(t, r.Issues, 2)
	assert.Equal(t, &model.ExportReportIssue{ObjectId: "object", Severity: model.ExportReportIssue_ERROR, Code: "invalid_block", Path: "/blocks/0", Message: "invalid block"}, r.Issues[0])
	assert.Equal(t, "validation_failed", r.Issues[1].Code)
	assert.Equal(t, "/properties/name", r.Issues[1].Path)
}

func TestWarningsAndBundleDiagnostics(t *testing.T) {
	var c report.Collector
	warningSink(&c, "object")(anyblockjson.Issue{Path: "/blocks", Code: "indent_clamped", Message: "indent clamped"})
	recordStats(&c, compose.Stats{
		UnusedPropertyKeys: []string{"unused"},
		OrphanUsedKeys:     []string{"undefined"},
		UnresolvedTargets:  []string{"missing"},
		RefusedOptions:     []string{"property: unsupported vocabulary"},
	})
	r := c.Snapshot(nil)
	assert.Equal(t, model.ExportReport_PARTIAL, r.Status)
	require.Len(t, r.Issues, 5)
	byCode := map[string]*model.ExportReportIssue{}
	for _, issue := range r.Issues {
		byCode[issue.Code] = issue
	}
	assert.Equal(t, &model.ExportReportIssue{ObjectId: "object", Severity: model.ExportReportIssue_WARNING, Code: "indent_clamped", Path: "/blocks", Message: "indent clamped"}, byCode["indent_clamped"])
	assert.Equal(t, model.ExportReportIssue_ERROR, byCode["refused_options"].Severity)
	assert.Equal(t, "index.json", byCode["unresolved_target"].Path)
}

func TestTypeIdentityFailurePreservesAffectedType(t *testing.T) {
	var c report.Collector
	err := fmt.Errorf("render document: %w", &anyblockjson.TypeIdentityMismatchError{ObjectID: "typeid-test", InternalKey: "test", DocumentID: "type-test", ReferenceID: "typeid-test"})
	recordError(&c, "owning-object", "object_export_failed", "types/test.anyblock.json", err)
	r := c.Snapshot(fmt.Errorf("export: %w", err))
	require.Len(t, r.Issues, 1)
	assert.Equal(t, "type_identity_mismatch", r.Issues[0].Code)
	assert.Equal(t, "typeid-test", r.Issues[0].ObjectId)
	assert.Equal(t, "types/test.anyblock.json", r.Issues[0].Path)
	assert.Equal(t, "export: "+err.Error(), r.Issues[0].Message)
}

func TestNonBlockingExportNotes(t *testing.T) {
	var c report.Collector
	recordStats(&c, compose.Stats{UnusedPropertyKeys: []string{"custom-budget", "custom-phase"}})
	recordCompositionIssue(&c, "option-test", compose.Issue{Category: compose.IssueOptionDescriptionOmitted, Detail: "description omitted"})
	r := c.Snapshot(nil)
	require.Equal(t, model.ExportReport_SUCCESS, r.Status)
	require.Len(t, r.Issues, 3)
	for _, issue := range r.Issues {
		assert.Equal(t, model.ExportReportIssue_INFO, issue.Severity)
	}
	assert.Zero(t, r.ObjectErrors)
	recordCompositionIssue(&c, "option-missing", compose.Issue{Category: "omitted_reconstruction", Detail: "missing option name"})
	assert.Equal(t, model.ExportReport_PARTIAL, c.Snapshot(nil).Status)
}

func TestOptionContentOmissionIsWarningOnlyForOptions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		issue    compose.Issue
		severity model.ExportReportIssueSeverity
	}{
		{"option page content", compose.Issue{Category: compose.IssueOptionContentOmitted, Detail: "option page blocks omitted"}, model.ExportReportIssue_WARNING},
		{"property page content", compose.Issue{Category: "omitted_reconstruction", Detail: "property page blocks omitted"}, model.ExportReportIssue_ERROR},
		{"missing option name", compose.Issue{Category: "omitted_reconstruction", Detail: "option has no name"}, model.ExportReportIssue_ERROR},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c report.Collector
			recordCompositionIssue(&c, "synthetic-object", tc.issue)
			r := c.Snapshot(nil)
			require.Len(t, r.Issues, 1)
			assert.Equal(t, tc.severity, r.Issues[0].Severity)
			assert.Equal(t, string(tc.issue.Category), r.Issues[0].Code)
			assert.Equal(t, "synthetic-object", r.Issues[0].ObjectId)
			assert.Equal(t, tc.issue.Detail, r.Issues[0].Message)
			if tc.severity == model.ExportReportIssue_ERROR {
				assert.Equal(t, model.ExportReport_PARTIAL, r.Status)
			} else {
				assert.Equal(t, model.ExportReport_SUCCESS, r.Status)
			}
			assert.Zero(t, r.ObjectErrors)
		})
	}
}

func TestReferenceWarningPreservesSourceAndTarget(t *testing.T) {
	var c report.Collector
	warningSink(&c, "source-object")(anyblockjson.Issue{Code: "unresolved_target", Path: "/blocks/source-block/object_id", Message: "missing target"})
	r := c.Snapshot(nil)
	require.Len(t, r.Issues, 1)
	assert.Equal(t, &model.ExportReportIssue{ObjectId: "source-object", Severity: model.ExportReportIssue_WARNING, Code: "unresolved_target", Path: "source-object/blocks/source-block/object_id", Message: "missing target"}, r.Issues[0])
	assert.Equal(t, model.ExportReport_SUCCESS, r.Status)
}

func TestIndexReferenceWarningsPreserveFullSourcePaths(t *testing.T) {
	var c report.Collector
	recordStats(&c, compose.Stats{
		UnresolvedTargets: []string{"missing"},
		UnresolvedReferences: []codec.ObjectReference{
			{TargetObjectID: "missing", ObjectID: "space", SourcePath: "/properties/homepage", Path: "/homepage"},
			{TargetObjectID: "missing", ObjectID: "widgets", SourcePath: "/blocks/widget-link/object_id", Path: "/widgets/0/target"},
			{TargetObjectID: "missing", Path: "/entrypoint"},
		},
	})
	r := c.Snapshot(nil)
	require.Len(t, r.Issues, 3)
	assert.Equal(t, model.ExportReport_SUCCESS, r.Status)
	assert.Equal(t, "index.json#/entrypoint", r.Issues[0].Path)
	assert.Equal(t, "space", r.Issues[1].ObjectId)
	assert.Equal(t, "space/properties/homepage", r.Issues[1].Path)
	assert.Equal(t, "widgets", r.Issues[2].ObjectId)
	assert.Equal(t, "widgets/blocks/widget-link/object_id", r.Issues[2].Path)
	for _, issue := range r.Issues {
		assert.Equal(t, "unresolved_target", issue.Code)
		assert.Contains(t, issue.Message, "missing")
	}
}

func TestReferenceSourcePath(t *testing.T) {
	assert.Equal(t, "source/properties/related/1", referenceSourcePath("source", "/properties/related/1"))
	assert.Equal(t, "source~1id~0/blocks/link/object_id", referenceSourcePath("source/id~", "/blocks/link/object_id"))
	assert.Equal(t, "/properties/homepage", referenceSourcePath("", "/properties/homepage"))
	assert.Empty(t, referenceSourcePath("source", ""))
}

func TestUnusedBuiltInPropertiesDoNotCreateExportNotes(t *testing.T) {
	keys := []string{"custom-sync-status"}
	for _, url := range bundle.ListRelationsUrls() {
		key, err := bundle.RelationKeyFromID(url)
		require.NoError(t, err)
		keys = append(keys, key.String())
	}
	require.Greater(t, len(keys), 1)
	var c report.Collector
	recordStats(&c, compose.Stats{UnusedPropertyKeys: keys, UnresolvedTargets: []string{"missing-icon"}})
	r := c.Snapshot(nil)
	require.Equal(t, model.ExportReport_SUCCESS, r.Status)
	require.Len(t, r.Issues, 2)
	assert.Equal(t, "unresolved_target", r.Issues[0].Code)
	assert.Equal(t, model.ExportReportIssue_WARNING, r.Issues[0].Severity)
	assert.Equal(t, "unused_property", r.Issues[1].Code)
	assert.Equal(t, model.ExportReportIssue_INFO, r.Issues[1].Severity)
	assert.Equal(t, `Unused property "custom-sync-status" was omitted from the dictionary`, r.Issues[1].Message)
}

// The index's declared dangling targets reach the report graded by class
// (SPEC §2c): a tombstone is by-design state and lands as info, an object
// the space holds that the export did not write is a warning, and an id the
// space had no row for — absent, most likely unsynced — keeps the
// unresolved_target warning. Only the last is a loss worth a user's eye.
func TestUnresolvedTargetsAreGradedByClass(t *testing.T) {
	var c report.Collector
	recordStats(&c, compose.Stats{
		UnresolvedTargets: []string{"absent", "gone", "shelved"},
		UnresolvedDeleted: []string{"gone"},
		UnresolvedOmitted: []string{"shelved"},
		UnresolvedReferences: []codec.ObjectReference{
			{TargetObjectID: "gone", Path: "/homepage", ObjectID: "space", SourcePath: "/details/homepage"},
			{TargetObjectID: "shelved", Path: "/widgets/0/target", ObjectID: "widget", SourcePath: "/blocks/w1/link"},
			{TargetObjectID: "absent", Path: "/widgets/1/target", ObjectID: "widget", SourcePath: "/blocks/w2/link"},
		},
	})
	r := c.Snapshot(nil)
	byTarget := map[string]*model.ExportReportIssue{}
	for _, issue := range r.Issues {
		for _, target := range []string{"absent", "gone", "shelved"} {
			if strings.Contains(issue.Message, `"`+target+`"`) {
				byTarget[target] = issue
			}
		}
	}
	require.Len(t, byTarget, 3)
	assert.Equal(t, "deleted_target", byTarget["gone"].Code)
	assert.Equal(t, model.ExportReportIssue_INFO, byTarget["gone"].Severity)
	assert.Equal(t, "omitted_target", byTarget["shelved"].Code)
	assert.Equal(t, model.ExportReportIssue_WARNING, byTarget["shelved"].Severity)
	assert.Equal(t, "unresolved_target", byTarget["absent"].Code)
	assert.Equal(t, model.ExportReportIssue_WARNING, byTarget["absent"].Severity)
	assert.Equal(t, "widget/blocks/w2/link", byTarget["absent"].Path, "source-path attribution is unchanged")
}

// A document naming a type no document carries and the source space never
// held (SPEC §2c, unresolved.types) reaches the report as a warning per
// reference, attributed to the document and the slot, so the user learns
// which objects will restore as Pages.
func TestUnresolvedTypeReferencesAreReported(t *testing.T) {
	var c report.Collector
	recordStats(&c, compose.Stats{
		UnresolvedTypes: []string{"type-69aab06861fab2bc0d9afc59"},
		UnresolvedTypeReferences: []codec.ObjectReference{
			{TargetObjectID: "type-69aab06861fab2bc0d9afc59", Path: "/type_internal_key", ObjectID: "orphan"},
		},
	})
	r := c.Snapshot(nil)
	require.Len(t, r.Issues, 1)
	assert.Equal(t, "unresolved_type", r.Issues[0].Code)
	assert.Equal(t, model.ExportReportIssue_WARNING, r.Issues[0].Severity)
	assert.Equal(t, "orphan", r.Issues[0].ObjectId)
	assert.Equal(t, "orphan/type_internal_key", r.Issues[0].Path)
	assert.Contains(t, r.Issues[0].Message, "69aab06861fab2bc0d9afc59")
	assert.Contains(t, r.Issues[0].Message, "Page")
}
