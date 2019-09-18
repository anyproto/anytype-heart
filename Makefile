setup:
	go mod download
	go get github.com/ahmetb/govvv
	npm install

test:
	./test_compile

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

build:
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/textileio\/go-textile\/common/g'))
	go build -ldflags "-w $(FLAGS)" -i -o textile textile.go
	mkdir -p dist
	mv textile dist/

build-debug:
	$(eval FLAGS := $$(shell govvv -flags -pkg github.com/requilence/go-anytype/common))
	go build -ldflags "$(FLAGS)" -gcflags "all=-N -l"  -i -o textile textile.go
	mkdir -p dist
	mv textile dist/

sign:
	codesign --force --sign  "Developer ID Application: Smovie LTD (789H935J7W)" dist/textile

install:
	mv dist/textile $$GOPATH/bin

ios:
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/textileio\/go-textile\/common/g'))
	env go111module=off gomobile bind -ldflags "-w $(FLAGS)" -v -target=ios github.com/requilence/go-anytype/mobile github.com/requilence/go-anytype/core
	mkdir -p mobile/dist/ios/ && cp -r Mobile.framework mobile/dist/ios/
	rm -rf Mobile.framework

android:
	$(eval FLAGS := $$(shell govvv -flags | sed 's/main/github.com\/textileio\/go-textile\/common/g'))
	env go111module=off gomobile bind -ldflags "-w $(FLAGS)" -v -target=android -o mobile.aar github.com/requilence/go-anytype/mobile github.com/requilence/go-anytype/core
	mkdir -p mobile/dist/android/ && mv mobile.aar mobile/dist/android/

protos:
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=github.com/golang/protobuf/ptypes/timestamp)
	$(eval P_ANY := Mgoogle/protobuf/any.proto=github.com/golang/protobuf/ptypes/any)
	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_ANY))
	cd pb/protos; protoc --go_out=$(PKGMAP):.. *.proto

protos_js:
	rm -rf mobile/dist
	mkdir mobile/dist
	cd mobile; rm -rf node_modules && npm i @textile/protobufjs --no-save
	cd mobile; ./node_modules/.bin/pbjs -t static-module -o dist/index.js ../pb/protos/*
	cd mobile; ./node_modules/.bin/pbts -o dist/index.d.ts dist/index.js
