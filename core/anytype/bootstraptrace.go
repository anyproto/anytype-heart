//go:build appdebug
// +build appdebug

package anytype

import (
	"fmt"
	"hash/crc32"
	"os"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/app"
)

/*
	How to use:
	Add the code and tun the app, login to the account
	collector := newComponentCollector()
	a.SetOnComponentListener(collector.collectInference)
	...
	if err = a.Start(ctx); err != nil {
		metrics.Service.Close()
		a = nil
		return
	}
	printGraph(a)

	Then type in shell:
	dot -Tsvg graph.dot -o graph.svg && open graph.svg
*/

type componentCollector struct {
	// key - component (package.structName), value - list of injected components
	depsGraph map[string][]string
	mu        sync.Mutex
}

func newComponentCollector() *componentCollector {
	return &componentCollector{
		depsGraph: make(map[string][]string),
	}
}

func (cc *componentCollector) printGraph() {
	graph := map[string][]string{}
	nogo := []string{
		"anytype-heart/core/block.Service",
		"anytype-heart/core/anytype/config.Config",
		"anytype-heart/core/wallet.wallet",
		"anytype-heart/core/block.Service",
		"anytype-heart/pkg/lib/localstore/objectstore.dsObjectStore",
		"anytype-heart/core/event.GrpcSender",
	}
	for key, values := range cc.depsGraph {
		for _, value := range values {
			contains := false
			for _, ng := range nogo {
				if strings.Contains(key, ng) || strings.Contains(value, ng) {
					contains = true
					break
				}
			}
			if !contains {
				graph[key] = append(graph[key], value)
			}
		}
	}

	cycles, found := findCycles(graph)
	cycles = removeDuplicates(cycles)

	if found {
		fmt.Println("# cycles")
		_ = prettyPrintMatrixToFile(cycles, "cycles.txt")
		_ = prettyPrintMatrixDotToFile(cycles, "cycles.dot")
	}

	topologicalSortAndPrint(graph)
}

func (cc *componentCollector) collectInference(found app.Component) {
	pc, _, _, ok := runtime.Caller(2)
	details := runtime.FuncForPC(pc)
	if ok && details != nil {
		fullTypeName := reflect.TypeOf(found).PkgPath() + "." + reflect.TypeOf(found).Name()
		if len(fullTypeName) == 1 {
			fullTypeName = reflect.TypeOf(found).Elem().PkgPath() + "." + reflect.TypeOf(found).Elem().Name()
		}
		calledFrom := runtime.FuncForPC(details.Entry()).Name()
		calledForType := fullTypeName

		calledFrom = strings.TrimPrefix(calledFrom, "github.com/anyproto/")
		calledForType = strings.TrimPrefix(calledForType, "github.com/anyproto/")

		if strings.Contains(calledFrom, ".(*") {
			calledFrom = strings.Split(calledFrom, ".(*")[0] + "." + strings.Split(strings.Split(calledFrom, ".(*")[1], ").")[0]
		}
		calledForType = strings.Split(calledForType, ".(*")[0]
		cc.mu.Lock()
		cc.depsGraph[calledFrom] = append(cc.depsGraph[calledFrom], calledForType)
		cc.mu.Unlock()
	}
}

func dfs(
	graph map[string][]string,
	node string,
	visited map[string]struct{},
	path []string,
	cycles map[int][]string,
) {
	visited[node] = struct{}{}
	path = append(path, node)
	for _, neighbor := range graph[node] {
		if _, ok := visited[neighbor]; !ok {
			dfs(graph, neighbor, visited, path, cycles)
		} else if slices.Contains(path, neighbor) {
			cycle := removeElementsBefore(path, neighbor)
			cycles[int(hashcode(cycle))] = cycle
		}
	}
}

func findCycles(graph map[string][]string) ([][]string, bool) {
	cycles := make(map[int][]string)

	for node := range graph {
		dfs(graph, node, make(map[string]struct{}), []string{}, cycles)
	}
	return convertMapToSlice(cycles), len(cycles) != 0
}

func removeElementsBefore(slice []string, element string) []string {
	for i, val := range slice {
		if val == element {
			newSlice := make([]string, len(slice[i:]))
			copy(newSlice, slice[i:])
			return newSlice
		}
	}
	return []string{}
}

func convertMapToSlice(cycles map[int][]string) [][]string {
	result := make([][]string, 0, len(cycles))

	for _, cycle := range cycles {
		result = append(result, cycle)
	}

	return result
}

func hashcode(slice []string) uint32 {
	combined := strings.Join(slice, ",")

	hash := crc32.ChecksumIEEE([]byte(combined))

	return hash
}

func topologicalSort(graph map[string][]string) ([]string, error) {
	inDegree := make(map[string]int)
	for node := range graph {
		if _, ok := inDegree[node]; !ok {
			inDegree[node] = 0
		}
		for _, neighbor := range graph[node] {
			inDegree[neighbor]++
		}
	}

	var zeroInDegree []string
	for node, degree := range inDegree {
		if degree == 0 {
			zeroInDegree = append(zeroInDegree, node)
		}
	}

	var result []string
	for len(zeroInDegree) > 0 {
		node := zeroInDegree[0]
		zeroInDegree = zeroInDegree[1:]
		result = append(result, node)

		for _, neighbor := range graph[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				zeroInDegree = append(zeroInDegree, neighbor)
			}
		}
	}

	return result, nil

	/*
		Return this code to do strict topological sort
		if len(result) == len(inDegree) {
			return result, nil
		}
		return nil, fmt.Errorf("graph has a cycle or is disconnected")*/
}

func topologicalSortAndPrint(graph map[string][]string) {

	sorted, err := topologicalSort(graph)
	if err != nil {
		fmt.Println("sort not successful:", err)
	}

	file, err := os.Create("graph.dot")
	if err != nil {
		fmt.Println("file not created:", err)
	}
	defer file.Close()

	_, _ = fmt.Fprintln(file, "digraph G {")
	_, _ = fmt.Fprintln(file, "rankdir=TB;")
	_, _ = fmt.Fprintln(file, "splines=ortho;")
	_, _ = fmt.Fprintln(file, "concentrate=true;")

	nodeRanks := make(map[string]int)
	for rank, node := range sorted {
		nodeRanks[node] = rank
	}

	for node, rank := range nodeRanks {
		_, _ = fmt.Fprintf(file, "{rank=%d; \"%s\";}\n", rank, node)
	}

	for from, tos := range graph {
		for _, to := range tos {
			_, _ = fmt.Fprintf(file, "\"%s\" -> \"%s\";\n", from, to)
		}
	}

	_, _ = fmt.Fprintln(file, "}")
}

func prettyPrintMatrixToFile(matrix [][]string, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, row := range matrix {
		_, _ = fmt.Fprintf(file, "[")
		for i, elem := range row {
			if i != len(row)-1 {
				_, _ = fmt.Fprintf(file, "%s, ", elem)
			} else {
				_, _ = fmt.Fprintf(file, "%s", elem)
			}
		}
		_, _ = fmt.Fprintln(file, "]")
	}
	return nil
}

func isDuplicateWithShift(list1, list2 []string) bool {
	if len(list1) != len(list2) {
		return false
	}

	doubleList1 := strings.Join(list1, ",") + "," + strings.Join(list1, ",")
	joinedList2 := strings.Join(list2, ",")

	return strings.Contains(doubleList1, joinedList2)
}

func removeDuplicates(lists [][]string) [][]string {
	uniqueLists := [][]string{}

	for i := 0; i < len(lists); i++ {
		isDuplicate := false

		for j := 0; j < len(uniqueLists); j++ {
			if isDuplicateWithShift(lists[i], uniqueLists[j]) {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			uniqueLists = append(uniqueLists, lists[i])
		}
	}

	sort.Slice(uniqueLists, func(i, j int) bool {
		return len(uniqueLists[i]) < len(uniqueLists[j])
	})
	return uniqueLists
}

func prettyPrintMatrixDotToFile(matrix [][]string, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	edges := make(map[string]bool)

	_, err = file.WriteString("digraph G {\n")
	_, _ = fmt.Fprintln(file, "rankdir=TB;")
	_, _ = fmt.Fprintln(file, "splines=ortho;")
	_, _ = fmt.Fprintln(file, "concentrate=true;")
	if err != nil {
		return err
	}

	for _, cycle := range matrix {
		for i := 0; i < len(cycle); i++ {
			from := cycle[i]
			to := cycle[(i+1)%len(cycle)]
			edge := fmt.Sprintf("\"%s\" -> \"%s\"", from, to)

			if !edges[edge] {
				edges[edge] = true
				_, err := file.WriteString("    " + edge + ";\n")
				if err != nil {
					return err
				}
			}
		}
	}

	_, err = file.WriteString("}\n")
	if err != nil {
		return err
	}

	return nil
}
