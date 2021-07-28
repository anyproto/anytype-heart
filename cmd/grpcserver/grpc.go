// +build !nogrpcserver,!_test

package main

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"

	"github.com/anytypeio/go-anytype-middleware/core"
	"github.com/anytypeio/go-anytype-middleware/pb/service"

	jaegercfg "github.com/uber/jaeger-client-go/config"
)

const defaultAddr = "127.0.0.1:31007"
const defaultWebAddr = "127.0.0.1:31008"

// do not change this, js client relies on this msg to ensure that server is up
const grpcWebStartedMessagePrefix = "gRPC Web proxy started at: "

var log = logging.Logger("anytype-grpc-server")

func main() {
	var addr string
	var webaddr string

	fmt.Printf("mw grpc: %s\n", core.GetVersionDescription())
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
		go func() {
			http.ListenAndServe(debug, nil)
		}()
	}
	metrics.SharedClient.InitWithKey(metrics.DefaultAmplitudeKey)

	var stopChan = make(chan os.Signal, 2)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	var mw = core.New()
	mw.EventSender = event.NewGrpcSender()

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

	grpcDebug, _ := strconv.Atoi(os.Getenv("ANYTYPE_GRPC_LOG"))
	if grpcDebug > 0 {
		decider := func(_ context.Context, _ string, _ interface{}) bool {
			return true
		}

		grpcLogger := logging.Logger("grpc")

		unaryInterceptors = append(unaryInterceptors, grpc_zap.UnaryServerInterceptor(grpcLogger.Desugar()))
		streamInterceptors = append(streamInterceptors, grpc_zap.StreamServerInterceptor(grpcLogger.Desugar()))
		if grpcDebug > 1 {
			unaryInterceptors = append(unaryInterceptors, grpc_zap.PayloadUnaryServerInterceptor(grpcLogger.Desugar(), decider))
		}
		if grpcDebug > 2 {
			streamInterceptors = append(streamInterceptors, grpc_zap.PayloadStreamServerInterceptor(grpcLogger.Desugar(), decider))
		}
	}

	grpcTrace, _ := strconv.Atoi(os.Getenv("ANYTYPE_GRPC_TRACE"))
	if grpcTrace > 0 {
		jLogger := jaeger.StdLogger

		cfg, err := jaegercfg.FromEnv()
		if err != nil {
			log.Fatal(err.Error())
		}
		if cfg.ServiceName == "" {
			cfg.ServiceName = "mw"
		}
		// Initialize tracer with a logger and a metrics factory
		tracer, closer, err := cfg.NewTracer(jaegercfg.Logger(jLogger))
		if err != nil {
			log.Fatal(err.Error())
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

	server := grpc.NewServer(grpc.MaxRecvMsgSize(20*1024*1024),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
	)

	service.RegisterClientCommandsServer(server, mw)
	if metrics.Enabled {
		grpc_prometheus.EnableHandlingTimeHistogram()
		//grpc_prometheus.Register(server)
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

	// do not change this, js client relies on this msg to ensure that server is up and parse address
	fmt.Println(grpcWebStartedMessagePrefix + webaddr)

	select {
	case <-stopChan:
		server.Stop()
		proxy.Close()
		mw.Shutdown(&pb.RpcShutdownRequest{})
		return
	}
}
