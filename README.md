### Anytype Middleware Library
[![CircleCI](https://circleci.com/gh/anytypeio/go-anytype-middleware/tree/master.svg?style=svg&circle-token=eb74d38301ec933d25eb6778f662c94b175186ef)](https://circleci.com/gh/anytypeio/go-anytype-middleware/tree/master)

### Build from Source
1. Install Golang 1.13.* [from here](http://golang.org/dl/)
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
This repo uses custom protoc located at [anytypeio/protobuf](https://github.com/anytypeio/protobuf/tree/master/protoc-gen-gogo). It adds `gomobile` plugin and some env-controlled options to control the generated code style.

This protobuf generator will replace your `protoc` binary, BTW it doesn't have any breaking changes for other protobuf and grpc code

You can install it with a simple command:
```
make setup-protoc
```

Then you can easily regenerate proto files:
```
make protos
```

### Run tests
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
Useful tools for debug:

**GUI**

https://github.com/uw-labs/bloomrpc

`HowTo: Set the import path to the middleware root, then select commands.proto file`

**CLI**

https://github.com/njpatel/grpcc
