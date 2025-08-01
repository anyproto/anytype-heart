# build

build-lib:
	@echo 'Building library...'
	@$(eval FLAGS += $$(shell govvv -flags -pkg github.com/anyproto/anytype-heart/util/vcs))
	@GO111MODULE=on go build -v -o dist/lib.a -tags nogrpcserver -ldflags "$(FLAGS)" -buildmode=c-archive -v ./clientlibrary/clib

build-js-addon:
	@echo 'Building JS-addon...'
	@cp dist/lib.a clientlibrary/jsaddon/lib.a
	@cp dist/lib.h clientlibrary/jsaddon/lib.h
	@cp clientlibrary/clib/bridge.h clientlibrary/jsaddon/bridge.h
	# Electron's version.
	@export npm_config_target=12.0.4
	@export npm_config_arch=x64
	@export npm_config_target_arch=x64
	# The architecture of Electron, see https://electronjs.org/docs/tutorial/support#supported-platforms
	# for supported architectures.
	# Download headers for Electron.
	@export npm_config_disturl=https://electronjs.org/headers
	# Tell node-pre-gyp that we are building for Electron.
	@export npm_config_runtime=electron
	# Tell node-pre-gyp to build module from source code.
	@export npm_config_build_from_source=true
	@npm install -C ./clientlibrary/jsaddon
	@rm clientlibrary/jsaddon/lib.a clientlibrary/jsaddon/lib.h clientlibrary/jsaddon/bridge.h


build-server: setup-network-config check-tantivy-version
	@echo 'Building anytype-heart server...'
	@$(eval FLAGS += $$(shell govvv -flags -pkg github.com/anyproto/anytype-heart/util/vcs))
	@$(eval TAGS := $(TAGS) nosigar nowatchdog)
ifdef ANY_SYNC_NETWORK
	@$(eval TAGS := $(TAGS) envnetworkcustom)
endif
	go build -o dist/server -ldflags "$(FLAGS)" --tags "$(TAGS)" $(BUILD_FLAGS) -v github.com/anyproto/anytype-heart/cmd/grpcserver

build-js: setup-go build-server protos-js
	@echo "Run 'make install-dev-js' instead if you want to build & install into $(CLIENT_DESKTOP_PATH)"
