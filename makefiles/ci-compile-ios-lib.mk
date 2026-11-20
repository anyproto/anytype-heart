PUBLISH_GRADLE ?= 0

# build-ios-lib leaves the repacked Lib.xcframework in the working directory. The release build runs
# it on its own and packs the framework later; compile-ios-lib also packs it, and publishes it to
# Maven when PUBLISH_GRADLE is 1.
build-ios-lib:
	go install github.com/vektra/mockery/v2@v2.53.6
	go install go.uber.org/mock/mockgen@v0.6.0
	make test-deps
	gomobile bind -tags "$(BUILD_TAG_NETWORK) nogrpcserver gomobile nowatchdog nosigar nomutexdeadlockdetector timetzdata rasterizesvg" -ldflags "$(FLAGS)" -v -target=ios/arm64,iossimulator/arm64 -o Lib.xcframework github.com/anyproto/anytype-heart/clientlibrary/service github.com/anyproto/anytype-heart/core || true
	mkdir -p dist/ios/
	mv Lib.xcframework dist/ios/
	go run cmd/iosrepack/main.go
	mv dist/ios/Lib.xcframework .

compile-ios-lib: build-ios-lib
	gtar --exclude ".*" -czvf ios_framework.tar.gz Lib.xcframework protobuf json
	@if [ "$(PUBLISH_GRADLE)" -eq 1 ]; then \
		gradle publish; \
	fi
	mv ios_framework.tar.gz .release/ios_framework_$(VERSION).tar.gz
