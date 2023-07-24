# Anytype Heart
Middleware library for Anytype.

## Build from Source
1. Install Golang 1.19.x [from here](http://golang.org/dl/) or using preferred package manager
2. Follow instructions below for the target systems

### Build and install for the [desktop client](https://github.com/anyproto/anytype-ts)
`make install-dev-js` — build the local server and copy it and protobuf binding into `../anytype-ts`

Parameters:
- `ANY_SYNC_NETWORK=/path/to/network.yml` — build using self-hosted [network configuration](https://tech.anytype.io/anytype-heart/configuration)

### Build for iOS
Instructions to set up environment for iOS: [here](https://github.com/anyproto/anytype-swift/blob/main/docs/Setup_For_Middleware.md)
1. `make build-ios` to build the framework into `dist/ios` folder

   Parameters:
    - `ANY_SYNC_NETWORK=/path/to/network.yml` — build using self-hosted [network configuration](https://tech.anytype.io/anytype-heart/configuration)
2. `make protos-swift` to generate swift protobuf bindings into `dist/ios/pb`

### Build for Android
Instructions to setup environment for Android: [here](https://github.com/anyproto/anytype-kotlin/blob/main/docs/Setup_For_Middleware.md)
1. `make build-android` to build the library into `dist/android` folder

   Parameters:
    - `ANY_SYNC_NETWORK=/path/to/network.yml` — build using self-hosted [network configuration](https://tech.anytype.io/anytype-heart/configuration)
2. `make protos-java` to generate java protobuf bindings into `dist/android/pb`

## Rebuild protobuf generated files
First, you need to install [protobuf](https://github.com/anyproto/anytype-heart#install-local-deps-mac) pkg using your preferred package manager.
This repo uses custom protoc located at [anyproto/protobuf](https://github.com/anyproto/protobuf/tree/master/protoc-gen-gogo). It adds `gomobile` plugin and some env-controlled options to control the generated code style.
This protobuf generator will replace your `protoc` binary, BTW it doesn't have any breaking changes for other protobuf and grpc code

You can override the binary with a simple command:
```
make setup-protoc
```

Then you can easily regenerate proto files:
```
make protos
```

## Run tests
Install dependencies for running tests and generate mocks:
```
make test-deps
```

GO test:
```
make test
```
You'll need to install latest (at least clang 15)
```
brew install llvm 
echo 'export PATH="/<homebrew location>/llvm/bin:$PATH"' >> ~/.zshrc 
```

### Integration tests
First you need to start a docker container via docker-compose:
```
export ANYTYPE_TEST_GRPC_PORT=31088
docker-compose up -d
```

Then you can run the basic integration tests:
```
make test-integration
```


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
Use env var ANYDEBUG=address to enable debugging HTTP server. For example: `ANYDEBUG=:6061` will start debug server on port 6061

You can find all endpoints in `/debug` page. For example: http://localhost:6061/debug

### gRPC logging
In order to log mw gRPC requests/responses use `ANYTYPE_GRPC_LOG` env var:
- `ANYTYPE_LOG_LEVEL="grpc=DEBUG" ANYTYPE_GRPC_LOG=1` - log only method names   
- `ANYTYPE_LOG_LEVEL="grpc=DEBUG" ANYTYPE_GRPC_LOG=2` - log method names  + payloads for commands
- `ANYTYPE_LOG_LEVEL="grpc=DEBUG" ANYTYPE_GRPC_LOG=2` - log method names  + payloads for commands&events

### gRPC tracing
1. Run jaeger UI on the local machine: 
```docker run --rm -d -p6832:6832/udp -p6831:6831/udp -p16686:16686 -p5778:5778 -p5775:5775/udp jaegertracing/all-in-one:latest```
2. Run mw with `ANYTYPE_GRPC_TRACE` env var:
- `ANYTYPE_GRPC_TRACE=1` - log only method names/times
- `ANYTYPE_GRPC_TRACE=2` - log method names  + payloads for commands
- `ANYTYPE_GRPC_TRACE=2` - log method names  + payloads for commands&events
3. Open Jaeger UI at http://localhost:16686

### Debug tree
1. You can use `cmd/debugtree.go` to perform different operations with tree exported in zip archive (`rpc DebugTree`)
2. The usage looks like this `go run debugtree.go -j -t -f [path to zip archive]` where `-t` tells the cmd to generate tree graph view and `-j` - to generate json representation of the tree (i.e. data in each individual block)
3. You can use flag `-r` to build the tree from its root, that way you will see all the changes in the tree, and not only those from the common snapshot
3. For more info please check the command usage in `debugtree.go`

**GUI**

https://github.com/uw-labs/bloomrpc

`HowTo: Set the import path to the middleware root, then select commands.proto file`

**CLI**

https://github.com/njpatel/grpcc

## Running with prometheus and grafana
- `cd metrics/docker` – cd into folder with docker-compose file
- `docker-compose up` - run the prometheus/grafana
- use `ANYTYPE_PROM=0.0.0.0:9094` when running middleware to enable metrics collection. Client commands metrics available only in gRPC mode
- open http://127.0.0.1:3000 to view collected metrics in Grafana. You can find several dashboards there:
    - **MW** internal middleware metrics such as changes, added and created threads histograms
    - **MW commands server** metrics for clients commands. Works only in grpc-server mode
    
    
## Install local deps (Mac)
As of 16.01.23 last protobuf version (21.12) broke the JS plugin support, so you can use the v3 branch:
```
brew install protobuf@3
```

To generate Swift protobuf:
```
brew install swift-protobuf
```

## Install local deps (Debian-Ubuntu)
We need to have protoc binary (3.x version) and libprotoc headers in orderto build the grpc-web plugin
```
apt install protobuf-compiler libprotoc-dev
```


## Contribution
Thank you for your desire to develop Anytype together!

❤️ This project and everyone involved in it is governed by the [Code of Conduct](docs/CODE_OF_CONDUCT.md).

🧑‍💻 Check out our [contributing guide](docs/CONTRIBUTING.md) to learn about asking questions, creating issues, or submitting pull requests.

🫢 For security findings, please email [security@anytype.io](mailto:security@anytype.io) and refer to our [security guide](docs/SECURITY.md) for more information.

🤝 Follow us on [Github](https://github.com/anyproto) and join the [Contributors Community](https://github.com/orgs/anyproto/discussions).

---
Made by Any — a Swiss association 🇨🇭

Licensed under [Any Source Available License 1.0](./LICENSE.md).
