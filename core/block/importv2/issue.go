package importv2

import (
	"errors"
	"fmt"
)

// Severity orders issues: info and warnings never abort, object errors abort
// in all-or-nothing mode, fatal issues always abort.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityObjectError
	SeverityFatal
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityObjectError:
		return "objectError"
	case SeverityFatal:
		return "fatal"
	default:
		return fmt.Sprintf("severity(%d)", int(s))
	}
}

// IssueCode is the typed classification of an issue. Codes are stable
// identifiers: they appear in golden tests and map onto the wire
// model.Import.ErrorCode in the adapter.
type IssueCode string

const (
	// Informational — run diagnostics, never a problem.
	IssueFlavourDetected IssueCode = "flavourDetected"
	IssueTypeSuggested   IssueCode = "typeSuggested"
	// IssuePropertyMapped records one adopted schema-plan property decision:
	// a remap onto a bundled relation, a cross-container merge, or a format
	// fix.
	IssuePropertyMapped IssueCode = "propertyMapped"

	// Warnings — deliberate data decisions and placeholders.
	IssueUnsupportedBlock IssueCode = "unsupportedBlock"
	IssueDataLoss         IssueCode = "dataLoss"
	IssueMissingTarget    IssueCode = "missingTarget"
	// IssueLLMPlanFailed means the optional LLM structure analysis was
	// requested but unavailable (endpoint, auth, budget, invalid response);
	// the run degraded to built-in rules. Never fatal.
	IssueLLMPlanFailed IssueCode = "llmPlanFailed"
	// IssueLLMPlanEntryDropped means one plan entry failed validation and was
	// ignored while the rest of the plan applied.
	IssueLLMPlanEntryDropped IssueCode = "llmPlanEntryDropped"

	// Object errors.
	IssueObjectFailed    IssueCode = "objectFailed"
	IssueFileFetchFailed IssueCode = "fileFetchFailed"
	// IssueObjectTooLarge marks an object whose snapshot exceeds the sync
	// ceiling for a single CRDT change — persisted objects must
	// stay under it or they could never replicate.
	IssueObjectTooLarge IssueCode = "objectTooLarge"

	// Fatal.
	IssueSourceInvalid IssueCode = "sourceInvalid"
	IssueNoObjects     IssueCode = "noObjects"
	IssueAuthFailed    IssueCode = "authFailed"
	IssueRateLimited   IssueCode = "rateLimited"
	IssueCancelled     IssueCode = "cancelled"
	IssueStoreError    IssueCode = "storeError"
	// IssueInvariant flags an engine/converter contract violation (e.g. a
	// file reference before its definition). Always a bug, never data.
	IssueInvariant IssueCode = "invariant"
)

// Issue is the single structured error/warning record used by every stage.
//
// Message is the CONSTANT half — the same sentence for every issue of the same
// kind, so the report can group by it and say "435 of these" instead of
// printing 435 lines. Whatever varies (a property name, a block kind, a child
// title) belongs in Subject, and how many times it happened in one place
// belongs in Count. A message that interpolates a value splits its own group.
type Issue struct {
	Severity  Severity
	Code      IssueCode
	SourceKey string // which source object, when known
	ObjectId  string // which created object, when known
	// Subject names the thing inside the source object the issue is about:
	// the property, the block kind, the child page. Empty when the issue is
	// about the object as a whole.
	Subject string
	// Count is how many times this happened for this source object; 0 and 1
	// both mean once. Converters that would otherwise emit one issue per
	// block report the tally instead, which keeps the ledger (capped at
	// IssueCap) describing the whole import rather than its first thousand
	// blocks.
	Count   int
	Message string
	Err     error
}

// Times returns the issue with an occurrence count.
func (i Issue) Times(count int) Issue {
	i.Count = count
	return i
}

// About returns the issue with the subject it concerns.
func (i Issue) About(subject string) Issue {
	i.Subject = subject
	return i
}

// Occurrences normalizes Count for arithmetic: an unset count is one.
func (i Issue) Occurrences() int {
	if i.Count < 1 {
		return 1
	}
	return i.Count
}

// Error implements error so an Issue can travel through error returns.
func (i Issue) Error() string {
	msg := i.Message
	if msg == "" && i.Err != nil {
		msg = i.Err.Error()
	}
	if i.Subject != "" {
		msg = fmt.Sprintf("%s: %s", i.Subject, msg)
	}
	if i.Count > 1 {
		msg = fmt.Sprintf("%s (x%d)", msg, i.Count)
	}
	if i.SourceKey != "" {
		return fmt.Sprintf("%s [%s] %s: %s", i.Severity, i.Code, i.SourceKey, msg)
	}
	return fmt.Sprintf("%s [%s]: %s", i.Severity, i.Code, msg)
}

func (i Issue) Unwrap() error {
	return i.Err
}

// Info builds an informational issue.
func Info(code IssueCode, message string) Issue {
	return Issue{Severity: SeverityInfo, Code: code, Message: message}
}

// Warning builds a warning-severity issue.
func Warning(code IssueCode, sourceKey, message string) Issue {
	return Issue{Severity: SeverityWarning, Code: code, SourceKey: sourceKey, Message: message}
}

// ObjectError builds an object-error-severity issue.
func ObjectError(code IssueCode, sourceKey string, err error) Issue {
	return Issue{Severity: SeverityObjectError, Code: code, SourceKey: sourceKey, Err: err}
}

// Fatal builds a fatal-severity issue.
func Fatal(code IssueCode, err error) Issue {
	return Issue{Severity: SeverityFatal, Code: code, Err: err}
}

// AsIssue extracts an Issue from an error chain, or wraps the error as a
// fallback issue with the given defaults.
func AsIssue(err error, defaultSeverity Severity, defaultCode IssueCode) Issue {
	var issue Issue
	if errors.As(err, &issue) {
		return issue
	}
	return Issue{Severity: defaultSeverity, Code: defaultCode, Err: err}
}

// ErrSuspended is the cancellation cause of a graceful shutdown: the run
// stops promptly but is NOT compensated — its durable state is kept for the
// startup sweep. Distinct from user cancellation (a plain cancel, which
// compensates): the adapter cancels the run context with this cause from
// Close, and the engine checks context.Cause against it.
var ErrSuspended = errors.New("import run suspended for shutdown")

// Mode is the run-wide failure policy.
type Mode int

const (
	// ModeAllOrNothing aborts (and compensates) on the first object error.
	ModeAllOrNothing Mode = iota
	// ModeContinueOnError skips failed objects and keeps going.
	ModeContinueOnError
)

// ShouldAbort is the single mode predicate applied uniformly by every stage.
func ShouldAbort(severity Severity, mode Mode) bool {
	if severity >= SeverityFatal {
		return true
	}
	return severity >= SeverityObjectError && mode == ModeAllOrNothing
}
