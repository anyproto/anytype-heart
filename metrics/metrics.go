package metrics

import (
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-telemetry")

var (
	DefaultInHouseKey string
)

const (
	EnvVarPromAddr = "ANYTYPE_PROM"
)

func GenerateAnalyticsId() string {
	return uuid.New().String()
}

var (
	Enabled bool
)

func init() {
	if DefaultInHouseKey == "" {
		DefaultInHouseKey = loadenv.Get("INHOUSE_KEY")
	}
}
