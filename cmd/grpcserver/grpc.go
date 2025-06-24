//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	// nolint: gosec
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/anyproto/any-sync/app"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/api"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/conc"
	"github.com/anyproto/anytype-heart/util/grpcprocess"
	"github.com/anyproto/anytype-heart/util/vcs"
)

const defaultAddr = "127.0.0.1:31007"
const defaultWebAddr = "127.0.0.1:31008"

// do not change this, js client relies on this msg to ensure that server is up
const grpcWebStartedMessagePrefix = "gRPC Web proxy started at: "

var log = logging.Logger("anytype-grpc-server")
var commonOSSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGINT}

func main() {
	var addr string
	var webaddr string
	app.StartWarningAfter = time.Second * 5
	fmt.Printf("mw grpc: %s\n", vcs.GetVCSInfo().Description())
	if len(os.Args) > 1 {
		addr = os.Args[1]
		if len(os.Args) > 2 {
			webaddr = os.Args[2]
		}
	}

	if addr == "" {
		if env := os.Getenv("ANYTYPE_GRPC_ADDR"); env != "" {
			addr = env
		} else {
			addr = defaultAddr
		}
	}

	if webaddr == "" {
		if env := os.Getenv("ANYTYPE_GRPCWEB_ADDR"); env != "" {
			webaddr = env
		} else {
			webaddr = defaultWebAddr
		}
	}

	if debug, ok := os.LookupEnv("ANYPROF"); ok && debug != "" {
		fmt.Printf("Running GO debug HTTP server at: %s\n", debug)
		go func() {
			http.ListenAndServe(debug, nil)
		}()
	}
	metrics.Service.InitWithKeys(metrics.DefaultInHouseKey)

	var signalChan = make(chan os.Signal, 2)
	signal.Notify(signalChan, signals...)

	var mw = core.New()
	mw.SetEventSender(event.NewGrpcSender())

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	addr = lis.Addr().String()

	webLis, err := net.Listen("tcp", webaddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	webaddr = webLis.Addr().String()
	var (
		unaryInterceptors  []grpc.UnaryServerInterceptor
		streamInterceptors []grpc.StreamServerInterceptor
	)

	if metrics.Enabled {
		unaryInterceptors = append(unaryInterceptors, grpc_prometheus.UnaryServerInterceptor)
	}
	unaryInterceptors = append(unaryInterceptors, metrics.UnaryTraceInterceptor)
	unaryInterceptors = append(unaryInterceptors, func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		resp, err = mw.Authorize(ctx, req, info, handler)
		if err != nil {
			log.Errorf("authorize: %s", err)
		}
		return
	})

	// todo: we may want to change it to the opposite check with a public release
	if os.Getenv("ANYTYPE_GRPC_NO_DEBUG_TIMEOUT") != "1" {
		unaryInterceptors = append(unaryInterceptors, metrics.LongMethodsInterceptor)
	}

	grpcDebug, _ := strconv.Atoi(os.Getenv("ANYTYPE_GRPC_LOG"))
	if grpcDebug > 0 {
		decider := func(_ context.Context, _ string, _ interface{}) bool {
			return true
		}

		grpcLogger := logging.LoggerNotSugared("grpc")

		unaryInterceptors = append(unaryInterceptors, grpc_zap.UnaryServerInterceptor(grpcLogger))
		streamInterceptors = append(streamInterceptors, grpc_zap.StreamServerInterceptor(grpcLogger))
		if grpcDebug > 1 {
			unaryInterceptors = append(unaryInterceptors, grpc_zap.PayloadUnaryServerInterceptor(grpcLogger, decider))
		}
		if grpcDebug > 2 {
			streamInterceptors = append(streamInterceptors, grpc_zap.PayloadStreamServerInterceptor(grpcLogger, decider))
		}
	}

	grpcTrace, _ := strconv.Atoi(os.Getenv("ANYTYPE_GRPC_TRACE"))
	if grpcTrace > 0 {
		jLogger := jaeger.StdLogger

		cfg, err := jaegercfg.FromEnv()
		if err != nil {
			log.Fatal(err)
		}
		if cfg.ServiceName == "" {
			cfg.ServiceName = "mw"
		}
		// Initialize tracer with a logger and a metrics factory
		tracer, closer, err := cfg.NewTracer(jaegercfg.Logger(jLogger))
		if err != nil {
			log.Fatal(err)
		}
		defer closer.Close()

		var (
			unaryOptions  []otgrpc.Option
			streamOptions []otgrpc.Option
		)

		// Set the singleton opentracing.Tracer with the Jaeger tracer.
		opentracing.SetGlobalTracer(tracer)
		if grpcTrace > 1 {
			unaryOptions = append(unaryOptions, otgrpc.LogPayloads())
		}
		if grpcTrace > 2 {
			streamOptions = append(streamOptions, otgrpc.LogPayloads())
		}

		unaryInterceptors = append(unaryInterceptors, otgrpc.OpenTracingServerInterceptor(tracer, unaryOptions...))
		streamInterceptors = append(streamInterceptors, otgrpc.OpenTracingStreamServerInterceptor(tracer, streamOptions...))
	}

	unaryInterceptors = appendInterceptor(unaryInterceptors, mw)
	unaryInterceptors = append(unaryInterceptors, grpcprocess.ProcessInfoInterceptor(
		"/anytype.ClientCommands/AccountLocalLinkNewChallenge",
	))

	server := grpc.NewServer(grpc.MaxRecvMsgSize(20*1024*1024),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
	)

	service.RegisterClientCommandsServer(server, mw)
	if metrics.Enabled {
		grpc_prometheus.EnableHandlingTimeHistogram()
		// grpc_prometheus.Register(server)
	}

	webrpc := grpcweb.WrapServer(
		server,
		grpcweb.WithOriginFunc(func(origin string) bool {
			return true
		}),
		grpcweb.WithWebsockets(true),
		grpcweb.WithWebsocketOriginFunc(func(req *http.Request) bool {
			return true
		}))

	proxy := &http.Server{
		Addr: webaddr,
	}

	proxy.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if webrpc.IsGrpcWebRequest(r) ||
			webrpc.IsAcceptableGrpcCorsRequest(r) ||
			webrpc.IsGrpcWebSocketRequest(r) {
			webrpc.ServeHTTP(w, r)
		}
	})

	go func() {
		server.Serve(lis)
	}()
	fmt.Println("gRPC server started at: " + addr)

	go func() {
		if err := proxy.Serve(webLis); err != nil && err != http.ErrServerClosed {
			log.Fatalf("proxy error: %v", err)
		}
	}()

	startReportMemory(mw)
	api.SetMiddlewareParams(mw)

	shutdown := func() {
		server.Stop()
		proxy.Close()
		mw.AppShutdown(context.Background(), &pb.RpcAppShutdownRequest{})
	}
	// do not change this, js client relies on this msg to ensure that server is up and parse address
	fmt.Println(grpcWebStartedMessagePrefix + webaddr)
	if runtime.GOOS == "windows" {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			message := scanner.Text()
			if message == "shutdown" {
				fmt.Println("[anytype-heart] Shutdown: received shutdown msg, closing components...")
				// Perform cleanup or exit
				shutdown()
				return
			}
		}
	}

	for {
		sig := <-signalChan
		if shouldSaveStack(sig) {
			if err = mw.SaveGoroutinesStack(""); err != nil {
				log.Errorf("failed to save stack of goroutines: %s", err)
			}
			continue
		}
		fmt.Printf("[anytype-heart] Shutdown: received OS signal (%s), closing components...\n", sig.String())
		shutdown()
		return
	}
}

func appendInterceptor(
	unaryInterceptors []grpc.UnaryServerInterceptor,
	mw *core.Middleware,
) []grpc.UnaryServerInterceptor {
	return append(unaryInterceptors, func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				if rerr, ok := r.(error); ok && errors.Is(rerr, core.ErrNotLoggedIn) {
					resp = onNotLoggedInError(resp, rerr)
				} else {
					resp = onDefaultError(mw, r, resp)
				}
			}
		}()

		resp, err = handler(ctx, req)
		return resp, err
	})
}

func onDefaultError(mw *core.Middleware, r any, resp interface{}) interface{} {
	conc.OnPanic(r)
	resp = &pb.RpcGenericErrorResponse{
		Error: &pb.RpcGenericErrorResponseError{
			Code:        pb.RpcGenericErrorResponseError_UNKNOWN_ERROR,
			Description: "panic recovered",
		},
	}
	return resp
}

func onNotLoggedInError(resp interface{}, rerr error) interface{} {
	resp = &pb.RpcGenericErrorResponse{
		Error: &pb.RpcGenericErrorResponseError{
			Code:        pb.RpcGenericErrorResponseError_UNKNOWN_ERROR,
			Description: rerr.Error(),
		},
	}
	return resp
}
