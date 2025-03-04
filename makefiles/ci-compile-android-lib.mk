compile-android-lib:
	gomobile bind -tags "$(BUILD_TAG_NETWORK) nogrpcserver gomobile nowatchdog nosigar nomutexdeadlockdetector timetzdata rasterizesvg" -ldflags "$(FLAGS)" -v -target=android -androidapi 26 -o lib.aar github.com/anyproto/anytype-heart/clientlibrary/service github.com/anyproto/anytype-heart/core || true
	gtar --exclude ".*" -czvf android_lib_$(VERSION).tar.gz lib.aar protobuf json
	mv android_lib_$(VERSION).tar.gz .release/
