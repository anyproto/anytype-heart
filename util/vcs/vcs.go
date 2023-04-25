package vcs

import (
	"runtime/debug"
	"strconv"
	"time"
)

func GetVCSInfo() (buildDate time.Time, revision string, modified bool, cgo bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	cgo = true // missing means CGO is enabled
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.time":
			buildDate, _ = time.Parse(time.RFC3339, setting.Value)
		case "vcs.timestamp":
			ts, err := strconv.Atoi(setting.Value)
			if err == nil {
				buildDate = time.Unix(int64(ts), 0)
			}
		case "vcs.modified":
			modified, _ = strconv.ParseBool(setting.Value)
		case "vcs.revision":
			revision = setting.Value
		case "CGO_ENABLED":
			cgo, _ = strconv.ParseBool(setting.Value)
		}
	}
	return
}
