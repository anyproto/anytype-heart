build-ios: setup-go setup-gomobile
	# PATH is not working here, so we need to use absolute path
	$(DEPS_PATH)/gomobile init
	@echo 'Clear xcframework'
	@rm -rf ./dist/ios/Lib.xcframework
	@echo 'Building library for iOS...'
	@$(eval FLAGS += $$(shell govvv -flags | sed 's/main/github.com\/anyproto\/anytype-heart\/util\/vcs/g'))
	@$(eval TAGS := nogrpcserver gomobile nowatchdog nosigar timetzdata rasterizesvg)
ifdef ANY_SYNC_NETWORK
	@$(eval TAGS := $(TAGS) envnetworkcustom)
endif
	gomobile bind -tags "$(TAGS)" -ldflags "$(FLAGS)" $(BUILD_FLAGS) -target=ios -o Lib.xcframework github.com/anyproto/anytype-heart/clientlibrary/service github.com/anyproto/anytype-heart/core
	@mkdir -p dist/ios/ && mv Lib.xcframework dist/ios/
	@mkdir -p dist/ios/json/
	@cp pkg/lib/bundle/system*.json dist/ios/json/
	@cp pkg/lib/bundle/relations.json dist/ios/json/
	@cp pkg/lib/bundle/internal*.json dist/ios/json/
	@go mod tidy
	@echo 'Repacking iOS framework...'
	chmod -R 755 dist/ios/Lib.xcframework
	@go run cmd/iosrepack/main.go

install-dev-ios: setup-go build-ios protos-swift
	@echo 'Installing iOS framework locally at $(CLIENT_IOS_PATH)...'
	@chmod -R 755 $(CLIENT_IOS_PATH)/Dependencies/Middleware/Lib.xcframework
	@rm -rf $(CLIENT_IOS_PATH)/Dependencies/Middleware/*
	@cp -r dist/ios/Lib.xcframework $(CLIENT_IOS_PATH)/Dependencies/Middleware
	@rm -rf $(CLIENT_IOS_PATH)/Modules/ProtobufMessages/Sources/Protocol/*
	@cp -r dist/ios/protobuf/*.swift $(CLIENT_IOS_PATH)/Modules/ProtobufMessages/Sources/Protocol
	@mkdir -p $(CLIENT_IOS_PATH)/Dependencies/Middleware/protobuf/protos
	@cp -r pb/protos/*.proto $(CLIENT_IOS_PATH)/Dependencies/Middleware/protobuf/protos
	@cp -r pb/protos/service/*.proto $(CLIENT_IOS_PATH)/Dependencies/Middleware/protobuf/protos
	@cp -r pkg/lib/pb/model/protos/*.proto $(CLIENT_IOS_PATH)/Dependencies/Middleware/protobuf/protos
	@mkdir -p $(CLIENT_IOS_PATH)/Dependencies/Middleware/json
	@cp -r pkg/lib/bundle/*.json $(CLIENT_IOS_PATH)/Dependencies/Middleware/json
