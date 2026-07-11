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

	// Warnings — deliberate data decisions and placeholders.
	IssueUnsupportedBlock IssueCode = "unsupportedBlock"
	IssueDataLoss         IssueCode = "dataLoss"
	IssueMissingTarget    IssueCode = "missingTarget"

	// Object errors.
	IssueObjectFailed    IssueCode = "objectFailed"
	IssueFileFetchFailed IssueCode = "fileFetchFailed"
	// IssueObjectTooLarge marks an object whose snapshot exceeds the sync
	// ceiling for a single CRDT change (§16 item 8) — persisted objects must
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
type Issue struct {
	Severity  Severity
	Code      IssueCode
	SourceKey string // which source object, when known
	ObjectId  string // which created object, when known
	Message   string
	Err       error
}

// Error implements error so an Issue can travel through error returns.
func (i Issue) Error() string {
	msg := i.Message
	if msg == "" && i.Err != nil {
		msg = i.Err.Error()
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
