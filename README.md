### Proto -> TS generation

```
pbjs -t static-module -w commonjs -o build/ts/event.js event.proto
pbts -o build/ts/event.d.ts build/ts/event.js
```

Additionally, TypeScript definitions of static modules are compatible with their reflection-based counterparts (i.e. as exported by JSON modules), as long as the following conditions are met:

1. Instead of using `new SomeMessage(...)`, always use `SomeMessage.create(...)` because reflection objects do not provide a constructor.
2. Types, services and enums must start with an uppercase letter to become available as properties of the reflected types as well (i.e. to be able to use `MyMessage.MyEnum` instead of `root.lookup("MyMessage.MyEnum"))`.

### Proto -> GO generation

```
protoc -I protocol/ protocol/event.proto --go_out=plugins=grpc:protocol/build/go
```