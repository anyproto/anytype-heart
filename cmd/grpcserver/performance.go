package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/util/vcs"
)

var (
	PromUser     string
	PromPassword string
)

func startReportMemory(mw *core.Middleware) {
	if env := os.Getenv("ANYTYPE_REPORT_MEMORY"); env != "" {
		go func() {
			var maxAlloc uint64
			var meanCPU float64
			var maxHeapObjects uint64
			var maxRSS uint64
			var maxNative uint64
			var memStats runtime.MemStats
			var curProc *process.Process
			times := 60 * 3
			rev := getRev()
			pid := os.Getpid()

			curProc, err := process.NewProcess(int32(pid))
			if err != nil {
				fmt.Printf("Can't get current process: %s\n", err)
				return
			}

			_ = writeCpuProfile(rev, func() {
				for i := 0; i < times; i++ {

					memInfo, err := curProc.MemoryInfo()
					if err != nil {
						fmt.Printf("Can't get rss: %s\n", err)
					}

					runtime.ReadMemStats(&memStats)
					percent, err := cpu.Percent(time.Second, false)
					if err != nil {
						fmt.Printf("Can't get cpu percent: %s\n", err)
					}

					if maxRSS < memInfo.RSS {
						maxRSS = memInfo.RSS
					}
					if maxAlloc < memStats.Alloc {
						maxAlloc = memStats.Alloc
					}
					if maxHeapObjects < memStats.HeapObjects {
						maxHeapObjects = memStats.HeapObjects
					}
					if maxNative < memInfo.RSS-memStats.Alloc && memInfo.RSS > memStats.Alloc {
						maxNative = memInfo.RSS - memStats.Alloc
					}
					meanCPU += percent[0]
					time.Sleep(time.Second)
				}

				accountSelectTimeBytes, err := os.ReadFile("root/ACCOUNT_SELECT_TIME")
				if err != nil {
					fmt.Println("Error reading account select time:", err)
					return
				}

				accountSelectTime, err := strconv.Atoi(string(accountSelectTimeBytes))
				fmt.Println("###Account select time:", accountSelectTime)
				if err != nil {
					fmt.Println("Error converting account select time:", err)
					return
				}

				err = sendMetrics(
					lo.Assign(
						getFileSizes(),
						getTableSizes(mw),
						map[string]uint64{
							"MaxAlloc":      bytesToMegabytes(maxAlloc),
							"TotalAlloc":    bytesToMegabytes(memStats.TotalAlloc),
							"MaxNative":     bytesToMegabytes(maxNative),
							"MaxRSS":        bytesToMegabytes(maxRSS),
							"Mallocs":       memStats.Mallocs,
							"Frees":         memStats.Frees,
							"MeanCpu":       uint64(meanCPU / float64(times)),
							"HeapObjects":   maxHeapObjects,
							"AccountSelect": uint64(accountSelectTime),
						}),
				)
				if err != nil {
					os.Exit(-1)
				}
			})
			_ = writeHeapProfile(rev)
			os.Exit(0)
		}()
	}
}

func bytesToMegabytes(maxAlloc uint64) uint64 {
	return maxAlloc / 1024 / 1024
}

func getRev() string {
	return vcs.GetVCSInfo().Revision[:8]
}

func writeCpuProfile(rev string, toProfile func()) error {
	file, err := os.Create(fmt.Sprintf("cpu_profile_%s_%d.pprof", rev, time.Now().UnixMilli()))
	if err != nil {
		fmt.Println("Error creating profile file:", err)
		return err
	}
	defer file.Close()

	if err := pprof.StartCPUProfile(file); err != nil {
		fmt.Println("Error writing cpu profile:", err)
		return err
	}

	toProfile()

	pprof.StopCPUProfile()
	return nil
}

func writeHeapProfile(rev string) error {
	file, err := os.Create(fmt.Sprintf("heap_profile_%s_%d.pprof", rev, time.Now().UnixMilli()))
	if err != nil {
		fmt.Println("Error creating profile file:", err)
		return err
	}
	defer file.Close()

	if err := pprof.WriteHeapProfile(file); err != nil {
		fmt.Println("Error writing heap profile:", err)
		return err
	}
	return nil
}

func sendMetrics(metrics map[string]uint64) error {
	url := "https://pushgateway.anytype.io/metrics/job/heart_tech"

	var sb strings.Builder
	for key, value := range metrics {
		data := fmt.Sprintf("r_%s_%s %d\n", key, getRev(), value)
		fmt.Println(data)
		sb.WriteString(data)
	}

	client := &http.Client{
		Timeout: 1 * time.Minute,
	}
	err := makeMetricsRequest(client, url, sb.String())
	if err != nil {
		return err
	}

	time.Sleep(1 * time.Minute)
	err = makeDeleteRequest(client, url)
	if err != nil {
		return err
	}

	fmt.Println("metric has been sent")
	return nil
}

func makeDeleteRequest(client *http.Client, url string) error {
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		fmt.Println("metric delete err:", err)
		return err
	}

	req.SetBasicAuth(PromUser, PromPassword)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("metric delete err:", err)
		return err
	}
	defer resp.Body.Close()
	return nil
}

func makeMetricsRequest(client *http.Client, url string, content string) error {
	req, err := http.NewRequest("POST", url, strings.NewReader(content))
	if err != nil {
		fmt.Println("metric send err:", err)
		return err
	}

	req.SetBasicAuth(PromUser, PromPassword)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("metric send err:", err)
		return err
	}
	defer resp.Body.Close()
	return nil
}

func getSize(path string) (uint64, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	if !fileInfo.IsDir() {
		return uint64(fileInfo.Size()), nil
	}

	var size int64
	err = filepath.Walk(path, func(subpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return uint64(size), nil
}

func getFileSizes() map[string]uint64 {
	dir, err := os.Open("root")
	if err != nil {
		fmt.Println("Error opening directory:", err)
		return nil
	}
	defer dir.Close()

	fileInfos, err := dir.Readdir(-1)
	if err != nil {
		fmt.Println("Error reading directory contents:", err)
		return nil
	}

	resultsSizes := make(map[string]uint64)
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			subdirPath := path.Join("root", fileInfo.Name())
			subdir, err := os.Open(subdirPath)
			if err != nil {
				fmt.Println("Error opening subdirectory:", err)
				continue
			}
			defer subdir.Close()

			subdirFileInfos, err := subdir.Readdir(-1)
			if err != nil {
				fmt.Println("Error reading subdirectory contents:", err)
				continue
			}

			subdirFileNames := make([]string, len(subdirFileInfos))
			for _, subdirFileInfo := range subdirFileInfos {
				subdirFileNames = append(subdirFileNames, subdirFileInfo.Name())
			}

			for _, filename := range []string{
				"fts",
				"fts_tantivy",
				"localstore",
				"spacestore",
				"spaceStore.db",
			} {
				if lo.Contains(subdirFileNames, filename) {
					size, _ := getSize(path.Join(subdirPath, filename))
					resultsSizes[fmt.Sprintf("f_%s", strings.Replace(filename, ".", "_", -1))] = bytesToMegabytes(size)
				}
			}
		}
	}
	fmt.Println(resultsSizes)
	return resultsSizes
}

func getTableSizes(mw *core.Middleware) (tables map[string]uint64) {
	tables = make(map[string]uint64)
	cfg := mw.GetApp().MustComponent(config.CName).(*config.Config)

	db, err := sql.Open("sqlite3", cfg.GetSpaceStorePath())
	if err != nil {
		fmt.Println("Error opening database:", err)
		return
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		fmt.Println("Error pinging database:", err)
		return
	}

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table';")
	if err != nil {
		fmt.Println("Error querying database:", err)
	}
	defer rows.Close()

	fmt.Println("Tables:")
	for rows.Next() {
		var tableName string
		err := rows.Scan(&tableName)
		if err != nil {
			fmt.Println("Error scanning row:", err)
		}

		var count int
		err = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s;", tableName)).Scan(&count)
		if err != nil {
			fmt.Println("Error querying row:", err)
		}

		tables[fmt.Sprintf("t_%s", tableName)] = uint64(count)

		fmt.Printf("%s: %d\n", tableName, count)
	}
	if err := rows.Err(); err != nil {
		fmt.Println("Error iterating over rows:", err)
	}
	return
}

func init() {
	if PromUser == "" {
		PromUser = loadenv.Get("PROM_KEY")
	}

	if PromPassword == "" {
		PromPassword = loadenv.Get("PROM_PASSWORD")
	}
}
