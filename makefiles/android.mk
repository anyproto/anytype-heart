build-android: setup-go setup-gomobile
	$(DEPS_PATH)/gomobile init
	@echo 'Building library for Android...'
	@$(eval FLAGS += $$(shell govvv -flags | sed 's/main/github.com\/anyproto\/anytype-heart\/util\/vcs/g'))
	@$(eval TAGS := nogrpcserver gomobile nowatchdog nosigar timetzdata rasterizesvg)
ifdef ANY_SYNC_NETWORK
	@$(eval TAGS := $(TAGS) envnetworkcustom)
endif
	gomobile bind -tags "$(TAGS)" -ldflags "$(FLAGS)" $(BUILD_FLAGS) -target=android -androidapi 26 -o lib.aar github.com/anyproto/anytype-heart/clientlibrary/service github.com/anyproto/anytype-heart/core
	@mkdir -p dist/android/ && mv lib.aar dist/android/
	@go mod tidy

install-dev-android: setup-go build-android
	@echo 'Installing android lib locally in $(CLIENT_ANDROID_PATH)...'
	@rm -f $(CLIENT_ANDROID_PATH)/libs/lib.aar
	@cp -r dist/android/lib.aar $(CLIENT_ANDROID_PATH)/libs/lib.aar
	@cp -r pb/protos/*.proto $(CLIENT_ANDROID_PATH)/protocol/src/main/proto
	@cp -r pkg/lib/pb/model/protos/*.proto $(CLIENT_ANDROID_PATH)/protocol/src/main/proto
	# Compute the SHA hash of lib.aar
	@$(eval hash := $$(shell shasum -b dist/android/lib.aar | cut -d' ' -f1))
	@echo "Version hash: ${hash}"
	# Update the gradle file with the new version
ifeq ($(shell uname),Darwin)
	@sed -i '' "s/version = '.*'/version = '${hash}'/g" $(CLIENT_ANDROID_PATH)/libs/build.gradle
	@sed -i '' "s/middlewareVersion = \".*\"/middlewareVersion = \"${hash}\"/" $(CLIENT_ANDROID_PATH)/gradle/libs.versions.toml
else
	@sed -i "s/version = '.*'/version = '${hash}'/g" $(CLIENT_ANDROID_PATH)/libs/build.gradle
	@sed -i "s/middlewareVersion = \".*\"/middlewareVersion = \"${hash}\"/" $(CLIENT_ANDROID_PATH)/gradle/libs.versions.toml
endif
	@cat $(CLIENT_ANDROID_PATH)/libs/build.gradle
	@cat $(CLIENT_ANDROID_PATH)/gradle/libs.versions.toml
