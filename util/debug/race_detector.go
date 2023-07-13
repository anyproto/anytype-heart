package debug

import (
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

func TryCollectRaces() {
	if isRaceDetectorOn() {
		go collectRace()
	}
}

func isRaceDetectorOn() bool {
	b, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}

	for _, s := range b.Settings {
		if s.Key == "-race" && s.Value == "true" {
			return true
		}
	}
	return false
}

func collectRace() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	raceLogFileName := parseLogFileName() + "." + strconv.Itoa(os.Getpid())

	for range ticker.C {
		logRaces(raceLogFileName)
	}
}

func logRaces(raceLogFileName string) {
	contentBytes, err := os.ReadFile(raceLogFileName)
	content := string(contentBytes)

	if err != nil {
		log.Debugf("Failed to read file: %s", err)
		return
	}
	if content == "" {
		return
	}

	log.Error("races collected", content)

	truncateFile(raceLogFileName)
}

// nolint:errcheck
func truncateFile(filePath string) {
	// Truncate the file by opening it in write-only mode and setting the file size to 0
	file, err := os.OpenFile(filePath, os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	file.Truncate(0)
}

func parseLogFileName() string {
	envVarName := "GORACE"
	envVarValue := os.Getenv(envVarName)

	if envVarValue == "" {
		return ""
	}

	split := strings.Split(envVarValue, "=")
	if len(split) != 2 {
		return ""
	}

	if split[0] != "log_path" {
		return ""
	}

	return split[1]
}
