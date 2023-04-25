export GOPRIVATE=github.com/anytypeio
export GOLANGCI_LINT_VERSION=v1.49.0
LOCALNODE ?= false

ifndef $(GOPATH)
    GOPATH=$(shell go env GOPATH)
    export GOPATH
endif

ifndef $(GOROOT)
    GOROOT=$(shell go env GOROOT)
    export GOROOT
endif

export PATH:=deps:$(GOPATH)/bin:$(PATH)

all:
	@set -e;
	@git config core.hooksPath .githooks;
.PHONY :

setup: setup-go
	@echo 'Setting up npm...'
	@npm install


setup-go:
	@echo 'Setting up go modules...'
	@go mod download
	@GO111MODULE=off go install github.com/ahmetb/govvv
	go install golang.org/x/mobile/cmd/gomobile@latest
	go install golang.org/x/mobile/cmd/gobind@latest

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
	@ANYTYPE_LOG_NOGELF=1 go test -cover github.com/anytypeio/go-anytype-middleware/...

test-integration:
	@echo 'Running integration tests...'
	@go test -tags=integration -v -count 1 ./tests

test-race:
	@echo 'Running tests with race-detector...'
	@ANYTYPE_LOG_NOGELF=1 go test -race github.com/anytypeio/go-anytype-middleware/...

test-deps:
	@echo 'Generating test mocks...'
	@go install github.com/golang/mock/mockgen
	@go generate ./...

clear-test-deps:
	@echo 'Removing test mocks...'
	@find util/testMock -name "*_mock.go" | xargs -r rm -v

build-lib:
	@echo 'Building library...'
	@$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/util/vcs))
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

build-ios: setup-go
	gomobile init
	@go get golang.org/x/mobile/bind
	@echo 'Clear xcframework'
	@rm -rf ./dist/ios/Lib.xcframework
	@echo 'Building library for iOS...'
	@$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/anytypeio\/go-anytype-middleware\/util\/vcs/g'))
	gomobile bind -tags "nogrpcserver gomobile nowatchdog nosigar" -ldflags "$(FLAGS)" -v -target=ios -o Lib.xcframework github.com/anytypeio/go-anytype-middleware/clientlibrary/service github.com/anytypeio/go-anytype-middleware/core
	@mkdir -p dist/ios/ && mv Lib.xcframework dist/ios/
	@go mod tidy

build-android: setup-go
	gomobile init
	@go get golang.org/x/mobile/bind
	@echo 'Building library for Android...'
	@$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/anytypeio\/go-anytype-middleware\/util\/vcs/g'))
	@$(eval TAGS := nogrpcserver gomobile nowatchdog nosigar)
ifeq ($(LOCALNODE), true)
	$(eval TAGS := $(TAGS) localnode)
endif
	gomobile bind -tags "$(TAGS)" -ldflags "$(FLAGS)" -v -target=android -androidapi 19 -o lib.aar github.com/anytypeio/go-anytype-middleware/clientlibrary/service github.com/anytypeio/go-anytype-middleware/core
	@mkdir -p dist/android/ && mv lib.aar dist/android/
	@go mod tidy

setup-protoc-go:
	@echo 'Setting up protobuf compiler...'
	go build -o deps/ github.com/gogo/protobuf/protoc-gen-gogofaster
	go build -o deps/ github.com/gogo/protobuf/protoc-gen-gogofast
	go build -o deps/ github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc
	#go build -o deps/ github.com/gogo/protobuf/protoc-gen-gogo/gomobile


setup-protoc-jsweb:
	@echo 'Installing grpc-web plugin...'
	@rm -rf grpc-web
	@git clone http://github.com/grpc/grpc-web
	git apply ./clientlibrary/jsaddon/grpcweb_mac.patch
	@[ -d "/opt/homebrew" ] && PREFIX="/opt/homebrew" $(MAKE) -C grpc-web install-plugin || $(MAKE) -C grpc-web install-plugin
	@rm -rf grpc-web

setup-protoc: setup-protoc-go setup-protoc-jsweb

protos-server:
	@echo 'Generating protobuf packages for lib-server (Go)...'
	@$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	@$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	@$(eval P_DESCRIPTOR := Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor)
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_DESCRIPTOR))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 GOGO_GRPC_SERVER_METHOD_NO_ERROR=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=grpc:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./pb/service/

protos-go:
	@echo 'Generating protobuf packages for lib (Go)...'
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	@$(eval P_DESCRIPTOR := Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor)
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpkg/lib/pb/model/protos/localstore.proto=github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model)

	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_DESCRIPTOR))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pkg/lib/pb/ pkg/lib/pb/model/protos/*.proto; mv pkg/lib/pb/pkg/lib/pb/model/*.go pkg/lib/pb/model/; rm -rf pkg/lib/pb/pkg
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pkg/lib/pb/ pkg/lib/pb/storage/protos/*.proto; mv pkg/lib/pb/pkg/lib/pb/storage/*.go pkg/lib/pb/storage/; rm -rf pkg/lib/pb/pkg
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP),plugins=grpc:./ pkg/lib/cafe/pb/*.proto
	@echo 'Generating protobuf packages for mw (Go)...'
	@$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	@$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_DESCRIPTOR))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc -I . --gogofaster_out=$(PKGMAP):. ./pb/protos/*.proto; mv ./pb/protos/*.pb.go ./pb/
	@$(eval P_PROTOS4 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval P_PROTOS5 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_PROTOS4),$$(P_PROTOS5),$$(P_DESCRIPTOR))
	@GOGO_NO_UNDERSCORE=1 GOGO_GOMOBILE_WITH_CONTEXT=1 GOGO_EXPORT_ONEOF_INTERFACE=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./clientlibrary/service/
	@protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos-gomobile:
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 GOGO_GOMOBILE_WITH_CONTEXT=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./clientlibrary/service/

protos-docs:
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
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

build-cli:
	@echo 'Building middleware cli...'
	@$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/util/vcs))
	@go build -v -o dist/cli -ldflags "$(FLAGS)" github.com/anytypeio/go-anytype-middleware/cmd/cli

build-server:
	@echo 'Building middleware server...'
	@$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/util/vcs))
	@$(eval TAGS := nosigar nowatchdog)
ifeq ($(LOCALNODE), true)
	$(eval TAGS := $(TAGS) localnode)
endif
	go build -v -o dist/server -ldflags "$(FLAGS)" --tags "$(TAGS)" github.com/anytypeio/go-anytype-middleware/cmd/grpcserver

build-server-debug: protos-server
	@echo 'Building middleware server with debug symbols...'
	@$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/util/vcs))
	@go build -v -o dist/server -gcflags "all=-N -l" -ldflags "$(FLAGS)" github.com/anytypeio/go-anytype-middleware/cmd/grpcserver

run-server: build-server
	@echo 'Running server...'
	@./dist/server

install-dev-js-addon: setup build-lib build-js-addon protos-js
	@echo 'Installing JS-addon (dev-mode)...'
	@rm -rf ../js-anytype/build
	@cp -r clientlibrary/jsaddon/build ../js-anytype/
	@cp -r dist/js/pb/* ../js-anytype/dist/lib

install-dev-js: build-js
	@echo 'Installing JS-server (dev-mode)...'
	@rm -f ../js-anytype/dist/anytypeHelper

ifeq ($(OS),Windows_NT)
	@cp -r dist/server ../js-anytype/dist/anytypeHelper.exe
else
	@cp -r dist/server ../js-anytype/dist/anytypeHelper
endif

	@cp -r dist/js/pb/* ../js-anytype/dist/lib
	@cp -r dist/js/pb/* ../js-anytype/dist/lib
	@mkdir -p ../js-anytype/dist/lib/json/
	@cp pkg/lib/bundle/system*.json ../js-anytype/dist/lib/json/
	@cp pkg/lib/bundle/internal*.json ../js-anytype/dist/lib/json/

build-js: setup-go build-server protos-js
	@echo "Run 'make install-dev-js' instead if you want to build&install into ../js-anytype"

install-linter:
	@go install github.com/daixiang0/gci@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

run-linter:
ifdef GOLANGCI_LINT_BRANCH
	@golangci-lint run -v ./... --new-from-rev=$(GOLANGCI_LINT_BRANCH) --skip-files ".*_test.go" --timeout 15m --skip-files ".*_test.go"
else 
	@golangci-lint run -v ./... --new-from-rev=master --skip-files ".*_test.go" --timeout 15m --skip-files ".*_test.go"
endif

run-linter-fix:
ifdef GOLANGCI_LINT_BRANCH
	@golangci-lint run -v ./... --new-from-rev=$(GOLANGCI_LINT_BRANCH) --skip-files ".*_test.go" --timeout 15m --fix --skip-files ".*_test.go"
else 
	@golangci-lint run -v ./... --new-from-rev=master --skip-files ".*_test.go" --timeout 15m --fix --skip-files ".*_test.go"
endif

version = latest
docker-run:
	@docker run --interactive --tty --rm --publish 31007:31007 --publish 31008:31008 ghcr.io/anytypeio/go-anytype-middleware:$(version)
