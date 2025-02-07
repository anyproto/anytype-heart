install-brew-and-node-deps:
	curl https://raw.githubusercontent.com/Homebrew/homebrew-core/31b24d65a7210ea0a5689d5ad00dd8d1bf5211db/Formula/protobuf.rb --output protobuf.rb
	curl https://raw.githubusercontent.com/Homebrew/homebrew-core/d600b1f7119f6e6a4e97fb83233b313b0468b7e4/Formula/s/swift-protobuf.rb --output swift-protobuf.rb
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install ./protobuf.rb
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install --ignore-dependencies ./swift-protobuf.rb
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install mingw-w64
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install grpcurl
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew tap messense/macos-cross-toolchains && brew install x86_64-unknown-linux-musl && brew install aarch64-unknown-linux-musl
	npm i -g node-gyp

BUILD_TAGS ?= $(BUILD_TAG_NETWORK) nographviz nowatchdog nosigar nomutexdeadlockdetector
OUTPUT_DIR ?= .release

cross-compile-library:
	echo $(FLAGS)
	$(MAKE) -j \
		cross-compile-library-darwin-amd64 \
		cross-compile-library-darwin-arm64 \
		cross-compile-library-windows-amd64 \
		cross-compile-library-linux-amd64 \
		cross-compile-library-linux-arm64

$(OUTPUT_DIR):
	mkdir -p $(OUTPUT_DIR)

cross-compile-library-darwin-amd64: $(OUTPUT_DIR)
	echo $(SDKROOT)
	GOOS="darwin" \
		CGO_CFLAGS="-mmacosx-version-min=11" \
		MACOSX_DEPLOYMENT_TARGET=11.0 \
		GOARCH="amd64" \
		CGO_ENABLED="1" \
		go build -tags="$(BUILD_TAGS)" -ldflags="$(FLAGS)" -o darwin-amd64 github.com/anyproto/anytype-heart/cmd/grpcserver

cross-compile-library-darwin-arm64: $(OUTPUT_DIR)
	SDKROOT=$(shell xcrun --sdk macosx --show-sdk-path)
	echo $(SDKROOT)
	GOOS="darwin" \
		CGO_CFLAGS="-mmacosx-version-min=11" \
		MACOSX_DEPLOYMENT_TARGET=11.0 \
		GOARCH="arm64" \
		CGO_ENABLED="1" \
		go build -tags="$(BUILD_TAGS)" -ldflags="$(FLAGS)" -o darwin-arm64 github.com/anyproto/anytype-heart/cmd/grpcserver

cross-compile-library-windows-amd64: $(OUTPUT_DIR)
	GOOS="windows" \
		GOARCH="amd64" \
		CGO_ENABLED="1" \
		CC="x86_64-w64-mingw32-gcc" \
		CXX="x86_64-w64-mingw32-g++" \
		go build -tags="$(BUILD_TAGS) noheic" -ldflags="$(FLAGS) -linkmode external -extldflags=-static" -o windows-amd64 github.com/anyproto/anytype-heart/cmd/grpcserver

cross-compile-library-linux-amd64: $(OUTPUT_DIR)
	GOOS="linux" \
		GOARCH="amd64" \
		CGO_ENABLED="1" \
		CC="x86_64-linux-musl-gcc" \
		go build -tags="$(BUILD_TAGS) noheic" -ldflags="$(FLAGS) -linkmode external -extldflags '-static -Wl,-z stack-size=1000000'" -o linux-amd64 github.com/anyproto/anytype-heart/cmd/grpcserver

cross-compile-library-linux-arm64: $(OUTPUT_DIR)
	GOOS="linux" \
		GOARCH="arm64" \
		CGO_ENABLED="1" \
		CC="aarch64-linux-musl-gcc" \
		go build -tags="$(BUILD_TAGS) noheic" -ldflags="$(FLAGS) -linkmode external" -o linux-arm64 github.com/anyproto/anytype-heart/cmd/grpcserver
