package objectcache

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricCacheSizes      map[string]func() int
	metricCacheSizesMutex sync.Mutex
)

func metricSetSpace(spaceId string, f func() int) {
	metricCacheSizesMutex.Lock()
	defer metricCacheSizesMutex.Unlock()
	if metricCacheSizes == nil {
		metricCacheSizes = make(map[string]func() int)
	}
	metricCacheSizes[spaceId] = f
}

func metricUnsetSpace(spaceId string) {
	metricCacheSizesMutex.Lock()
	defer metricCacheSizesMutex.Unlock()
	if metricCacheSizes == nil {
		return
	}
	delete(metricCacheSizes, spaceId)
}

func MetricAllCachesSize() float64 {
	metricCacheSizesMutex.Lock()
	defer metricCacheSizesMutex.Unlock()
	if metricCacheSizes == nil {
		return 0
	}
	var total float64
	for _, v := range metricCacheSizes {
		total += float64(v())
	}
	return total
}

func MetricReset() {
	metricCacheSizesMutex.Lock()
	defer metricCacheSizesMutex.Unlock()
	metricCacheSizes = make(map[string]func() int)
}

func init() {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "anytype",
		Subsystem: "object",
		Name:      "cache_size",
		Help:      "Object cache len",
	}, MetricAllCachesSize)
}
