setup: setup-go

setup-network-config:
ifdef ANYENV
	@echo "ANYENV is now deprecated. Use ANY_SYNC_NETWORK instead."
	@exit 1;
endif
	@if [ -z "$$ANY_SYNC_NETWORK" ]; then \
	echo "Using the default production Any Sync Network"; \
elif [ ! -e "$$ANY_SYNC_NETWORK" ]; then \
	echo "Network configuration file not found at $$ANY_SYNC_NETWORK"; \
	exit 1; \
else \
	echo "Using Any Sync Network configuration at $$ANY_SYNC_NETWORK"; \
	cp $$ANY_SYNC_NETWORK $(CUSTOM_NETWORK_FILE); \
fi

setup-go: setup-network-config check-tantivy-version
	@echo 'Setting up go modules...'
	@go mod download
	@go build -o deps github.com/ahmetb/govvv
	@go build -o deps github.com/awalterschulze/goderive

setup-gomobile:
	go build -o deps golang.org/x/mobile/cmd/gomobile
	go build -o deps golang.org/x/mobile/cmd/gobind

setup-protoc-go:
	@echo 'Setting up protobuf compiler...'
	go build -o deps github.com/gogo/protobuf/protoc-gen-gogofaster
	go build -o deps github.com/gogo/protobuf/protoc-gen-gogofast
	go build -o deps github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc

setup-protoc-js:
	@echo 'Setting up js protobuf plugins...'
	@npm -D install

setup-swag:
	@echo 'Setting up swag...'
	# swag is pinned the way every other build tool here is: deps/deps.go
	# blank-imports github.com/swaggo/swag/v2/cmd/swag behind the `deps` build
	# tag, so `go mod tidy` keeps the module and the command's own
	# requirements, and the require line in go.mod fixes the version. Without
	# that import tidy drops both, since no Go file imports a generator, and
	# the build then resolves the newest swag — which since rc5 emits
	# `BearerAuth` where scripts/fix_openapi_v2.py expects `bearerauth`.
	# If the pin ever goes missing again, restore it with:
	#   go get github.com/swaggo/swag/v2@v2.0.0-rc4 && go mod tidy
	# No -mod=mod here on purpose: with the pin it is unnecessary, and a
	# readonly build fails loudly instead of regenerating the documents with
	# whatever generator the graph happens to resolve.
	@go build -o deps github.com/swaggo/swag/v2/cmd/swag

setup-protoc: setup-protoc-go