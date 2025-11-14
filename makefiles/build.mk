# build

build-lib:
	@echo 'Building library...'
	@$(eval FLAGS += $$(shell govvv -flags -pkg github.com/anyproto/anytype-heart/util/vcs))
	@GO111MODULE=on go build -v -o dist/lib.a -tags nogrpcserver -ldflags "$(FLAGS)" -buildmode=c-archive -v ./clientlibrary/clib

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
