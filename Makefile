export GOPRIVATE=github.com/anytypeio

ifndef $(GOPATH)
    GOPATH=$(shell go env GOPATH)
    export GOPATH
endif

ifndef $(GOROOT)
    GOROOT=$(shell go env GOROOT)
    export GOROOT
endif

export PATH=$(GOPATH)/bin:$(shell echo $$PATH)

all:
	@set -e;
.PHONY :

setup: setup-go
	@echo 'Setting up npm...'
	@npm install

uuu:
	@echo $(PATH)

setup-go:
	@echo 'Setting up go modules...'
	@go mod download
	@GO111MODULE=off go get github.com/ahmetb/govvv
	@GO111MODULE=off go get golang.org/x/mobile/cmd/...

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
	@go test -cover github.com/anytypeio/go-anytype-middleware/...

test-race:
	@echo 'Running tests with race-detector...'
	@go test -race github.com/anytypeio/go-anytype-middleware/...

test-deps:
	@echo 'Generating test mocks...'
	@go install github.com/golang/mock/mockgen
	@go generate ./...

clear-test-deps:
	@echo 'Removing test mocks...'
	@find util/testMock -name "*_mock.go" | xargs -r rm -v

build-lib:
	@echo 'Building library...'
	@$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/core))
	@GO111MODULE=on go build -v -o dist/lib.a -tags nogrpcserver -ldflags "$(FLAGS)" -buildmode=c-archive -v ./clientlibrary/clib

build-js-addon:
	@echo 'Building JS-addon...'
	@cp dist/lib.a clientlibrary/jsaddon/lib.a
	@cp dist/lib.h clientlibrary/jsaddon/lib.h
	@cp clientlibrary/clib/bridge.h clientlibrary/jsaddon/bridge.h
    # Electron's version.
	@export npm_config_target=6.0.10
	# The architecture of Electron, see https://electronjs.org/docs/tutorial/support#supported-platforms
	# for supported architectures.
	@export npm_config_arch=x64
	@export npm_config_target_arch=x64
	# Download headers for Electron.
	@export npm_config_disturl=https://electronjs.org/headers
	# Tell node-pre-gyp that we are building for Electron.
	@export npm_config_runtime=electron
	# Tell node-pre-gyp to build module from source code.
	@export npm_config_build_from_source=true
	@npm install -C ./clientlibrary/jsaddon
	@rm clientlibrary/jsaddon/lib.a clientlibrary/jsaddon/lib.h clientlibrary/jsaddon/bridge.h

build-ios: setup-go
	@echo 'Building library for iOS...'
	@$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/anytypeio\/go-anytype-middleware\/core/g'))
	@GOPRIVATE=github.com/anytypeio gomobile bind -tags nogrpcserver -ldflags "$(FLAGS)" -v -target=ios github.com/anytypeio/go-anytype-middleware/clientlibrary
	@mkdir -p dist/ios/ && mv Lib.framework dist/ios/

build-android: setup-go
	@echo 'Building library for Android...'
	@$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/anytypeio\/go-anytype-middleware\/core/g'))
	@GOPRIVATE=github.com/anytypeio gomobile bind -tags nogrpcserver -ldflags "$(FLAGS)" -v -target=android -o mobile.aar github.com/anytypeio/go-anytype-middleware/clientlibrary
	@mkdir -p dist/android/ && mv lib.aar dist/android/

setup-protoc-go:
	@echo 'Setting up protobuf compiler...'
	@rm -rf $(GOPATH)/src/github.com/gogo
	@mkdir -p $(GOPATH)/src/github.com/gogo
	@cd $(GOPATH)/src/github.com/gogo; git clone https://github.com/anytypeio/protobuf
	@cd $(GOPATH)/src/github.com/gogo/protobuf; go install github.com/gogo/protobuf/protoc-gen-gogofaster
	@cd $(GOPATH)/src/github.com/gogo/protobuf; go install github.com/gogo/protobuf/protoc-gen-gogofast

setup-protoc-jsweb:
	@echo 'Installing grpc-web plugin...'
	@git clone https://github.com/grpc/grpc-web
	@$(MAKE) -C grpc-web install-plugin
	@rm -rf grpc-web

setup-protoc-doc:
	@echo 'Setting up documentation plugin for protobuf compiler...'
	@go get -u github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc

setup-protoc: setup-protoc-go setup-protoc-jsweb

protos-server:
	@echo 'Generating protobuf packages for lib-server (Go)...'
	@$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	@$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 GOGO_GRPC_SERVER_METHOD_NO_ERROR=1 GOGO_GRPC_SERVER_METHOD_NO_CONTEXT=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=grpc:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./pb/service/

protos-go:
	@echo 'Generating protobuf packages for lib (Go)...'
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpkg/lib/pb/model/protos/localstore.proto=github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model)
	@$(eval P_PROTOS3 := Mpkg/lib/pb/relation/protos/relation.proto=github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation)

	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pkg/lib/pb/ pkg/lib/pb/model/protos/*.proto; mv pkg/lib/pb/pkg/lib/pb/model/*.go pkg/lib/pb/model/; rm -rf pkg/lib/pb/pkg
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pkg/lib/pb/ pkg/lib/pb/storage/protos/*.proto; mv pkg/lib/pb/pkg/lib/pb/storage/*.go pkg/lib/pb/storage/; rm -rf pkg/lib/pb/pkg
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pkg/lib/pb/ pkg/lib/pb/relation/protos/*.proto; mv pkg/lib/pb/pkg/lib/pb/relation/*.go pkg/lib/pb/relation/; rm -rf pkg/lib/pb/pkg
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP),plugins=grpc:./ pkg/lib/cafe/pb/*.proto
	@echo 'Generating protobuf packages for mw (Go)...'
	@$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	@$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc -I . --gogofaster_out=$(PKGMAP):. ./pb/protos/*.proto; mv ./pb/protos/*.pb.go ./pb/
	@$(eval P_PROTOS4 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval P_PROTOS5 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_PROTOS4),$$(P_PROTOS5))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./clientlibrary/service/
	@protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos-gomobile:
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./clientlibrary/service/

protos-docs:
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	@protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos: protos-go protos-server protos-docs

protos-swift:
	@echo 'Generating protobuf packages (Swift)...'
	@protoc -I ./  --swift_opt=FileNaming=DropPath --swift_opt=Visibility=Internal --swift_out=./dist/ios/pb pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos-js:
	@echo 'Generating protobuf packages (JS)...'
	@protoc -I ./  --js_out=import_style=commonjs,binary:./dist/js/pb pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto
	@protoc -I ./  --grpc-web_out=import_style=commonjs+dts,mode=grpcwebtext:./dist/js/pb pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos-java:
	@echo 'Generating protobuf packages (Java)...'
	@protoc -I ./ --java_out=./dist/android/pb pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

build-server: protos-server
	@echo 'Building middleware server...'
	@$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/core))
	@go build -i -v -o dist/server -ldflags "$(FLAGS)" ./cmd/grpcserver/grpc.go

build-server-debug: protos-server
	@echo 'Building middleware server with debug symbols...'
	@$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/core))
	@go build -v -o dist/server -gcflags "all=-N -l" -ldflags "$(FLAGS)" ./cmd/grpcserver/grpc.go

run-server: build-server
	@echo 'Running server...'
	@./dist/server

install-dev-js-addon: setup build-lib build-js-addon protos-js
	@echo 'Installing JS-addon (dev-mode)...'
	@cp -r clientlibrary/jsaddon/build ../js-anytype/
	@cp -r dist/js/pb/* ../js-anytype/dist/lib

install-dev-js: build-js
	@echo 'Installing JS-server (dev-mode)...'
	@cp -r dist/server ../js-anytype/dist/anytypeHelper
	@cp -r dist/js/pb/* ../js-anytype/dist/lib
	@cp -r dist/js/pb/* ../js-anytype/dist/lib
	@cp -R pkg/lib/schema/* ../js-anytype/src/json/schema
	@chmod -R 755 ../js-anytype/src/json/schema/*

build-js: setup-go build-server protos-js
	@echo "Run 'make install-dev-js' instead if you want to build&install into ../js-anytype"
