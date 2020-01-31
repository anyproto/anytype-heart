### Anytype Middleware Library
[![CircleCI](https://circleci.com/gh/anytypeio/go-anytype-middleware/tree/master.svg?style=svg&circle-token=eb74d38301ec933d25eb6778f662c94b175186ef)](https://circleci.com/gh/anytypeio/go-anytype-middleware/tree/master)

#### How to build

1. Install Golang 1.12.* [from here](http://golang.org/dl/)
2. `make setup` to install deps
3. `make build-lib` to build C(`.so`) library into `dist` folder
4. `make build-js` to build NodeJS Addon into `jsaddon/build`
5. `npm install & npm build:ts` to compile proto files for TS/JS to `build/ts`

#### Rebuild proto files
This repo uses custom protoc plugin located at [anytypeio/protobuf/protoc-gen-gogo/gomobile](https://github.com/anytypeio/protobuf/tree/master/protoc-gen-gogo/gomobile).
So make sure you have installed it:
```
make setup-protoc
```

Then you can easily regenerate proto files:
```
make protos
```


#### Run local gRPC server to debug
⚠️ Make sure to update/install protobuf compiler from [this repo](https://github.com/anytypeio/protobuf) using `make setup-protoc`

Commands:
`make run-debug` - builds proto files for grpc server, builds the binary and runs it
`make build-debug` - builds proto files for grpc server and builds the binary into `dist/debug`

If you want to change the default port(9999):
`ANYTYPE_GRPC_ADDR=127.0.0.1:8888 make run-debug`


Useful tools for debug: 
**GUI**
https://github.com/uw-labs/bloomrpc 
`HowTo: Set the import path to the middleware root, then select commands.proto file`

**CLI** 
https://github.com/njpatel/grpcc

#### Run tests
GO test:
```
make test
```

NodeJS addon test:
```
cd jsaddon
npm run test
```
