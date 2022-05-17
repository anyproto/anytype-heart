## Anytype Middleware Library

### Build from Source
1. Install Golang 1.17.* [from here](http://golang.org/dl/) or using preferred package manager
2. Follow instructions below for the target systems

#### Build and install for the [desktop client](https://github.com/anytypeio/js-anytype)
1. `make install-dev-js` to build the local server and copy it and protobuf binding into `../js-anytype`

#### Build for iOS
1. `make build-ios` to build the framework into `dist/ios` folder
2. `make protos-swift` to generate swift protobuf bindings into `dist/ios/pb`

#### Build for Android
1. `make build-android` to build the library into `dist/android` folder
2. `make protos-java` to generate java protobuf bindings into `dist/android/pb`

### Rebuild protobuf generated files
First, you need to install [protobuf](https://github.com/protocolbuffers/protobuf) pkg using your preferred package manager.
This repo uses custom protoc located at [anytypeio/protobuf](https://github.com/anytypeio/protobuf/tree/master/protoc-gen-gogo). It adds `gomobile` plugin and some env-controlled options to control the generated code style.
This protobuf generator will replace your `protoc` binary, BTW it doesn't have any breaking changes for other protobuf and grpc code

You can override the binary with a simple command:
```
make setup-protoc
```

Then you can easily regenerate proto files:
```
make protos
```

#### Run tests
Generate mocks:
```
make test-deps
```

GO test:
```
make test
```

NodeJS addon test:
```
cd jsaddon
npm run test
```

### Run local gRPC server to debug
⚠️ Make sure to update/install protobuf compiler from [this repo](https://github.com/anytypeio/protobuf) using `make setup-protoc`

Commands:
- `make run-server` - builds proto files for grpc server, builds the binary and runs it
- `make build-server` - builds proto files for grpc server and builds the binary into `dist/server`

If you want to change the default port(9999):

`ANYTYPE_GRPC_ADDR=127.0.0.1:8888 make run-debug`

----
### Useful tools for debug

#### gRPC logging
In order to log mw gRPC requests/responses use `ANYTYPE_GRPC_LOG` env var:
- `ANYTYPE_LOG_LEVEL="grpc=DEBUG" ANYTYPE_GRPC_LOG=1` - log only method names   
- `ANYTYPE_LOG_LEVEL="grpc=DEBUG" ANYTYPE_GRPC_LOG=2` - log method names  + payloads for commands
- `ANYTYPE_LOG_LEVEL="grpc=DEBUG" ANYTYPE_GRPC_LOG=2` - log method names  + payloads for commands&events

#### gRPC tracing
1. Run jaeger UI on the local machine: 
```docker run --rm -d -p6832:6832/udp -p6831:6831/udp -p16686:16686 -p5778:5778 -p5775:5775/udp jaegertracing/all-in-one:latest```
2. Run mw with `ANYTYPE_GRPC_TRACE` env var:
- `ANYTYPE_GRPC_TRACE=1` - log only method names/times
- `ANYTYPE_GRPC_TRACE=2` - log method names  + payloads for commands
- `ANYTYPE_GRPC_TRACE=2` - log method names  + payloads for commands&events
3. Open Jaeger UI at http://localhost:16686


**GUI**

https://github.com/uw-labs/bloomrpc

`HowTo: Set the import path to the middleware root, then select commands.proto file`

**CLI**

https://github.com/njpatel/grpcc

### Running with prometheus and grafana
- `cd metrics/docker` – cd into folder with docker-compose file
- `docker-compose up` - run the prometheus/grafana
- use `ANYTYPE_PROM=0.0.0.0:9094` when running middleware to enable metrics collection. Client commands metrics available only in gRPC mode
- open http://127.0.0.1:3000 to view collected metrics in Grafana. You can find several dashboards there:
    - **Threads gRPC client** for go-threads client metrics(when you make requests to other nodes)
    - **Threads gRPC server** for go-threads server metrics(when other nodes make requests to you)
    - **MW** internal middleware metrics such as changes, added and created threads histograms
    - **MW commands server** metrics for clients commands. Works only in grpc-server mode
