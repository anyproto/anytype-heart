package metrics

import (
	"net"
	"os"
	"sync"
	"time"

	"github.com/anyproto/prommy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"modernc.org/libc"

	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-telemetry")

var (
	DefaultInHouseKey string
	serverAddr        string
)

func GenerateAnalyticsId() string {
	return uuid.New().String()
}

var (
	GrpcEnabled bool
	once        sync.Once

	ObjectFTDocUpdatedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "object",
		Name:      "fulltext_docs_updated",
		Help:      "Fulltext docs(blocks and properties) updated for an object. Update skipped if doc is not changed",
	})

	ObjectStoreUpdatedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "object",
		Name:      "store_updated",
		Help:      "Store updated for an object",
	})
	ObjectChangeCreatedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "object",
		Name:      "change_created",
		Help:      "Store updated for an object",
	})
	ObjectChangeStateAppendedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "object",
		Name:      "change_state_appended",
		Help:      "State appended for an object",
	})
	ObjectChangeStateRebuildCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "object",
		Name:      "change_state_rebuild",
		Help:      "State rebuild for an object",
	})

	ObjectCacheHitCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "object",
		Name:      "cache_hit",
		Help:      "Cache hit count",
	})
	ObjectCacheMissCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "object",
		Name:      "cache_miss",
		Help:      "Cache miss count",
	})
	ObjectCacheGCCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "anytype",
		Subsystem: "object",
		Name:      "cache_gc",
		Help:      "Cache garbage collected count",
	})
	// anytype_object_cache_size is registered in objectCache pkg due to circular import

	allocatorMMapCounter = prometheus.NewDesc(
		"libc_allocator_mmap",
		"Number of current mmap allocations",
		nil, nil)
	allocatorBytesCounter = prometheus.NewDesc(
		"libc_allocator_bytes",
		"Number of current bytes allocated",
		nil, nil)
	allocatorAllocsCounter = prometheus.NewDesc(
		"libc_allocator_allocs",
		"Number of current allocations",
		nil, nil)
)

func addrToHttp(addr string) string {
	addr, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	if addr == "" {
		addr = "localhost"
	}
	return "http://" + addr + ":" + port
}

// must be called only once
func registerCustomMetrics() {
	// throttle libc.MemStat() call to avoid locks
	var lastMemstat libc.MemAllocatorStat
	var lastMemstatTime time.Time
	var lastMemstatMutex sync.Mutex
	prometheus.MustRegister(prometheus.CollectorFunc(func(ch chan<- prometheus.Metric) {
		lastMemstatMutex.Lock()
		defer lastMemstatMutex.Unlock()
		if time.Since(lastMemstatTime) > time.Second*5 {
			lastMemstat = libc.MemStat()
			lastMemstatTime = time.Now()
		}
		ch <- prometheus.MustNewConstMetric(allocatorMMapCounter, prometheus.GaugeValue, float64(lastMemstat.Mmaps))
		ch <- prometheus.MustNewConstMetric(allocatorBytesCounter, prometheus.GaugeValue, float64(lastMemstat.Bytes))
		ch <- prometheus.MustNewConstMetric(allocatorAllocsCounter, prometheus.GaugeValue, float64(lastMemstat.Allocs))
	}))
}

func Start(addr string) string {
	once.Do(func() {
		if os.Getenv("ANYTYPE_PROM_GRPC") == "1" {
			GrpcEnabled = true
		}
		serverAddr = addr
		registerCustomMetrics()
		go func() {
			err := prommy.Serve(addr, prommy.WithDashboardJSON(`[
		[
            {"name": "anytype_object_change_created", "short": "CHANGES"},
			{"name": "anytype_object_change_state_appended", "short": "APPEND"},
			{"name": "anytype_object_change_state_rebuild", "short": "REBUILD"}
        ],        
		[
			{"name": "anytype_object_store_updated", "short": "STORE"},
			{"name": "anytype_object_fulltext_docs_updated", "short": "FT DOCS"},
			{"name": "anytype_object_cache_size", "short": "OBJ CACHE"}
		],
		[
            {"name": "go_memstats_heap_alloc_bytes", "short": "HEAP"}, 
            {"name": "go_goroutines", "short": "GOROUTINES"},
			{"name": "go_memstats_alloc_bytes", "short": "ALLOC"}
        ],
		[ 
            {"name": "go_memstats_sys_bytes", "short": "SYS"},
			{"name": "libc_allocator_bytes", "short": "LIBC"}
		]
    ]`))
			if err != nil {
				log.Errorf("failed to start metrics server: %v", err)
			}
		}()
	})

	return addrToHttp(serverAddr)
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
		_ = Start(addr)
	}
}
