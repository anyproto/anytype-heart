setup: setup-go
	@echo 'Setting up npm...'
	@npm install

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

setup-protoc: setup-protoc-go