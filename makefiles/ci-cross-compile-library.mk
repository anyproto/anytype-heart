BUILD_TAGS ?= $(BUILD_TAG_NETWORK) nographviz nowatchdog nosigar nomutexdeadlockdetector

cross-compile-library:
	echo $(FLAGS)
	$(MAKE) -j \
		cross-compile-library-darwin-amd64 \
		cross-compile-library-darwin-arm64 \
		cross-compile-library-windows-amd64 \
		cross-compile-library-linux-amd64 \
		cross-compile-library-linux-arm64

cross-compile-library-darwin-amd64:
	echo $(SDKROOT)
	GOOS="darwin" \
		CGO_CFLAGS="-mmacosx-version-min=11" \
		MACOSX_DEPLOYMENT_TARGET=11.0 \
		GOARCH="amd64" \
		CGO_ENABLED="1" \
		go build -tags="$(BUILD_TAGS)" -ldflags="$(FLAGS)" -o darwin-amd64 github.com/anyproto/anytype-heart/cmd/grpcserver

cross-compile-library-darwin-arm64:
	SDKROOT=$(shell xcrun --sdk macosx --show-sdk-path)
	echo $(SDKROOT)
	GOOS="darwin" \
		CGO_CFLAGS="-mmacosx-version-min=11" \
		MACOSX_DEPLOYMENT_TARGET=11.0 \
		GOARCH="arm64" \
		CGO_ENABLED="1" \
		go build -tags="$(BUILD_TAGS)" -ldflags="$(FLAGS)" -o darwin-arm64 github.com/anyproto/anytype-heart/cmd/grpcserver

cross-compile-library-windows-amd64:
	GOOS="windows" \
		GOARCH="amd64" \
		CGO_ENABLED="1" \
		CC="x86_64-w64-mingw32-gcc" \
		CXX="x86_64-w64-mingw32-g++" \
		go build -tags="$(BUILD_TAGS) noheic" -ldflags="$(FLAGS) -linkmode external -extldflags=-static" -o windows-amd64 github.com/anyproto/anytype-heart/cmd/grpcserver

cross-compile-library-linux-amd64:
	GOOS="linux" \
		GOARCH="amd64" \
		CGO_ENABLED="1" \
		CC="x86_64-linux-musl-gcc" \
		go build -tags="$(BUILD_TAGS) noheic" -ldflags="$(FLAGS) -linkmode external -extldflags '-static -Wl,-z stack-size=1000000'" -o linux-amd64 github.com/anyproto/anytype-heart/cmd/grpcserver

cross-compile-library-linux-arm64:
	GOOS="linux" \
		GOARCH="arm64" \
		CGO_ENABLED="1" \
		CC="aarch64-linux-musl-gcc" \
		go build -tags="$(BUILD_TAGS) noheic" -ldflags="$(FLAGS) -linkmode external" -o linux-arm64 github.com/anyproto/anytype-heart/cmd/grpcserver
