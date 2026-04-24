package metrics

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/samber/lo"
	"github.com/valyala/fastjson"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/core/debug/debugreporter"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/debug"
	"github.com/anyproto/anytype-heart/util/reflection"
)

const (
	unexpectedErrorCode = -1
	parsingErrorCode    = -2
	accountSelect       = "AccountSelect"
	accountStop         = "AccountStop"
	accountStopJson     = "account_stop.json"
)

var (
	maxDuration = time.Second * 10
	cache       = new(methodsCache)

	// reporter is the process-wide debugreporter.Reporter used to emit
	// long-method reports. Wired once at bootstrap via SetReporter; left nil
	// in non-app contexts (unit tests) where the interceptor simply skips
	// reporting.
	reporter atomic.Pointer[debugreporter.Reporter]
)

// SetReporter installs the Reporter used by the long-method interceptor.
// Passing nil clears it (useful in tests). Concurrency-safe.
func SetReporter(r debugreporter.Reporter) {
	if r == nil {
		reporter.Store(nil)
		return
	}
	reporter.Store(&r)
}

func loadReporter() debugreporter.Reporter {
	p := reporter.Load()
	if p == nil {
		return nil
	}
	return *p
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
	if lo.Contains(excludedLongExecutionMethods, methodName) {
		return actualCall(ctx, req)
	}
	done := make(chan struct{})
	start := time.Now()

	go func() {
		select {
		case <-done:
		case <-time.After(maxDuration):
			// Double-check the stack still contains the method: guards
			// against the race where actualCall returned between the timer
			// firing and this goroutine getting scheduled.
			if !cache.hasMethod(methodName) && stackTraceHasMethod(methodName, debug.Stack(true)) {
				reportLongMethod(methodName, start)
				cache.addMethod(methodName)
			}
		}
	}()
	ctx = context.WithValue(ctx, CtxKeyRPC, methodName)
	resp, err := actualCall(ctx, req)
	close(done)
	return resp, err
}

// reportLongMethod dispatches a LONG_RPC report through the registered
// Reporter. The Reporter captures goroutine stacks at this moment (while
// the method is still blocked if the goroutine fires first), so the
// archive contains the useful stack context. If no Reporter is registered
// yet (startup race, unit test), the call is silently skipped.
func reportLongMethod(methodName string, start time.Time) {
	r := loadReporter()
	if r == nil {
		return
	}
	r.Report("LONG_RPC", map[string]any{
		"method":     methodName,
		"durationMs": time.Since(start).Milliseconds(),
	}, debugreporter.Capture{Kind: debugreporter.KindGoroutines})
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
