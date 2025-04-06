PUBLISH_GRADLE ?= 0
compile-ios-lib:
	go install github.com/vektra/mockery/v2@v2.47.0
	go install go.uber.org/mock/mockgen@v0.5.0
	make test-deps
	gomobile bind -tags "$(BUILD_TAG_NETWORK) nogrpcserver gomobile nowatchdog nosigar nomutexdeadlockdetector timetzdata rasterizesvg" -ldflags "$(FLAGS)" -v -target=ios -o Lib.xcframework github.com/anyproto/anytype-heart/clientlibrary/service github.com/anyproto/anytype-heart/core || true
	mkdir -p dist/ios/
	mv Lib.xcframework dist/ios/
	go run cmd/iosrepack/main.go
	mv dist/ios/Lib.xcframework .
	gtar --exclude ".*" -czvf ios_framework.tar.gz Lib.xcframework protobuf json
	@if [ "$(PUBLISH_GRADLE)" -eq 1 ]; then \
		gradle publish; \
	fi
	mv ios_framework.tar.gz .release/ios_framework_$(VERSION).tar.gz
