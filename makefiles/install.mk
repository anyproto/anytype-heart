install-dev-js:  build-server protos-js
	@echo 'Installing JS-server (dev-mode) in $(CLIENT_DESKTOP_PATH)...'
	@rm -f $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper

ifeq ($(OS),Windows_NT)
	@cp -r dist/server $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper.exe
else
	@cp -r dist/server $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper
endif

	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/dist/lib
	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/dist/lib
	@mkdir -p $(CLIENT_DESKTOP_PATH)/dist/lib/json/generated
	@cp pkg/lib/bundle/system*.json $(CLIENT_DESKTOP_PATH)/dist/lib/json/generated
	@cp pkg/lib/bundle/internal*.json $(CLIENT_DESKTOP_PATH)/dist/lib/json/generated

# Install only generated protos to anytype-ts (use CLIENT_DESKTOP_PATH=/path/to/anytype-ts to override)
install-protos-ts: protos-js
	@echo 'Installing generated protos to $(CLIENT_DESKTOP_PATH)...'
	@mkdir -p $(CLIENT_DESKTOP_PATH)/dist/lib
	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/dist/lib
	@mkdir -p $(CLIENT_DESKTOP_PATH)/dist/lib/json/generated
	@cp pkg/lib/bundle/system*.json $(CLIENT_DESKTOP_PATH)/dist/lib/json/generated
	@cp pkg/lib/bundle/internal*.json $(CLIENT_DESKTOP_PATH)/dist/lib/json/generated
	@echo 'Protos installed successfully to $(CLIENT_DESKTOP_PATH)'
