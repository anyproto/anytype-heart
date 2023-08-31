# Build instructions
## Build from Source
1. Install Golang 1.20.x [from here](http://golang.org/dl/) or using preferred package manager
2. Follow instructions below for the target systems

### Install local deps

#### Mac
As of 16.01.23 last protobuf version (21.12) broke the JS plugin support, so you can use the v3 branch:
```
brew install protobuf@3
```

To generate Swift protobuf:
```
brew install swift-protobuf
```

#### Debian/Ubuntu
We need to have protoc binary (3.x version) and libprotoc headers in orderto build the grpc-web plugin
```
apt install protobuf-compiler libprotoc-dev
```

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