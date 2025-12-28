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

setup-protoc-base:
	 @echo "Checking protoc installation..."
	 @if ! command -v protoc >/dev/null 2>&1; then \
		echo "Installing protoc..."; \
		case "$$(uname -s)" in \
			Linux) \
				if command -v dnf >/dev/null 2>&1; then \
					sudo dnf install -y protobuf-compiler protobuf-devel && \
					PROTOC_ZIP=protoc-3.19.4-linux-x86_64.zip && \
					curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.19.4/$$PROTOC_ZIP && \
					sudo unzip -o $$PROTOC_ZIP -d /usr/local bin/protoc && \
					sudo unzip -o $$PROTOC_ZIP -d /usr/local 'include/*' && \
					rm -f $$PROTOC_ZIP; \
				else \
					sudo apt-get update && \
					sudo apt-get install -y protobuf-compiler libprotobuf-dev libprotoc-dev && \
					PROTOC_ZIP=protoc-3.19.4-linux-x86_64.zip && \
					curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.19.4/$$PROTOC_ZIP && \
					sudo unzip -o $$PROTOC_ZIP -d /usr/local bin/protoc && \
					sudo unzip -o $$PROTOC_ZIP -d /usr/local 'include/*' && \
					rm -f $$PROTOC_ZIP; \
				fi ;; \
			Darwin) \
				brew install protobuf && \
				brew install protobuf-c ;; \
			*) \
				echo "Unsupported platform" && exit 1 ;; \
			esac \
	fi

setup-protoc-deps:
	 @echo "Installing Go protoc plugins..."
	 @go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
	 @go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
	 @go install github.com/gogo/protobuf/protoc-gen-gogofaster@latest
	 @go install github.com/gogo/protobuf/protoc-gen-gogofast@latest
	 @go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@latest

setup-protoc-go: setup-protoc-deps
	 @echo 'Setting up protobuf compiler...'
	 go build -o deps github.com/gogo/protobuf/protoc-gen-gogofaster
	 go build -o deps github.com/gogo/protobuf/protoc-gen-gogofast
	 go build -o deps github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc

setup-protoc-jsweb:
	 @echo "Installing grpc-web plugin dependencies..."
	 @case "$$(uname -s)" in \
		Linux) \
			if command -v dnf >/dev/null 2>&1; then \
				sudo dnf install -y gcc-c++ protobuf-devel make glibc-static libstdc++-static; \
			else \
				sudo apt-get install -y build-essential libprotoc-dev libprotobuf-dev; \
			fi ;; \
		Darwin) \
			brew install protobuf-c && \
			brew install gcc ;; \
		esac
	 @echo 'Installing grpc-web plugin...'
	 @rm -rf deps/grpc-web
	 @git clone --depth 1 --branch 1.4.2 http://github.com/grpc/grpc-web deps/grpc-web
	 @if [ -f "./clientlibrary/jsaddon/grpcweb_mac.patch" ]; then \
		cd deps/grpc-web && git apply ../../clientlibrary/jsaddon/grpcweb_mac.patch || echo "Warning: patch application failed, continuing anyway"; \
	fi
	 @echo "Building grpc-web plugin from source..."
	 @mkdir -p deps/grpc-web/javascript/net/grpc/web/generator
	 @echo "CXX = g++" > deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @echo "CPPFLAGS += -I/usr/local/include" >> deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @echo "CXXFLAGS += -std=c++11 -pthread" >> deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @echo "LDFLAGS += -L/usr/local/lib" >> deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @echo "" >> deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @echo "protoc-gen-grpc-web: grpc_generator.o" >> deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @echo "	$$(CXX) $$(LDFLAGS) -o $$@ $^ $$(LDLIBS)" >> deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @echo "" >> deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @echo "grpc_generator.o: grpc_generator.cc" >> deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @echo "	$$(CXX) $$(CPPFLAGS) $$(CXXFLAGS) -c -o $$@ $$<" >> deps/grpc-web/javascript/net/grpc/web/generator/Makefile
	 @cd deps/grpc-web/javascript/net/grpc/web/generator && $(MAKE)
	 @mv deps/grpc-web/javascript/net/grpc/web/generator/protoc-gen-grpc-web deps/protoc-gen-grpc-web
	 @rm -rf deps/grpc-web

setup-protoc-js:
	@echo 'Setting up js protobuf plugins...'
	@npm -D install

setup-swag:
	@echo 'Setting up swag...'
	# -mod=mod allows go to auto-add swag's transitive deps to go.sum (they get stripped by go mod tidy since the main module doesn't import them directly)
	@GOFLAGS=-mod=mod go build -o deps github.com/swaggo/swag/v2/cmd/swag

setup-protoc: setup-swag setup-protoc-base setup-protoc-go setup-protoc-jsweb setup-protoc-js
