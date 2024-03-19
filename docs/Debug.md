# Debug
## Run local gRPC server to debug
⚠️ Make sure to update/install protobuf compiler from [this repo](https://github.com/anyproto/protobuf) using `make setup-protoc`

Commands:
- `make run-server` - builds proto files for grpc server, builds the binary and runs it
- `make build-server` - builds proto files for grpc server and builds the binary into `dist/server`

If you want to change the default port(9999):

`ANYTYPE_GRPC_ADDR=127.0.0.1:8888 make run-debug`

----
## Useful tools for debug

### Debug server

Firstly, build anytype-heart with build tag `-anydebug`

Use env var ANYDEBUG=address to enable debugging HTTP server. For example: `ANYDEBUG=:6061` will start debug server on port 6061

You can find all endpoints in `/debug` page. For example: http://localhost:6061/debug

### gRPC logging
In order to log mw gRPC requests/responses use `ANYTYPE_GRPC_LOG` env var:
- `ANYTYPE_LOG_LEVEL="grpc=DEBUG" ANYTYPE_GRPC_LOG=1` - log only method names
- `ANYTYPE_LOG_LEVEL="grpc=DEBUG" ANYTYPE_GRPC_LOG=2` - log method names  + payloads for commands
- `ANYTYPE_LOG_LEVEL="grpc=DEBUG" ANYTYPE_GRPC_LOG=3` - log method names  + payloads for commands&events

### gRPC tracing
1. Run jaeger UI on the local machine:
   ```docker run --rm -d -p6832:6832/udp -p6831:6831/udp -p16686:16686 -p5778:5778 -p5775:5775/udp jaegertracing/all-in-one:latest```
2. Run mw with `ANYTYPE_GRPC_TRACE` env var:
- `ANYTYPE_GRPC_TRACE=1` - log only method names/times
- `ANYTYPE_GRPC_TRACE=2` - log method names  + payloads for commands
- `ANYTYPE_GRPC_TRACE=3` - log method names  + payloads for commands&events
3. Open Jaeger UI at http://localhost:16686
4. If you can't see anything use JAEGER_SAMPLER_TYPE="const" and JAEGER_SAMPLER_PARAM=1 env vars to force sampling

### Debug tree
1. You can use `cmd/debugtree.go` to perform different operations with tree exported in zip archive (`rpc DebugTree`)
2. The usage looks like this `go run debugtree.go -j -t -f [path to zip archive]` where `-t` tells the cmd to generate tree graph view and `-j` - to generate json representation of the tree (i.e. data in each individual block)
3. You can use flag `-r` to build the tree from its root, that way you will see all the changes in the tree, and not only those from the common snapshot
3. For more info please check the command usage in `debugtree.go`

### gRPC clients

#### GUI

https://github.com/uw-labs/bloomrpc

HowTo: Set the import path to the middleware root, then select commands.proto file

#### CLI

https://github.com/fullstorydev/grpcurl

You should specify import-path to the root of anytype-heart repository and gRPC port of running application

Command examples:

- List available methods
```
grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto localhost:31007 describe
```

- Describe method signature
```
grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto localhost:31007 describe anytype.ClientCommands.ObjectCreate
```

- Describe structure of specified protobuf message
```
grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto localhost:31007 describe .anytype.Rpc.Object.Create.Request
```

- Call method with specified plain-text payload
```
grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d '{"details": {"name": "hello there", "type": "ot-page"}}' localhost:31007 anytype.ClientCommands.ObjectCreate
```

- Call method using unix pipe
```
echo '{"details": {"name": "hello there", "type": "ot-page"}}' | grpcurl -import-path ../anytype-heart/ -proto pb/protos/service/service.proto -plaintext -d @ localhost:31007 anytype.ClientCommands.ObjectCreate
```

### High memory usage detector
We have service for detecting high memory usage (core/debug/profiler package). It logs profiles, compressed via gzip and represented as base64 string.
To analyze profile:
1) Copy profile value string from log (from Graylog in instance)
2) Decode base64 and decompress gzip. Example for macOS: `pbpaste | base64 -d | gzip -d > mem.profile`
3) Analyze via `go tool pprof mem.profile`

## Running with prometheus and grafana
- `cd metrics/docker` – cd into folder with docker-compose file
- `docker-compose up` - run the prometheus/grafana
- use `ANYTYPE_PROM=0.0.0.0:9094` when running middleware to enable metrics collection. Client commands metrics available only in gRPC mode
- open http://127.0.0.1:3000 to view collected metrics in Grafana. You can find several dashboards there:
    - **MW** internal middleware metrics such as changes, added and created threads histograms
    - **MW commands server** metrics for clients commands. Works only in grpc-server mode
