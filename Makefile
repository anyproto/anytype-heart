CUSTOM_NETWORK_FILE ?= ./core/anytype/config/nodes/custom.yml
CLIENT_DESKTOP_PATH ?= ../anytype-ts
CLIENT_ANDROID_PATH ?= ../anytype-kotlin
CLIENT_IOS_PATH ?= ../anytype-swift
BUILD_FLAGS ?=

export GOLANGCI_LINT_VERSION=v1.54.2
export CGO_CFLAGS=-Wno-deprecated-non-prototype -Wno-unknown-warning-option -Wno-deprecated-declarations -Wno-xor-used-as-pow

ifndef $(GOPATH)
    GOPATH=$(shell go env GOPATH)
    export GOPATH
endif

ifndef $(GOROOT)
    GOROOT=$(shell go env GOROOT)
    export GOROOT
endif

DEPS_PATH := $(shell pwd)/deps
export PATH := $(DEPS_PATH):$(PATH)

$(shell git config core.hooksPath .githooks)

all:
	@set -e;
.PHONY :

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

setup-go: setup-network-config
	@echo 'Setting up go modules...'
	@go mod download
	@go install github.com/ahmetb/govvv@v0.2.0

fmt:
	@echo 'Formatting with prettier...'
	@npx prettier --write "./**" 2> /dev/null || true
	@echo 'Formatting with goimports...'
	@goimports -w -l `find . -type f -name '*.go' -not -path './vendor/*'`

lint:
	@echo 'Linting with prettier...'
	@npx prettier --check "./**" 2> /dev/null || true
	@echo 'Linting with golint...'
	@golint `go list ./... | grep -v /vendor/`

test:
	@echo 'Running tests...'
	@ANYTYPE_LOG_NOGELF=1 go test -cover github.com/anyproto/anytype-heart/...

test-integration:
	@echo 'Running integration tests...'
	@go test -run=TestBasic -tags=integration -v -count 1 ./tests

test-race:
	@echo 'Running tests with race-detector...'
	@ANYTYPE_LOG_NOGELF=1 go test -race github.com/anyproto/anytype-heart/...

test-deps:
	@echo 'Generating test mocks...'
	@go build -o deps go.uber.org/mock/mockgen
	@go build -o deps github.com/vektra/mockery/v2
	@go generate ./...
	@$(DEPS_PATH)/mockery --disable-version-string

clear-test-deps:
	@echo 'Removing test mocks...'
	@find . -name "*_mock.go" | xargs -r rm -v

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


build-ios: setup-go setup-gomobile
	# PATH is not working here, so we need to use absolute path
	$(DEPS_PATH)/gomobile init
	@echo 'Clear xcframework'
	@rm -rf ./dist/ios/Lib.xcframework
	@echo 'Building library for iOS...'
	@$(eval FLAGS += $$(shell govvv -flags | sed 's/main/github.com\/anyproto\/anytype-heart\/util\/vcs/g'))
	@$(eval TAGS := nogrpcserver gomobile nowatchdog nosigar timetzdata)
ifdef ANY_SYNC_NETWORK
	@$(eval TAGS := $(TAGS) envnetworkcustom)
endif
	gomobile bind -tags "$(TAGS)" -ldflags "$(FLAGS)" $(BUILD_FLAGS) -target=ios -o Lib.xcframework github.com/anyproto/anytype-heart/clientlibrary/service github.com/anyproto/anytype-heart/core
	@mkdir -p dist/ios/ && mv Lib.xcframework dist/ios/
	@mkdir -p dist/ios/json/
	@cp pkg/lib/bundle/system*.json dist/ios/json/
	@cp pkg/lib/bundle/relations.json dist/ios/json/
	@cp pkg/lib/bundle/internal*.json dist/ios/json/
	@go mod tidy

install-dev-ios: setup-go build-ios protos-swift
	@echo 'Installing iOS framework locally at $(CLIENT_IOS_PATH)...'
	@rm -rf $(CLIENT_IOS_PATH)/Dependencies/Middleware/*
	@cp -r dist/ios/Lib.xcframework $(CLIENT_IOS_PATH)/Dependencies/Middleware
	@rm -rf $(CLIENT_IOS_PATH)/Modules/ProtobufMessages/Sources/Protocol/*
	@cp -r dist/ios/protobuf/*.swift $(CLIENT_IOS_PATH)/Modules/ProtobufMessages/Sources/Protocol
	@mkdir -p $(CLIENT_IOS_PATH)/Dependencies/Middleware/protobuf/protos
	@cp -r pb/protos/*.proto $(CLIENT_IOS_PATH)/Dependencies/Middleware/protobuf/protos
	@cp -r pb/protos/service/*.proto $(CLIENT_IOS_PATH)/Dependencies/Middleware/protobuf/protos
	@cp -r pkg/lib/pb/model/protos/*.proto $(CLIENT_IOS_PATH)/Dependencies/Middleware/protobuf/protos
	@mkdir -p $(CLIENT_IOS_PATH)/Dependencies/Middleware/json
	@cp -r pkg/lib/bundle/*.json $(CLIENT_IOS_PATH)/Dependencies/Middleware/json

build-android: setup-go setup-gomobile
	$(DEPS_PATH)/gomobile init
	@echo 'Building library for Android...'
	@$(eval FLAGS += $$(shell govvv -flags | sed 's/main/github.com\/anyproto\/anytype-heart\/util\/vcs/g'))
	@$(eval TAGS := nogrpcserver gomobile nowatchdog nosigar timetzdata)
ifdef ANY_SYNC_NETWORK
	@$(eval TAGS := $(TAGS) envnetworkcustom)
endif
	gomobile bind -tags "$(TAGS)" -ldflags "$(FLAGS)" $(BUILD_FLAGS) -target=android -androidapi 19 -o lib.aar github.com/anyproto/anytype-heart/clientlibrary/service github.com/anyproto/anytype-heart/core
	@mkdir -p dist/android/ && mv lib.aar dist/android/
	@go mod tidy

install-dev-android: setup-go build-android
	@echo 'Installing android lib locally in $(CLIENT_ANDROID_PATH)...'
	@rm -f $(CLIENT_ANDROID_PATH)/libs/lib.aar
	@cp -r dist/android/lib.aar $(CLIENT_ANDROID_PATH)/libs/lib.aar
	@cp -r pb/protos/*.proto $(CLIENT_ANDROID_PATH)/protocol/src/main/proto
	@cp -r pkg/lib/pb/model/protos/*.proto $(CLIENT_ANDROID_PATH)/protocol/src/main/proto
	# Compute the SHA hash of lib.aar
	@$(eval hash := $$(shell shasum -b dist/android/lib.aar | cut -d' ' -f1))
	@echo "Version hash: ${hash}"
	# Update the gradle file with the new version
	@sed -i '' "s/version = '.*'/version = '${hash}'/g" $(CLIENT_ANDROID_PATH)/libs/build.gradle
	@cat $(CLIENT_ANDROID_PATH)/libs/build.gradle

	@sed -i '' "s/middlewareVersion = \".*\"/middlewareVersion = \"${hash}\"/" $(CLIENT_ANDROID_PATH)/gradle/libs.versions.toml
	@cat $(CLIENT_ANDROID_PATH)/gradle/libs.versions.toml

	# Print the updated gradle file (for verification)
	@cd $(CLIENT_ANDROID_PATH) && make setup_local_mw
	@cd $(CLIENT_ANDROID_PATH) && make normalize_mw_imports

setup-gomobile:
	go build -o deps golang.org/x/mobile/cmd/gomobile
	go build -o deps golang.org/x/mobile/cmd/gobind

setup-protoc-go:
	@echo 'Setting up protobuf compiler...'
	go build -o deps github.com/gogo/protobuf/protoc-gen-gogofaster
	go build -o deps github.com/gogo/protobuf/protoc-gen-gogofast
	go build -o deps github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc


setup-protoc-jsweb:
	@echo 'Installing grpc-web plugin...'
	@rm -rf deps/grpc-web
	@git clone --depth 1 --branch 1.4.2 http://github.com/grpc/grpc-web deps/grpc-web
	git apply ./clientlibrary/jsaddon/grpcweb_mac.patch
	@[ -d "/opt/homebrew" ] && PREFIX="/opt/homebrew" $(MAKE) -C deps/grpc-web plugin || $(MAKE) -C deps/grpc-web plugin
	mv deps/grpc-web/javascript/net/grpc/web/generator/protoc-gen-grpc-web deps/protoc-gen-grpc-web
	@rm -rf deps/grpc-web

setup-protoc: setup-protoc-go setup-protoc-jsweb

protos-server:
	@echo 'Generating protobuf packages for lib-server (Go)...'
	@$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	@$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	@$(eval P_DESCRIPTOR := Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor)
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_DESCRIPTOR))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 GOGO_GRPC_SERVER_METHOD_NO_ERROR=1 PACKAGE_PATH=github.com/anyproto/anytype-heart/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=grpc:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./pb/service/

protos-go:
	@echo 'Generating protobuf packages for lib (Go)...'
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	@$(eval P_DESCRIPTOR := Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor)
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpkg/lib/pb/model/protos/localstore.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)

	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_DESCRIPTOR))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pkg/lib/pb/ pkg/lib/pb/model/protos/*.proto; mv pkg/lib/pb/pkg/lib/pb/model/*.go pkg/lib/pb/model/; rm -rf pkg/lib/pb/pkg
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pkg/lib/pb/ pkg/lib/pb/storage/protos/*.proto; mv pkg/lib/pb/pkg/lib/pb/storage/*.go pkg/lib/pb/storage/; rm -rf pkg/lib/pb/pkg
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP),plugins=grpc:./ pkg/lib/cafe/pb/*.proto
	@echo 'Generating protobuf packages for mw (Go)...'
	@$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	@$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_DESCRIPTOR))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc -I . --gogofaster_out=$(PKGMAP):. ./pb/protos/*.proto; mv ./pb/protos/*.pb.go ./pb/
	@$(eval P_PROTOS4 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS5 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_PROTOS4),$$(P_PROTOS5),$$(P_DESCRIPTOR))
	@GOGO_NO_UNDERSCORE=1 GOGO_GOMOBILE_WITH_CONTEXT=1 GOGO_EXPORT_ONEOF_INTERFACE=1 PACKAGE_PATH=github.com/anyproto/anytype-heart/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./clientlibrary/service/
	@protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos-gomobile:
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 GOGO_GOMOBILE_WITH_CONTEXT=1 PACKAGE_PATH=github.com/anyproto/anytype-heart/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./clientlibrary/service/

protos-docs:
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	@protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos: protos-go protos-server protos-docs

protos-swift:
	@echo 'Clear protobuf files'
	@rm -rf ./dist/ios/protobuf/*
	@echo 'Generating swift protobuf files'
	@protoc -I ./  --swift_opt=FileNaming=DropPath --swift_opt=Visibility=Public --swift_out=./dist/ios/protobuf pb/protos/*.proto pkg/lib/pb/model/protos/*.proto
		@echo 'Generated swift protobuf files at ./dist/ios/pb'
	
protos-swift-local: protos-swift
	@echo 'Clear proto files'
	@rm -rf ./dist/ios/protobuf/protos
	@echo 'Copying proto files'
	@mkdir ./dist/ios/protobuf/protos
	@cp ./pb/protos/*.proto ./dist/ios/protobuf/protos
	@cp ./pb/protos/service/*.proto ./dist/ios/protobuf/protos
	@cp ./pkg/lib/pb/model/protos/*.proto ./dist/ios/protobuf/protos
	@open ./dist

protos-js:
	@echo 'Generating protobuf packages (JS)...'
	@protoc -I ./  --js_out=import_style=commonjs,binary:./dist/js/pb pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto
	@protoc -I ./  --grpc-web_out=import_style=commonjs+dts,mode=grpcwebtext:./dist/js/pb pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos-java:
	@echo 'Generating protobuf packages (Java)...'
	@protoc -I ./ --java_out=./dist/android/pb pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

build-server: setup-network-config
	@echo 'Building anytype-heart server...'
	@$(eval FLAGS += $$(shell govvv -flags -pkg github.com/anyproto/anytype-heart/util/vcs))
	@$(eval TAGS := nosigar nowatchdog)
ifdef ANY_SYNC_NETWORK
	@$(eval TAGS := $(TAGS) envnetworkcustom)
endif
	go build -o dist/server -ldflags "$(FLAGS)" --tags "$(TAGS)" $(BUILD_FLAGS) github.com/anyproto/anytype-heart/cmd/grpcserver

run-server: build-server
	@echo 'Running server...'
	@./dist/server

install-dev-js-addon: setup build-lib build-js-addon protos-js
	@echo 'Installing JS-addon (dev-mode) in ${CLIENT_DESKTOP_PATH}...'
	@rm -rf $(CLIENT_DESKTOP_PATH)/build
	@cp -r clientlibrary/jsaddon/build $(CLIENT_DESKTOP_PATH)/
	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/dist/lib

install-dev-js: setup-go build-server protos-js
	@echo 'Installing JS-server (dev-mode) in $(CLIENT_DESKTOP_PATH)...'
	@rm -f $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper

ifeq ($(OS),Windows_NT)
	@cp -r dist/server $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper.exe
else
	@cp -r dist/server $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper
endif

	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/dist/lib
	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/dist/lib
	@mkdir -p $(CLIENT_DESKTOP_PATH)/dist/lib/json/generated
	@cp pkg/lib/bundle/system*.json $(CLIENT_DESKTOP_PATH)/dist/lib/json/generated
	@cp pkg/lib/bundle/internal*.json $(CLIENT_DESKTOP_PATH)/dist/lib/json/generated

build-js: setup-go build-server protos-js
	@echo "Run 'make install-dev-js' instead if you want to build & install into $(CLIENT_DESKTOP_PATH)"

install-linter:
	@go install github.com/daixiang0/gci@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

run-linter:
ifdef GOLANGCI_LINT_BRANCH
	@golangci-lint run -v ./... --new-from-rev=$(GOLANGCI_LINT_BRANCH) --skip-files ".*_test.go" --skip-files "testMock/*" --timeout 15m --verbose
else 
	@golangci-lint run -v ./... --new-from-rev=origin/main --skip-files ".*_test.go" --skip-files "testMock/*" --timeout 15m --verbose
endif

run-linter-fix:
ifdef GOLANGCI_LINT_BRANCH
	@golangci-lint run -v ./... --new-from-rev=$(GOLANGCI_LINT_BRANCH) --skip-files ".*_test.go" --skip-files "testMock/*" --timeout 15m --fix
else 
	@golangci-lint run -v ./... --new-from-rev=origin/main --skip-files ".*_test.go" --skip-files "testMock/*" --timeout 15m --fix
endif
