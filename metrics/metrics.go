package metrics

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-telemetry")

var (
	DefaultInHouseKey string
)

func GenerateAnalyticsId() string {
	return uuid.New().String()
}

var (
	Enabled bool
	once    sync.Once

	ObjectFTUpdatedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "fulltext_index_updated",
		Help:      "Fulltext updated for an object",
	})
	ObjectDetailsUpdatedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "details_index_updated",
		Help:      "Details updated for an object",
	})
	ObjectDetailsHeadsNotChangedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "details_index_heads_not_changed",
		Help:      "Details head not changed optimization",
	})
	LinkPreviewStatusCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "link_preview_non_ok_status_total",
		Help:      "Total count of HTTP status codes from LinkPreview URL fetches",
	}, []string{"status_code", "status_class"})
)

func registerPrometheusExpvars() {
	expvarCollector := prometheus.NewExpvarCollector(map[string]*prometheus.Desc{
		"badger_blocked_puts_total": prometheus.NewDesc(
			"badger_blocked_puts_total",
			"badger_blocked_puts_total",
			nil, nil,
		),
		"badger_lsm_size_bytes": prometheus.NewDesc(
			"badger_lsm_size_bytes",
			"badger_lsm_size_bytes",
			[]string{"dir"}, nil,
		),
		"badger_vlog_size_bytes": prometheus.NewDesc(
			"badger_vlog_size_bytes",
			"badger_vlog_size_bytes",
			[]string{"dir"}, nil,
		),
		"badger_pending_writes_total": prometheus.NewDesc(
			"badger_pending_writes_total",
			"badger_pending_writes_total",
			[]string{"dir"}, nil,
		),
		"badger_disk_reads_total": prometheus.NewDesc(
			"badger_disk_reads_total",
			"badger_disk_reads_total",
			nil, nil,
		),
		"badger_disk_writes_total": prometheus.NewDesc(
			"badger_disk_writes_total",
			"badger_disk_writes_total",
			nil, nil,
		),
		"badger_read_bytes": prometheus.NewDesc(
			"badger_read_bytes",
			"badger_read_bytes",
			nil, nil,
		),
		"badger_written_bytes": prometheus.NewDesc(
			"badger_written_bytes",
			"badger_written_bytes",
			nil, nil,
		),
		"badger_lsm_level_gets_total": prometheus.NewDesc(
			"badger_lsm_level_gets_total",
			"badger_lsm_level_gets_total",
			[]string{"level"}, nil,
		),
		"badger_lsm_bloom_hits_total": prometheus.NewDesc(
			"badger_lsm_bloom_hits_total",
			"badger_lsm_bloom_hits_total",
			[]string{"level"}, nil,
		),
		"badger_gets_total": prometheus.NewDesc(
			"badger_gets_total",
			"badger_gets_total",
			nil, nil,
		),
		"badger_puts_total": prometheus.NewDesc(
			"badger_puts_total",
			"badger_puts_total",
			nil, nil,
		),
		"badger_memtable_gets_total": prometheus.NewDesc(
			"badger_memtable_gets_total",
			"badger_memtable_gets_total",
			nil, nil,
		),
	})

	prometheus.MustRegister(expvarCollector)
}

func runPrometheusHttp(addr string) {
	once.Do(func() {
		registerPrometheusExpvars()
		// Create a HTTP server for prometheus.
		httpServer := &http.Server{Handler: promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}), Addr: addr}
		Enabled = true

		// Start your http server for prometheus.
		go func() {
			if err := httpServer.ListenAndServe(); err != nil {
				Enabled = false
				log.Errorf("Unable to start a prometheus http server.")
			}
		}()
	})
}

func MetricTimeBuckets(scale []time.Duration) []float64 {
	buckets := make([]float64, len(scale))
	for i, b := range scale {
		buckets[i] = b.Seconds()
	}
	return buckets
}

func init() {
	if DefaultInHouseKey == "" {
		DefaultInHouseKey = loadenv.Get("INHOUSE_KEY")
	}

	if addr := os.Getenv("ANYTYPE_PROM"); addr != "" {
		runPrometheusHttp(addr)
	}
}
