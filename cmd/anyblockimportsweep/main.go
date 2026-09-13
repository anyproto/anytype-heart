// anyblockimportsweep imports retained per-space exports through a fresh headless
// account, recording asynchronous import errors for every source directory.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anyproto/anytype-heart/cmd/apiv2eval/heartboot"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
)

type input struct {
	ID         string `json:"sourceSpaceId"`
	Path       string `json:"path"`
	HasIndex   bool   `json:"hasIndex"`
	SkipReason string `json:"skipReason,omitempty"`
}
type issue struct {
	Stage   string `json:"stage"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}
type outcome struct {
	Input       input         `json:"input"`
	SpaceID     string        `json:"destinationSpaceId,omitempty"`
	ProcessID   string        `json:"processId,omitempty"`
	Status      string        `json:"status"`
	Duration    time.Duration `json:"durationNs"`
	Errors      []issue       `json:"errors"`
	Diagnostics []string      `json:"logDiagnostics,omitempty"`
}
type report struct {
	Started   time.Time  `json:"started"`
	Finished  *time.Time `json:"finished,omitempty"`
	AccountID string     `json:"accountId,omitempty"`
	DataDir   string     `json:"dataDir,omitempty"`
	Results   []outcome  `json:"results"`
}
type options struct {
	Inputs                []string
	Output, Binary, Space string
	Limit                 int
	Timeout               time.Duration
	Keep                  bool
}

// Later roots override earlier exports of the same source space (for repaired
// exports). Empty directories remain inputs so missing exports are visible.
func discover(roots []string) ([]input, error) {
	found := map[string]input{}
	var skips map[string]string
	add := func(name string) error {
		absolute, err := filepath.Abs(name)
		if err != nil {
			return err
		}
		_, err = os.Stat(filepath.Join(absolute, "index.json"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		found[filepath.Base(absolute)] = input{ID: filepath.Base(absolute), Path: absolute, HasIndex: err == nil, SkipReason: skips[filepath.Base(absolute)]}
		return nil
	}
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("input must be a bundle or per-space root directory: %s", root)
		}
		manifestRoot := filepath.Dir(filepath.Clean(root))
		_, indexErr := os.Stat(filepath.Join(root, "index.json"))
		if indexErr == nil {
			manifestRoot = filepath.Dir(manifestRoot)
		}
		skips, err = readSkippedSpaces(manifestRoot)
		if err != nil {
			return nil, err
		}
		if indexErr == nil {
			if err = add(root); err != nil {
				return nil, err
			}
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				if err = add(filepath.Join(root, entry.Name())); err != nil {
					return nil, err
				}
			}
		}
	}
	result := make([]input, 0, len(found))
	for _, in := range found {
		result = append(result, in)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

type received struct {
	Done         *pb.ModelProcess
	Notification *model.Notification
	Err          error
}

func receive(ctx context.Context, stream interface{ Recv() (*pb.Event, error) }) <-chan received {
	out := make(chan received, 256)
	go func() {
		defer close(out)
		for {
			event, err := stream.Recv()
			if err != nil {
				select {
				case out <- received{Err: err}:
				case <-ctx.Done():
				}
				return
			}
			for _, message := range event.Messages {
				item := received{Done: message.GetProcessDone().GetProcess(), Notification: message.GetNotificationSend().GetNotification()}
				if item.Done == nil && (item.Notification == nil || (item.Notification.GetImport() == nil && item.Notification.GetTest() == nil)) {
					continue
				}
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

// An ObjectImport RPC only acknowledges scheduling. Correlate its notification
// by destination space and then its ProcessDone by ID, in either arrival order.
func awaitImport(ctx context.Context, events <-chan received, spaceID string) (*pb.ModelProcess, *model.NotificationImport, error) {
	done := map[string]*pb.ModelProcess{}
	var notification *model.NotificationImport
	for {
		if notification != nil {
			if process := done[notification.ProcessId]; process != nil {
				return process, notification, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, notification, ctx.Err()
		case event, ok := <-events:
			if !ok {
				return nil, notification, errors.New("event stream closed")
			}
			if event.Err != nil {
				return nil, notification, event.Err
			}
			if event.Done != nil {
				done[event.Done.Id] = event.Done
			}
			if value := event.Notification.GetImport(); value != nil && value.SpaceId == spaceID {
				notification = value
			}
		}
	}
}

func subscribe(ctx context.Context, h *heartboot.Heart) (<-chan received, error) {
	stream, err := h.ListenSessionEvents(ctx)
	if err != nil {
		return nil, err
	}
	events := receive(ctx, stream)
	// A test notification is a registration barrier; opening the stream alone
	// does not guarantee the server has installed the session event sender.
	ready, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ready.Done():
			return nil, fmt.Errorf("subscribe to import events: %w", ready.Err())
		case event, ok := <-events:
			if !ok {
				return nil, errors.New("event stream closed during subscription")
			}
			if event.Err != nil {
				return nil, event.Err
			}
			if event.Notification.GetTest() != nil {
				return events, nil
			}
		case <-ticker.C:
			response, err := h.GRPCClient().NotificationTest(h.GRPCContext(ready), &pb.RpcNotificationTestRequest{})
			if err != nil {
				return nil, err
			}
			if response.GetError().GetCode() != 0 {
				return nil, fmt.Errorf("notification barrier: %s", response.GetError())
			}
		}
	}
}

// Keep log diagnostics separate from RPC failures: background service errors
// are useful evidence but cannot always be attributed to the current import.
type diagnosticLog struct {
	mu    sync.Mutex
	file  *os.File
	lines []string
}

func (l *diagnosticLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "warn") || strings.Contains(lower, "error") {
			l.lines = append(l.lines, line)
		}
	}
	return l.file.Write(data)
}
func (l *diagnosticLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	lines := l.lines
	l.lines = nil
	return lines
}
func save(output string, r *report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(output, "report.json")
	if err = os.WriteFile(path+".tmp", append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func run(ctx context.Context, opt options) (*report, error) {
	inputs, err := discover(opt.Inputs)
	if err != nil {
		return nil, err
	}
	if opt.Space != "" {
		filtered := inputs[:0]
		for _, in := range inputs {
			if in.ID == opt.Space {
				filtered = append(filtered, in)
			}
		}
		inputs = filtered
	}
	if opt.Limit > 0 {
		limited := inputs[:0]
		active := 0
		for _, in := range inputs {
			if in.SkipReason != "" {
				limited = append(limited, in)
				continue
			}
			if active < opt.Limit {
				limited = append(limited, in)
				active++
			}
		}
		inputs = limited
	}
	if len(inputs) == 0 {
		return nil, errors.New("no space export directories found")
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 10 * time.Minute
	}
	if err = os.MkdirAll(opt.Output, 0700); err != nil {
		return nil, err
	}
	// Refuse to overwrite a previous report or its account log.
	file, err := os.OpenFile(filepath.Join(opt.Output, "heart.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	logs := &diagnosticLog{file: file}
	r := &report{Started: time.Now().UTC()}
	active := 0
	for _, in := range inputs {
		status := "not_attempted"
		if in.SkipReason != "" {
			status = "skipped"
		} else {
			active++
		}
		r.Results = append(r.Results, outcome{Input: in, Status: status, Errors: []issue{}})
	}
	if active == 0 {
		finished := time.Now().UTC()
		r.Finished = &finished
		return r, save(opt.Output, r)
	}
	if err = save(opt.Output, r); err != nil {
		return r, err
	}
	h, err := heartboot.Start(ctx, heartboot.Options{BinaryPath: opt.Binary, KeepDataDir: opt.Keep, AccountName: "AnyBlock import sweep", AppName: "anyblockimportsweep", Log: logs})
	if err != nil {
		return r, err
	}
	defer h.Stop()
	r.AccountID = h.AccountId
	if opt.Keep {
		r.DataDir = h.DataDir
	}
	eventCtx, cancelEvents := context.WithCancel(ctx)
	defer cancelEvents()
	events, err := subscribe(eventCtx, h)
	if err != nil {
		return r, err
	}
	failures := 0
	for index := range r.Results {
		row := &r.Results[index]
		if row.Status == "skipped" {
			fmt.Printf("[%d/%d] skipped %s: %s\n", index+1, len(r.Results), row.Input.ID, row.Input.SkipReason)
			continue
		}
		started := time.Now()
		logs.take()
		attempt, cancel := context.WithTimeout(ctx, opt.Timeout)
		row.Status = "running"
		if err = save(opt.Output, r); err != nil {
			cancel()
			return r, err
		}
		response, e := h.GRPCClient().WorkspaceCreate(h.GRPCContext(attempt), &pb.RpcWorkspaceCreateRequest{Details: &types.Struct{Fields: map[string]*types.Value{"name": {Kind: &types.Value_StringValue{StringValue: "Sweep " + row.Input.ID}}}}})
		stage := "create_space"
		if e == nil && response.GetError().GetCode() != 0 {
			e = fmt.Errorf("%s", response.GetError())
		}
		if e == nil && response.GetSpaceId() == "" {
			e = errors.New("space creation returned no ID")
		}
		uncertain := false
		if e == nil {
			row.SpaceID = response.SpaceId
			stage = "import_rpc"
			ack, rpcErr := h.GRPCClient().ObjectImport(h.GRPCContext(attempt), &pb.RpcObjectImportRequest{SpaceId: row.SpaceID, Type: model.Import_Pb, Mode: pb.RpcObjectImportRequest_ALL_OR_NOTHING, IsNewSpace: true, Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{row.Input.Path}, NoCollection: true}}})
			e = rpcErr
			if e == nil && ack.GetError().GetCode() != 0 {
				e = fmt.Errorf("%s", ack.GetError())
			}
			if e == nil {
				stage = "import_completion"
				process, notification, waitErr := awaitImport(attempt, events, row.SpaceID)
				e = waitErr
				uncertain = e != nil
				if notification != nil {
					row.ProcessID = notification.ProcessId
					if notification.ErrorCode != 0 {
						row.Errors = append(row.Errors, issue{Stage: "notification", Code: notification.ErrorCode.String(), Message: "import reported failure"})
					}
					if notification.IssuesCount > 0 {
						row.Errors = append(row.Errors, issue{Stage: "notification", Message: fmt.Sprintf("%d issues; report object %s", notification.IssuesCount, notification.ReportObjectId)})
					}
				}
				if process != nil && process.Error == "" && (process.State == pb.ModelProcess_Error || process.State == pb.ModelProcess_Canceled) {
					row.Errors = append(row.Errors, issue{Stage: "process", Code: process.State.String(), Message: "import did not finish successfully"})
				}
				if process != nil && process.Error != "" {
					row.Errors = append(row.Errors, issue{Stage: "process", Message: process.Error})
				}
			} else {
				uncertain = rpcErr != nil
			}
		}
		cancel()
		if e != nil {
			row.Errors = append(row.Errors, issue{Stage: stage, Message: e.Error()})
		}
		row.Diagnostics = logs.take()
		row.Errors = append(row.Errors, importLogErrors(row.Diagnostics)...)
		row.Duration = time.Since(started)
		row.Status = "passed"
		if len(row.Errors) > 0 {
			row.Status = "failed"
			failures++
		}
		fmt.Printf("[%d/%d] %s %s (%s)\n", index+1, len(r.Results), row.Status, row.Input.ID, row.Duration.Round(time.Millisecond))
		if err = save(opt.Output, r); err != nil {
			return r, err
		}
		// Continuing after an unknown completion could overlap imports and corrupt
		// attribution. Completed failures are safe to continue past.
		if uncertain {
			return r, fmt.Errorf("import completion unknown; stopped with remaining spaces unattempted: %w", e)
		}
	}
	finished := time.Now().UTC()
	r.Finished = &finished
	if err = save(opt.Output, r); err != nil {
		return r, err
	}
	if failures > 0 {
		return r, fmt.Errorf("%d/%d imports failed; see %s", failures, active, filepath.Join(opt.Output, "report.json"))
	}
	return r, nil
}

func main() {
	var opt options
	flag.StringVar(&opt.Output, "output", "", "new report directory (required)")
	flag.StringVar(&opt.Binary, "heart-binary", "", "optional prebuilt grpcserver; default builds this checkout")
	flag.StringVar(&opt.Space, "space", "", "only this source space directory")
	flag.IntVar(&opt.Limit, "limit", 0, "maximum number of spaces; 0 means all")
	flag.DurationVar(&opt.Timeout, "timeout", 10*time.Minute, "timeout per space")
	flag.BoolVar(&opt.Keep, "keep-account", false, "retain the newly created scratch account")
	flag.Parse()
	opt.Inputs = flag.Args()
	if opt.Output == "" || len(opt.Inputs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: anyblockimportsweep -output NEW_DIR [flags] NATIVE_ROOT [REPAIRED_NATIVE_ROOT ...]")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if _, err := run(ctx, opt); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Older import stages log some object/file failures without propagating them
// to ProcessDone. Count those as failures too, keeping unrelated service logs
// only as diagnostics.
func importLogErrors(lines []string) []issue {
	var result []issue
	for _, line := range lines {
		var entry struct {
			Level   string `json:"level"`
			Logger  string `json:"logger"`
			Message string `json:"msg"`
			Error   string `json:"error"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if entry.Level != "ERROR" || (entry.Logger != "import" && !strings.HasPrefix(entry.Logger, "import.") && !strings.HasPrefix(entry.Logger, "import-")) {
			continue
		}
		message := entry.Message
		if entry.Error != "" {
			message += ": " + entry.Error
		}
		result = append(result, issue{Stage: "import_log", Message: message})
	}
	return result
}
