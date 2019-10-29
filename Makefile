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

build-lib:
	$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-middleware/lib))
	export GO111MODULE=on
	go build -o dist/lib.a -ldflags "$(FLAGS) -w -s" -buildmode=c-archive -v ./lib/clib

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
	env go111module=off gomobile bind -ldflags "-w $(FLAGS)" -v -target=ios github.com/anytypeio/go-anytype-middleware/lib
	mkdir -p dist/ios/ && cp -r Mobile.framework mobile/dist/ios/
	rm -rf Mobile.framework

build-android:
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/anytypeio\/go-anytype-middleware\/lib/g'))
	env go111module=off gomobile bind -ldflags "-w $(FLAGS)" -v -target=android -o mobile.aar github.com/anytypeio/go-anytype-middleware/lib
	mkdir -p dist/android/ && mv mobile.aar mobile/dist/android/

setup-protoc:
	rm -rf $(GOPATH)/src/github.com/gogo/protobuf
	mkdir -p $(GOPATH)/src/github.com/gogo
	cd $(GOPATH)/src/github.com/gogo
	git clone https://github.com/anytypeio/protobuf
	cd protobuf
	go install github.com/gogo/protobuf/protoc-gen-gogofaster
	export PATH=$PATH:$GOROOT/bin:$GOPATH/bin

protos:
	cd pb/protos; protoc --gogofaster_out=plugins=gomobile:.. *.proto
	cd pb/protos/service; env PACKAGE_PATH=github.com/anytypeio/go-anytype-middleware/pb protoc -I=.. -I=. --gogofaster_out=plugins=gomobile:../../../lib service.proto
