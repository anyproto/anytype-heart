package ftsearch

import (
	"strings"
	"time"
	"unicode"

	"github.com/anyproto/any-sync/metric"
	"github.com/blevesearch/bleve/v2/index/scorch"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type bleveStat struct {
	Index      map[string]int `json:"index"`
	SearchTime int            `json:"search_time"`
	Searches   int            `json:"searches"`
}

func getBleveStatKeys() []string {
	var keys []string
	// init empty stats so we can extract key names
	bleveStats := scorch.Stats{}
	for k := range bleveStats.ToMap() {
		keys = append(keys, k)
	}
	keys = append(keys, "Totsearches")
	keys = append(keys, "Totsearchtime")
	return keys
}

func bleveKeyToHelp(input string) string {
	if input == "" {
		return ""
	}

	var result []rune
	for i, r := range input {
		if i > 0 && unicode.IsUpper(r) {
			result = append(result, ' ')
		}
		result = append(result, r)
	}

	// Convert to lowercase and then capitalize the first letter
	output := strings.ToLower(string(result))

	if len(output) > 3 {
		switch output[0:3] {
		case "cur":
			output = "Current" + output[3:]
		case "tot":
			output = "Total" + output[3:]
		}
	}

	return output
}

func (f *ftSearch) getBleveStatKey(key string) float64 {
	if key == "" {
		return -1
	}

	f.bleveMetricsMutex.RLock()
	defer f.bleveMetricsMutex.RUnlock()

	if key == "Totsearches" {
		if v, ok := f.bleveMetricsCache["searches"]; ok {
			return float64(v.(uint64))
		}
		return 0

	}
	if key == "Totsearchtime" {
		if v, ok := f.bleveMetricsCache["search_time"]; ok {
			return float64(v.(uint64))
		}
		return 0
	}

	if v, ok := f.bleveMetricsCache["index"].(map[string]interface{}); ok {
		if iv, ok := v[key]; ok {
			if i, ok := iv.(uint64); ok {
				return float64(i)
			}
		}
	}

	return -1
}

func (f *ftSearch) updateMetrics() {
	for {
		select {
		case <-f.closedCh:
			return
		case <-time.After(time.Second):
			f.bleveMetricsMutex.Lock()
			f.bleveMetricsCache = f.index.StatsMap()
			f.bleveMetricsMutex.Unlock()
		}
	}
}

func (f *ftSearch) initMetrics(metrics metric.Metric) {
	if metrics == nil {
		return
	}
	f.closedCh = make(chan struct{})
	go f.updateMetrics()
	metrics.Registry().MustRegister(
		promauto.NewCounterFunc(prometheus.CounterOpts{
			Namespace: "mw",
			Subsystem: "ft",
			Name:      "index_updated",
			Help:      "Fulltext updated for an object",
		}, func() float64 {

			return float64(f.ftUpdatedCounter.Load())
		}),
		promauto.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "mw",
			Subsystem: "ft",
			Name:      "docs_count",
			Help:      "Amount of documents in the bleve index",
		}, func() float64 {
			size, _ := f.DocCount()
			return float64(size)
		}),
	)

	var collectors []prometheus.Collector
	for _, k := range getBleveStatKeys() {
		var key = k
		if strings.HasPrefix(k, "Cur") {
			collectors = append(collectors, prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Subsystem: "bleve",
				Name:      strings.ToLower(k),
				Help:      bleveKeyToHelp(k),
			}, func() float64 {
				return float64(f.getBleveStatKey(key))
			}))
		}
		if strings.HasPrefix(k, "Tot") {
			var key = k
			collectors = append(collectors, prometheus.NewCounterFunc(prometheus.CounterOpts{
				Subsystem: "bleve",
				Name:      strings.ToLower(k),
				Help:      bleveKeyToHelp(k),
			}, func() float64 {
				return float64(f.getBleveStatKey(key))
			}))
		}
	}

	if len(collectors) > 0 {
		metrics.Registry().MustRegister(collectors...)
	}
}
