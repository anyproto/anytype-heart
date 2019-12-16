.PHONY : protos_deps
setup:
	GOPRIVATE=github.com/anytypeio/go-anytype-library go mod download
	go get github.com/ahmetb/govvv
	npm install

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
	go test github.com/anytypeio/go-anytype-middleware/...

fast-test:
	go test github.com/anytypeio/go-anytype-middleware/core/block/... -cover

test-deps:
	go install github.com/golang/mock/mockgen
	go generate ./...

build-lib:
	$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/lib))
	export GO111MODULE=on
	go build -o dist/lib.a -ldflags "$(FLAGS)" -buildmode=c-archive -v ./lib/clib

build-js:
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

build-ios:
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/anytypeio\/go-anytype-middleware\/lib/g'))
	env go111module=off gomobile bind -ldflags "$(FLAGS)" -v -target=ios github.com/anytypeio/go-anytype-middleware/lib
	mkdir -p dist/ios/ && cp -r Mobile.framework dist/ios/
	rm -rf Mobile.framework

build-android:
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/anytypeio\/go-anytype-middleware\/lib/g'))
	env go111module=off gomobile bind -ldflags "$(FLAGS)" -v -target=android -o mobile.aar github.com/anytypeio/go-anytype-middleware/lib
	mkdir -p dist/android/ && mv mobile.aar dist/android/

setup-protoc:
	rm -rf $(GOPATH)/src/github.com/gogo
	mkdir -p $(GOPATH)/src/github.com/gogo
	cd $(GOPATH)/src/github.com/gogo; git clone https://github.com/anytypeio/protobuf
	cd $(GOPATH)/src/github.com/gogo/protobuf; go install github.com/gogo/protobuf/protoc-gen-gogofaster
	cd $(GOPATH)/src/github.com/gogo/protobuf; go install github.com/gogo/protobuf/protoc-gen-gogofast
	cd $(GOPATH); go get -u github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc
	export PATH=$(PATH):$(GOROOT)/bin:$(GOPATH)/bin

protos_deps:
	$(eval LIBRARY_PATH = $(shell go list -m -json all | jq -r 'select(.Path == "github.com/anytypeio/go-anytype-library") | .Dir'))
	mkdir -p vendor/github.com/anytypeio/go-anytype-library/
	cp -R $(LIBRARY_PATH)/pb vendor/github.com/anytypeio/go-anytype-library/


protos: protos_deps
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types)
	$(eval P_PROTOS := Mvendor/github.com/anytypeio/go-anytype-library/pb/model/protos/models.proto=github.com/anytypeio/go-anytype-library/pb/model)
	$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anytypeio/go-anytype-middleware/pb)
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc -I . --gogofaster_out=$(PKGMAP):. ./pb/protos/*.proto; mv ./pb/protos/*.pb.go ./pb/
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./lib/
	protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/*.proto

protos-swift:
	protoc -I ./  --swift_opt=FileNaming=DropPath --swift_opt=Visibility=Internal --swift_out=./build/swift pb/protos/* vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/*

protos-java:
	protoc -I ./ --java_out=./protobuf pb/protos/* vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/*.proto

protos-ts:
	npm run build:ts

build-dev-js:
	go mod download
	make build-lib build-js
	npm run build:ts
	cp -r jsaddon/build ../js-anytype/
	cp build/ts/commands.js ../js-anytype/electron/proto/commands.js

