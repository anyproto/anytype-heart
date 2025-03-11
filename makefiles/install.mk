install-dev-js-addon: setup build-lib build-js-addon protos-js
	@echo 'Installing JS-addon (dev-mode) in ${CLIENT_DESKTOP_PATH}...'
	@rm -rf $(CLIENT_DESKTOP_PATH)/build
	@cp -r clientlibrary/jsaddon/build $(CLIENT_DESKTOP_PATH)/
	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/dist/lib

install-dev-js: setup-go build-server protos-js
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
