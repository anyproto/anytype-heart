# Build instructions
## Build from Source
1. Install Golang 1.24.x [from here](http://golang.org/dl/) or using preferred package manager
2. For JS(desktop client) bindings generation, install [Node.js](https://nodejs.org/), protobuf plugins will be installed automatically
3. Follow instructions below for the target systems

### Install local deps

#### Update tantivy
Update the tantivy version in go.mod and run:
```
make download-tantivy-all-force
```

#### Mac

Install protobuf compiler:

```
brew install protobuf
```

Once installed, verify you have protoc:

```
which protoc
```

The expected version is `protoc` >= 33.0.0.

To generate Swift(iOS) protobuf bindings:
```
brew install swift-protobuf
```

#### Debian/Ubuntu

Install protobuf compiler and headers:
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

### Install custom Go protobuf generators
`make setup-protoc` to install custom Go protobuf generators (see [Protogen](https://github.com/anyproto/anytype-heart/blob/main/docs/Protogen.md) for additional information)

### Build and install for the [desktop client](https://github.com/anyproto/anytype-ts)
`make install-dev-js` to install js protobuf plugins and then build the local server and copy it and protobuf binding into `../anytype-ts`

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
