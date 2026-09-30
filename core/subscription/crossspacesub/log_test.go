package crossspacesub

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

// The package logger is created at package init, before logging is
// configured. It must still follow the levels applied later; a logger taken
// with Logger(...).Desugar() is a detached copy that keeps the development
// default (DEBUG) forever.
func TestPackageLoggerFollowsConfiguredLevel(t *testing.T) {
	logging.SetLogLevels("ERROR")
	t.Cleanup(func() { logging.SetLogLevels("DEBUG") })

	assert.False(t, log.Core().Enabled(zap.DebugLevel), "debug must be off after setting ERROR")
	assert.True(t, log.Core().Enabled(zap.ErrorLevel), "error must stay on")
}
