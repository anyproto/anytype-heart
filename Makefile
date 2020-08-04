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
.PHONY : protos-deps
setup: setup-go
	npm install
uuu:
	echo $(PATH)
setup-go:
	go mod download
	GO111MODULE=off go get github.com/ahmetb/govvv
	GO111MODULE=off go get golang.org/x/mobile/cmd/...

fmt:
	echo 'Formatting with prettier...'
	npx prettier --write "./**" 2> /dev/null || true
	echo 'Formatting with goimports...'
	goimports -w -l `find . -type f -name '*.go' -not -path './vendor/*'`

lint:
	echo 'Linting with prettier...'
	npx prettier --check "./**" 2> /dev/null || true
	echo 'Linting with golint...'
	golint `go list ./... | grep -v /vendor/`

test:
	go test -cover github.com/anytypeio/go-anytype-middleware/...

test-race:
	go test -race github.com/anytypeio/go-anytype-middleware/...

test-deps:
	go install github.com/golang/mock/mockgen
	go generate ./...

build-lib:
	$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/core))
	GO111MODULE=on go build -v -o dist/lib.a -tags nogrpcserver -ldflags "$(FLAGS)" -buildmode=c-archive -v ./lib/clib

build-js-addon:
	cp dist/lib.a jsaddon/lib.a
	cp dist/lib.h jsaddon/lib.h
	cp lib/clib/bridge.h jsaddon/bridge.h
    # Electron's version.
	export npm_config_target=6.0.10
	# The architecture of Electron, see https://electronjs.org/docs/tutorial/support#supported-platforms
	# for supported architectures.
	export npm_config_arch=x64
	export npm_config_target_arch=x64
	# Download headers for Electron.
	export npm_config_disturl=https://electronjs.org/headers
	# Tell node-pre-gyp that we are building for Electron.
	export npm_config_runtime=electron
	# Tell node-pre-gyp to build module from source code.
	export npm_config_build_from_source=true
	npm install -C ./jsaddon
	rm jsaddon/lib.a jsaddon/lib.h jsaddon/bridge.h

build-ios: setup-go
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/anytypeio\/go-anytype-middleware\/core/g'))
	GOPRIVATE=github.com/anytypeio gomobile bind -tags nogrpcserver -ldflags "$(FLAGS)" -v -target=ios github.com/anytypeio/go-anytype-middleware/lib
	mkdir -p dist/ios/ && mv Lib.framework dist/ios/

build-android: setup-go
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/anytypeio\/go-anytype-middleware\/core/g'))
	GOPRIVATE=github.com/anytypeio gomobile bind -tags nogrpcserver -ldflags "$(FLAGS)" -v -target=android -o mobile.aar github.com/anytypeio/go-anytype-middleware/lib
	mkdir -p dist/android/ && mv lib.aar dist/android/

setup-protoc-go:
	rm -rf $(GOPATH)/src/github.com/gogo
	mkdir -p $(GOPATH)/src/github.com/gogo
	cd $(GOPATH)/src/github.com/gogo; git clone https://github.com/anytypeio/protobuf
	cd $(GOPATH)/src/github.com/gogo/protobuf; go install github.com/gogo/protobuf/protoc-gen-gogofaster
	cd $(GOPATH)/src/github.com/gogo/protobuf; go install github.com/gogo/protobuf/protoc-gen-gogofast

setup-protoc-jsweb:
	git clone https://github.com/grpc/grpc-web
	$(MAKE) -C grpc-web install-plugin
	rm -rf grpc-web

setup-protoc-doc:
	go get -u github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc

setup-protoc: setup-protoc-go setup-protoc-jsweb

protos-deps:
	$(eval LIBRARY_PATH = $(shell go list -m -json all | jq -r 'select(.Path == "github.com/anytypeio/go-anytype-library") | .Dir'))
	mkdir -p vendor/github.com/anytypeio/go-anytype-library/
	cp -R $(LIBRARY_PATH)/pb vendor/github.com/anytypeio/go-anytype-library/
	cp -R $(LIBRARY_PATH)/schema vendor/github.com/anytypeio/go-anytype-library/

	chmod -R 755 ./vendor/github.com/anytypeio/go-anytype-library/

protos-server: protos-deps
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	$(eval P_PROTOS := Mvendor/github.com/anytypeio/go-anytype-library/pb/model/protos/models.proto=github.com/anytypeio/go-anytype-library/pb/model)
	$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 GOGO_GRPC_SERVER_METHOD_NO_ERROR=1 GOGO_GRPC_SERVER_METHOD_NO_CONTEXT=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=grpc:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./lib-server/

protos-go: protos-deps
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	$(eval P_PROTOS := Mvendor/github.com/anytypeio/go-anytype-library/pb/model/protos/models.proto=github.com/anytypeio/go-anytype-library/pb/model)
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc -I . --gogofaster_out=$(PKGMAP):. ./pb/protos/*.proto; mv ./pb/protos/*.pb.go ./pb/
	$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./lib/
	protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/*.proto

protos: protos-go protos-server
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	$(eval P_PROTOS := Mvendor/github.com/anytypeio/go-anytype-library/pb/model/protos/models.proto=github.com/anytypeio/go-anytype-library/pb/model)
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc -I . --gogofaster_out=$(PKGMAP):. ./pb/protos/*.proto; mv ./pb/protos/*.pb.go ./pb/
	$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anytypeio/go-anytype-middleware/pb)
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./lib/
	protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/*.proto

protos-swift: protos-deps
	protoc -I ./  --swift_opt=FileNaming=DropPath --swift_opt=Visibility=Internal --swift_out=./dist/ios/pb pb/protos/*.proto vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/*.proto

protos-js: protos-deps
	protoc -I ./  --js_out=import_style=commonjs,binary:./dist/js/pb pb/protos/service/*.proto pb/protos/*.proto vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/*.proto
	protoc -I ./  --grpc-web_out=import_style=commonjs+dts,mode=grpcwebtext:./dist/js/pb pb/protos/service/*.proto pb/protos/*.proto vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/*.proto

protos-java: protos-deps
	protoc -I ./ --java_out=./dist/android/pb pb/protos/*.proto vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/*.proto

build-server: protos-server
	$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/core))
	go build -i -v -o dist/server -ldflags "$(FLAGS)" ./lib-server/server/grpc.go

build-server-debug: protos-server
	$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/core))
	go build -v -o dist/server -gcflags "all=-N -l" -ldflags "$(FLAGS)" ./lib-server/server/grpc.go

run-server: build-server
	./dist/server

install-dev-js-addon: setup build-lib build-js-addon protos-js
	cp -r jsaddon/build ../js-anytype/
	cp -r dist/js/pb/* ../js-anytype/dist/lib

build-js: setup-go build-server protos-js
	echo "Run 'make install-dev-js' insted if you want to build&install into ../js-anytype"

install-dev-js: build-js
	cp -r dist/server ../js-anytype/dist/anytypeHelper
	cp -r dist/js/pb/* ../js-anytype/dist/lib
	cp -r dist/js/pb/* ../js-anytype/dist/lib
	$(eval LIBRARY_PATH = $(shell go list -m -json all | jq -r 'select(.Path == "github.com/anytypeio/go-anytype-library") | .Dir'))
	cp -R $(LIBRARY_PATH)/schema/* ../js-anytype/src/json/schema
	chmod -R 755 ../js-anytype/src/json/schema/*
