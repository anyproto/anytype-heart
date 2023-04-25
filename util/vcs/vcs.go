package vcs

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"time"
)

var (
	GitBranch, GitSummary string
)

type VCSInfo struct {
	BuildDate time.Time
	Revision  string
	Branch    string
	Summary   string
	Modified  bool
	CGO       bool
}

func (v VCSInfo) Version() string {
	if v.Summary != "" {
		return v.Summary
	}
	if v.Revision == "" {
		return "unknown"
	}
	rev := v.Revision
	if len(rev) == 40 {
		// trim sha1 to 8 chars
		rev = rev[0:8]
	}
	if v.Modified {
		rev = rev + "-dirty"
	}
	return rev
}

func (v VCSInfo) Description() string {
	var desc string
	if v.Branch != "" {
		desc = fmt.Sprintf("build on %s from %s at #%s", v.BuildDate, v.Branch, v.Revision)
	} else {
		desc = fmt.Sprintf("build on %s at #%s", v.BuildDate, v.Revision)
	}
	if v.Modified {
		desc += " (dirty)"
	}
	if !v.CGO {
		desc += " (no-cgo)"
	}
	return fmt.Sprintf("build on %s from %s at #%s(%s)", v.BuildDate, v.Branch, v.Revision, v.Summary)
}

// GetVCSInfo returns git build info
// branch and summary are set by linker flags via govvv
func GetVCSInfo() VCSInfo {
	info := VCSInfo{
		Branch:  GitBranch,
		Summary: GitSummary,
		CGO:     true,
	}
	d, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	for _, setting := range d.Settings {
		switch setting.Key {
		case "vcs.time":
			info.BuildDate, _ = time.Parse(time.RFC3339, setting.Value)
		case "vcs.modified":
			info.Modified, _ = strconv.ParseBool(setting.Value)
		case "vcs.revision":
			info.Revision = setting.Value
		case "CGO_ENABLED":
			info.CGO, _ = strconv.ParseBool(setting.Value)
		}
	}

	return info
}
