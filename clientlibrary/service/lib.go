package service

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"sync"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/conc"
	"github.com/anyproto/anytype-heart/util/vcs"
)

var log = logging.Logger("anytype-mw-library")

var mw = core.New()

func init() {
	fixTZ()
	// if android
	if runtime.GOOS == "android" {
		// disable GSO on android because incorrect detection
		// https://github.com/quic-go/quic-go/pull/4447
		os.Setenv("QUIC_GO_DISABLE_GSO", "1")
	}
	fmt.Printf("mw lib: %s\n", vcs.GetVCSInfo().Description())

	PanicHandler = conc.OnPanic
	metrics.Service.InitWithKeys(metrics.DefaultInHouseKey)
	registerClientCommandsHandler(
		&ClientCommandsHandlerProxy{
			client: mw,
			interceptors: []func(ctx context.Context, req any, methodName string, actualCall func(ctx context.Context, req any) (any, error)) (any, error){
				metrics.SharedTraceInterceptor,
				metrics.SharedLongMethodsInterceptor,
			},
		})
	if addr, ok := os.LookupEnv("ANYPROF"); ok && addr != "" {
		RunDebugServer(addr)
	}
}

func SetEventHandler(eh func(event *pb.Event)) {
	mw.SetEventSender(event.NewCallbackSender(eh))
}

var debugServerOnce sync.Once

func RunDebugServer(addr string) {
	fmt.Printf("Running GO debug HTTP server at: %s\n", addr)
	debugServerOnce.Do(func() {
		go func() {
			http.ListenAndServe(addr, nil)
		}()
	})
}

func SetEnv(key, value string) {
	os.Setenv(key, value)
}

func SetEventHandlerMobile(eh MessageHandler) {
	SetEventHandler(func(event *pb.Event) {
		if len(event.Messages) == 0 {
			return
		}
		b, err := proto.Marshal(event)
		if err != nil {
			log.Errorf("eventHandler failed to marshal error: %s", err.Error())
		}
		eh.Handle(b)
	})
}
