# Build instructions
## Build from Source
1. Install Golang 1.22.x [from here](http://golang.org/dl/) or using preferred package manager
2. Follow instructions below for the target systems

### Install local deps

#### Update tantivy
Update the tantivy version in go.mod and run:
```
make download-tantivy-all-force
```

#### Mac

Make sure you install a recent version of protobuf. Some older versions had
plugin compatibility issues
(*as of 16.01.23 last protobuf version (21.12) broke the JS plugin support.*).
[This protobuf-javascript issue](https://github.com/protocolbuffers/protobuf-javascript/issues/127)
tracks the history and workarounds for the compatibility issues.

Luckily, plugins now work when installed as separate packages alongside protobuf:

```
brew install protobuf protoc-gen-js protoc-gen-grpc-web
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

#### Nix

Repository provides `flake.nix` with `devShell` which has all the build dependencies (excluding tentivy-go pre-compiled libraries yet).

```bash
nix develop .
```
> [!TIP]
> It is also convenient to use nix shell with [direnv](https://direnv.net/) with [nix-direnv](https://github.com/nix-community/nix-direnv),
> which enables `devShell` when you `cd` into directory.
>
> With direnv, you can also switch environments per project in your code editor: [emacs-direnv](https://github.com/wbolster/emacs-direnv)

### Install custom protoc
`make setup-protoc` to install grpc-web plugin (see [Protogen](https://github.com/anyproto/anytype-heart/blob/main/docs/Protogen.md) for additional information)

### Build and install for the [desktop client](https://github.com/anyproto/anytype-ts)
`make install-dev-js` to build the local server and copy it and protobuf binding into `../anytype-ts`

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
