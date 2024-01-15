package metrics

import (
	"os"
	"sync"

	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-logger")

var DefaultAmplitudeKey = ""

const (
	EnvVarPromAddr = "ANYTYPE_PROM"
)

func GenerateAnalyticsId() string {
	return uuid.New().String()
}

var (
	Enabled bool
	once    sync.Once
)

func init() {
	if DefaultAmplitudeKey == "" {
		DefaultAmplitudeKey = loadenv.Get("AMPLITUDE_KEY")
	}
	if os.Getenv(EnvVarPromAddr) != "" {
		Enabled = true
	}

}
