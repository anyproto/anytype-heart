ROOT:=${PWD}

comma:=,

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

ifeq ($(UNAME_S), Darwin)
    PROTOC_OS = osx
else
    PROTOC_OS = linux
endif

ifeq ($(UNAME_M), x86_64)
    PROTOC_ARCH = x86_64
else
    PROTOC_ARCH = aarch_64
endif

PROTOC_ZIP = protoc-$(PROTOC_VERSION)-$(PROTOC_OS)-$(PROTOC_ARCH).zip

export DEPS=${ROOT}/deps
export PACKAGE_PATH=github.com/anyproto/anytype-heart/pb

PROTOC_VERSION = 29.3

PROTOC_URL = https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP)

PROTOC = $(DEPS)/protoc
PROTOC_GEN_GO = $(DEPS)/protoc-gen-go
PROTOC_GEN_DRPC = $(DEPS)/protoc-gen-go-drpc
PROTOC_GEN_VTPROTO = $(DEPS)/protoc-gen-go-vtproto
PROTOC_INCLUDE := $(DEPS)/include

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

fork:

setup-go: setup-network-config check-tantivy-version
	@echo 'Setting up go modules...'
	@go mod download
	@go install github.com/ahmetb/govvv@v0.2.0

setup-gomobile:
	go build -o deps golang.org/x/mobile/cmd/gomobile
	go build -o deps golang.org/x/mobile/cmd/gobind

VT_PROTOBUF_REPO := $(DEPS)/vtprotobuf
GO_PROTOBUF_REPO := $(DEPS)/protobuf-go

VT_PROTOBUF_COMMIT := 57a97b786bfdef686fce425af0b32376dedac8ce
GO_PROTOBUF_COMMIT := d58efe595bddd808375cd0c4f66dafe33a11d8b0

setup-protoc-go:
	go mod download
	rm -rf $(PROTOC)
	rm -rf $(PROTOC_GEN_GO)
	rm -rf $(PROTOC_GEN_DRPC)
	rm -rf $(PROTOC_GEN_VTPROTO)
	rm -rf $(PROTOC_INCLUDE)
	rm -rf $(GO_PROTOBUF_REPO)
	rm -rf $(VT_PROTOBUF_REPO)
	@echo "Downloading protoc $(PROTOC_VERSION)..."
	curl -OL $(PROTOC_URL)
	mkdir -p $(DEPS)
	unzip -o $(PROTOC_ZIP) -d $(ROOT)
	mv bin/protoc $(DEPS)
	rm $(PROTOC_ZIP)
	mv include deps
	rm -rf readme.txt
	rm -rf bin
	@echo "protoc installed in $(DEPS)/bin"

	GOBIN=$(DEPS) go install storj.io/drpc/cmd/protoc-gen-go-drpc@latest
	GOBIN=$(DEPS) go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@echo "Cloning vtprotobuf fork..."
	git clone https://github.com/anyproto/vtprotobuf.git $(VT_PROTOBUF_REPO) || true
	cd $(VT_PROTOBUF_REPO) && git fetch && git checkout $(VT_PROTOBUF_COMMIT)
	@echo "Building protoc-gen-go-vtproto..."
	@echo "Cloning protoc-gen-go fork..."
	git clone https://github.com/anyproto/protobuf-go.git $(GO_PROTOBUF_REPO) || true
	cd $(GO_PROTOBUF_REPO) && git fetch && git checkout $(GO_PROTOBUF_COMMIT)
	@echo "Building protoc-gen-go..."
	GOBIN=$(DEPS) go install storj.io/drpc/cmd/protoc-gen-go-drpc@latest
	go build -o deps github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc
	cd $(VT_PROTOBUF_REPO)/cmd/protoc-gen-go-vtproto && go build -o $(PROTOC_GEN_VTPROTO)
	cd $(GO_PROTOBUF_REPO)/cmd/protoc-gen-go && go build -o $(PROTOC_GEN_GO)

setup-protoc-jsweb:
	@echo 'Installing grpc-web plugin...'
	@rm -rf deps/grpc-web
	@git clone --depth 1 --branch 1.4.2 http://github.com/grpc/grpc-web deps/grpc-web
	git apply ./clientlibrary/jsaddon/grpcweb_mac.patch
	@[ -d "/opt/homebrew" ] && PREFIX="/opt/homebrew" $(MAKE) -C deps/grpc-web plugin || $(MAKE) -C deps/grpc-web plugin
	mv deps/grpc-web/javascript/net/grpc/web/generator/protoc-gen-grpc-web deps/protoc-gen-grpc-web
	@rm -rf deps/grpc-web

setup-protoc: setup-protoc-go setup-protoc-jsweb