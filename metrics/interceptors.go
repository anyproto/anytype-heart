package metrics

import (
	"context"
	"time"

	"github.com/anyproto/anytype-heart/util/debug"
)

const defaultUnaryWarningAfter = time.Second * 3

func SharedLongMethodsInterceptor(ctx context.Context, req any, methodName string, actualCall func(ctx context.Context, req any) (any, error)) (any, error) {
	doneCh := make(chan struct{})
	start := time.Now()

	l := log.With("method", methodName)

	go func() {
		select {
		case <-doneCh:
		case <-time.After(defaultUnaryWarningAfter):
			l.With("in_progress", true).With("goroutines", debug.StackCompact(true)).With("total", defaultUnaryWarningAfter.Milliseconds()).Warnf("grpc unary request is taking too long")
		}
	}()
	ctx = context.WithValue(ctx, CtxKeyRPC, methodName)
	resp, err := actualCall(ctx, req)
	close(doneCh)
	if time.Since(start) > defaultUnaryWarningAfter {
		l.With("error", err).With("in_progress", false).With("total", time.Since(start).Milliseconds()).Warnf("grpc unary request took too long")
	}
	return resp, err
}
