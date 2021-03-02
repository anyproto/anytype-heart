package metrics

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"os"
	"sync"
	"time"
)

var log = logging.Logger("anytype-logger")

var (
	Enabled       bool
	once          sync.Once
	ServedThreads = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "threads_total",
		Help:      "Number of served threads",
	})

	ChangeCreatedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "change_created",
		Help:      "Number of changes created",
	})

	ChangeReceivedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "change_received",
		Help:      "Number of changes received",
	})

	ExternalThreadReceivedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "external_thread_received",
		Help:      "New thread received",
	})

	ExternalThreadHandlingAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "external_thread_handling_attempts",
		Help:      "New thread handling attempts",
	})

	ExternalThreadHandlingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "external_thread_handling_seconds",
		Help:      "New thread successfully handling duration",
		Buckets: MetricTimeBuckets([]time.Duration{
			256 * time.Millisecond,
			512 * time.Millisecond,
			1024 * time.Millisecond,
			2 * time.Second,
			4 * time.Second,
			8 * time.Second,
			16 * time.Second,
			30 * time.Second,
			45 * time.Second,
			60 * time.Second,
			90 * time.Second,
			120 * time.Second,
			180 * time.Second,
			240 * time.Second,
		}),
	})

	ThreadAdded = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "thread_added",
		Help:      "New thread added",
	})

	ThreadAddReplicatorAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "thread_add_replicator_attempts",
		Help:      "New thread handling attempts",
	})

	ThreadAddReplicatorDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "anytype",
		Subsystem: "mw",
		Name:      "thread_add_replicator_seconds",
		Help:      "New thread successfully handling duration",
		Buckets: MetricTimeBuckets([]time.Duration{
			256 * time.Millisecond,
			512 * time.Millisecond,
			1024 * time.Millisecond,
			2 * time.Second,
			4 * time.Second,
			8 * time.Second,
			16 * time.Second,
			30 * time.Second,
			45 * time.Second,
			60 * time.Second,
			90 * time.Second,
			120 * time.Second,
			180 * time.Second,
			240 * time.Second,
		}),
	})
)

func runPrometheusHttp(addr string) {
	once.Do(func() {
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
	if addr := os.Getenv("ANYTYPE_PROM"); addr != "" {
		runPrometheusHttp(addr)
	}
}
