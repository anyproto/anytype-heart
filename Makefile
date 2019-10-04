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
	go test github.com/anytypeio/go-anytype-middleware/lib

build-lib:
	$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-library/common))
	export GO111MODULE=on
	go build -o dist/lib.so -ldflags "$(FLAGS)" -buildmode=c-archive -v ./lib/clib

build-js:
	$(eval FLAGS := $$(shell govvv -flags -pkg github.com/anytypeio/go-anytype-library/common))
	cp dist/lib.so jsaddon/lib.so
	cp dist/lib.h jsaddon/lib.h
	cp lib/clib/bridge.h jsaddon/bridge.h
	cd jsaddon
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
	npm install
	cd ..

ios:
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/textileio\/go-textile\/common/g'))
	env go111module=off gomobile bind -ldflags "-w $(FLAGS)" -v -target=ios github.com/anytypeio/go-anytype-library/lib github.com/anytypeio/go-anytype-library/core
	mkdir -p dist/ios/ && cp -r Mobile.framework mobile/dist/ios/
	rm -rf Mobile.framework

android:
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/textileio\/go-textile\/common/g'))
	env go111module=off gomobile bind -ldflags "-w $(FLAGS)" -v -target=android -o mobile.aar github.com/anytypeio/go-anytype-library/lib github.com/anytypeio/go-anytype-library/core
	mkdir -p dist/android/ && mv mobile.aar mobile/dist/android/

protos:
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/golang/protobuf/ptypes/timestamp)
	$(eval P_ANY := Mgoogle/protobuf/any.proto=github.com/golang/protobuf/ptypes/any)
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_ANY))
	cd pb/protos; protoc --go_out=$(PKGMAP):.. *.proto
