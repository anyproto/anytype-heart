package metrics

import (
	"bytes"
	"context"
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
)

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
	doneCh := make(chan struct{})
	start := time.Now()

	lastTrace := atomic.NewString("")
	l := log.With("method", methodName)
	go func() {
		select {
		case <-doneCh:
			break
		case <-time.After(maxDuration):
			trace := debug.Stack(true)
			// double check, because we can have a race and the stack trace can be taken after the method is already finished
			if !cache.hasMethod(methodName) && stackTraceHasMethod(methodName, trace) {
				lastTrace.Store(string(trace))
				traceCompressed := debug.CompressBytes(trace)
				l.With("ver", 2).
					With("in_progress", true).
					With("goroutines", traceCompressed).
					With("total", time.Since(start).Milliseconds()).
					Warnf("grpc unary request is taking too long")
				cache.addMethod(methodName)
			}
		}
	}()
	ctx = context.WithValue(ctx, CtxKeyRPC, methodName)
	resp, err := actualCall(ctx, req)
	close(doneCh)
	if time.Since(start) > maxDuration {
		if !cache.hasMethod(methodName) {
			// todo: save long stack trace to files
			lastTraceB := debug.CompressBytes([]byte(lastTrace.String()))
			l.With("ver", 2).
				With("error", err).
				With("in_progress", false).
				With("goroutines", lastTraceB).
				With("total", time.Since(start).Milliseconds()).
				Warnf("grpc unary request took too long")
			cache.addMethod(methodName)
		}

		Service.Send(
			&LongMethodEvent{
				methodName: methodName,
				middleTime: time.Since(start).Milliseconds(),
				stack:      debug.ParseGoroutinesDump(lastTrace.String(), "core.(*Middleware)."+methodName),
			},
		)
	}
	return resp, err
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
