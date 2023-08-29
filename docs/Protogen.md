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