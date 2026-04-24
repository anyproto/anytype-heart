package metrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/valyala/fastjson"
	"go.uber.org/atomic"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/initialparams"
	"github.com/anyproto/anytype-heart/util/debug"
	"github.com/anyproto/anytype-heart/util/reflection"
)

const (
	unexpectedErrorCode = -1
	parsingErrorCode    = -2
	accountSelect       = "AccountSelect"
	accountStop         = "AccountStop"
	accountStopJson     = "account_stop.json"

	// LongMethodTracePrefix is the filename prefix used by saveLongMethodTrace.
	// Exported so consumers (report summary, cleanup) can identify these files
	// without hard-coding the string.
	LongMethodTracePrefix = "long_method_"
)

var (
	maxDuration = time.Second * 10
	cache       = new(methodsCache)

	// profileCreatedHook, when set, is invoked after saveLongMethodTrace
	// successfully writes a trace file so consumers (currently the profiler
	// component) can broadcast an Event.Debug.ProfileCreated notification
	// without pulling the event sender into this package.
	profileCreatedHook atomic.Value // stores func(reason, reasonDesc, path string, full bool)
)

// SetProfileCreatedHook registers a callback invoked after the middleware
// writes a profile artifact of its own accord (currently only the
// long-method trace file; the memory-growth detector and other callers
// broadcast the event themselves). Passing nil clears the hook.
func SetProfileCreatedHook(f func(reason, reasonDesc, path string, full bool)) {
	if f == nil {
		profileCreatedHook.Store((func(string, string, string, bool))(nil))
		return
	}
	profileCreatedHook.Store(f)
}

func emitProfileCreated(reason, reasonDesc, path string, full bool) {
	v := profileCreatedHook.Load()
	if v == nil {
		return
	}
	f, _ := v.(func(reason, reasonDesc, path string, full bool))
	if f == nil {
		return
	}
	f(reason, reasonDesc, path, full)
}

type methodsCache struct {
	methods map[string]struct{}
	mu      sync.RWMutex
}

func (c *methodsCache) addMethod(method string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.methods == nil {
		c.methods = make(map[string]struct{})
	}
	c.methods[method] = struct{}{}
}

func (c *methodsCache) hasMethod(method string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.methods[method]
	return exists
}

var excludedMethods = []string{
	"BlockSetCarriage",
	"BlockTextSetText",
	"ObjectSearchSubscribe",
	"ObjectSearchUnsubscribe",
	"ObjectSubscribeIds",
	"InitialSetParameters",
	"AppSetDeviceState",
}

func UnaryTraceInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	return SharedTraceInterceptor(ctx, req, extractMethodName(info.FullMethod), handler)
}

func extractMethodName(info string) string {
	// it looks like that, we need the last part /anytype.ClientCommands/FileNodeUsage
	return strings.Split(info, "/")[2]
}

func SharedTraceInterceptor(ctx context.Context, req any, methodName string, actualCall func(ctx context.Context, req any) (any, error)) (any, error) {
	var hotSync bool
	if methodName == accountSelect {
		hotSync = extractHotSync(req.(*pb.RpcAccountSelectRequest))
	}
	start := time.Now().UnixMilli()
	resp, err := actualCall(ctx, req)
	delta := time.Now().UnixMilli() - start
	var event *MethodEvent
	if methodName == accountSelect {
		if hotSync {
			event = toEvent(methodName+"Hot", err, resp, delta)
		} else {
			event = toEvent(methodName+"Cold", err, resp, delta)
		}
		_ = trySendAccountStop()
	} else {
		event = toEvent(methodName, err, resp, delta)
	}

	if event != nil {
		if methodName == accountStop {
			_ = saveAccountStop(event)
		} else {
			Service.Send(event)
		}
	}
	return resp, err
}

func saveAccountStop(event *MethodEvent) error {
	arena := &fastjson.Arena{}

	json := arena.NewObject()
	json.Set("method_name", arena.NewString(event.methodName))
	json.Set("middle_time", arena.NewNumberInt(int(event.middleTime)))
	json.Set("error_code", arena.NewNumberInt(int(event.errorCode)))
	json.Set("description", arena.NewString(event.description))

	data := json.MarshalTo(nil)
	jsonPath := filepath.Join(Service.getWorkingDir(), accountStopJson)
	_ = os.Remove(jsonPath)
	return os.WriteFile(jsonPath, data, 0600)
}

func trySendAccountStop() error {
	jsonPath := filepath.Join(Service.getWorkingDir(), accountStopJson)
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}

	_ = os.Remove(jsonPath)

	parsedJson, err := fastjson.ParseBytes(data)
	if err != nil {
		return err
	}

	Service.Send(&MethodEvent{
		methodName:  string(parsedJson.GetStringBytes("method_name")),
		middleTime:  parsedJson.GetInt64("middle_time"),
		errorCode:   parsedJson.GetInt64("error_code"),
		description: string(parsedJson.GetStringBytes("description")),
	})

	return nil
}

func toEvent(method string, err error, resp any, delta int64) *MethodEvent {
	if !lo.Contains(excludedMethods, method) {
		if err != nil {
			return &MethodEvent{
				methodName:  method,
				errorCode:   unexpectedErrorCode,
				description: err.Error(),
			}
		}
		errorCode, description, err := reflection.GetError(resp)
		if err != nil {
			return &MethodEvent{
				methodName: method,
				errorCode:  parsingErrorCode,
			}
		}
		if errorCode > 0 {
			return &MethodEvent{
				methodName:  method,
				errorCode:   errorCode,
				description: description,
			}
		}
		return &MethodEvent{
			methodName: method,
			middleTime: delta,
		}
	}
	return nil
}

func LongMethodsInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	return SharedLongMethodsInterceptor(ctx, req, extractMethodName(info.FullMethod), handler)
}

var excludedLongExecutionMethods = []string{
	"DebugRunProfiler",
}

func SharedLongMethodsInterceptor(ctx context.Context, req any, methodName string, actualCall func(ctx context.Context, req any) (any, error)) (any, error) {
	return actualCall(ctx, req)
	// todo: noop till GO-7143
	if lo.Contains(excludedLongExecutionMethods, methodName) {
		return actualCall(ctx, req)
	}
	doneCh := make(chan struct{})
	start := time.Now()

	lastTrace := atomic.NewString("")
	go func() {
		select {
		case <-doneCh:
			break
		case <-time.After(maxDuration):
			trace := debug.Stack(true)
			// double check, because we can have a race and the stack trace can be taken after the method is already finished
			if !cache.hasMethod(methodName) && stackTraceHasMethod(methodName, trace) {
				lastTrace.Store(string(trace))
				saveLongMethodTrace(methodName, trace, start)
				cache.addMethod(methodName)
			}
		}
	}()
	ctx = context.WithValue(ctx, CtxKeyRPC, methodName)
	resp, err := actualCall(ctx, req)
	close(doneCh)
	if time.Since(start) > maxDuration {
		if !cache.hasMethod(methodName) {
			trace := []byte(lastTrace.String())
			if len(trace) == 0 {
				trace = debug.Stack(true)
			}
			saveLongMethodTrace(methodName, trace, start)
			cache.addMethod(methodName)
		}
	}
	return resp, err
}

func saveLongMethodTrace(methodName string, trace []byte, start time.Time) {
	dir := initialparams.Get().Paths.ProfilesDir
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Warnw("long-method trace skipped: cannot create profiles dir", "dir", dir, "error", err)
		return
	}
	ts := time.Now().Format("20060102_150405")
	duration := time.Since(start).Truncate(time.Millisecond)
	filename := filepath.Join(dir, fmt.Sprintf("%s%s_%s_%s.txt.gz", LongMethodTracePrefix, methodName, ts, duration))
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Warnw("long-method trace skipped: cannot open file", "filename", filename, "error", err)
		return
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	_, writeErr := gz.Write(trace)
	if writeErr != nil {
		log.Warnw("long-method trace: gzip write failed", "filename", filename, "error", writeErr)
	}
	closeErr := gz.Close()
	if closeErr != nil {
		log.Warnw("long-method trace: gzip close failed", "filename", filename, "error", closeErr)
	}
	if writeErr == nil && closeErr == nil {
		emitProfileCreated("LONG_METHOD", methodName, filename, false)
	}
}

func extractHotSync(req *pb.RpcAccountSelectRequest) bool {
	exists, err := dirExists(filepath.Join(req.RootPath, req.Id))
	if err != nil {
		return false
	}
	return exists
}

func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func stackTraceHasMethod(method string, stackTrace []byte) bool {
	return bytes.Contains(stackTrace, []byte("core.(*Middleware)."+method+"("))
}
