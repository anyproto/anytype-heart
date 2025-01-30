package internal

import (
	"fmt"
	"os"
	"time"
)

// logToFile writes logs to a specified file
func logToFile(file *os.File, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, _ = file.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, message))
}

// logInfo logs informational messages
func logInfo(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	fmt.Println(message)
	appendToLogFile(message)
}

// logError logs errors
func logError(format string, args ...interface{}) {
	message := fmt.Sprintf("❌ "+format, args...)
	fmt.Println(message)
	appendToLogFile(message)
}

// appendToLogFile appends logs to the log file
func appendToLogFile(message string) {
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		defer logFile.Close()
		logToFile(logFile, message)
	}
}
