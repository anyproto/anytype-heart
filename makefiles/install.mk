# Usage: make install-dev-js [CLIENT_DESKTOP_PATH=../anytype-bun] [CLIENT_LIB_DIR=middleware]
install-dev-js: setup-go build-server protos-js
	@echo 'Installing JS-server (dev-mode) in $(CLIENT_DESKTOP_PATH)...'
	@mkdir -p $(CLIENT_DESKTOP_PATH)/dist
	@rm -f $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper

ifeq ($(OS),Windows_NT)
	@cp -r dist/server $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper.exe
else
	@cp -r dist/server $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper
endif

	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/system*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/internal*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@echo 'Installed to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)'

# Usage: make install-dev-ts [CLIENT_DESKTOP_PATH=../anytype-bun] [CLIENT_LIB_DIR=middleware]
install-dev-ts: setup-go build-server protos-ts
	@echo 'Installing TS-server (dev-mode) in $(CLIENT_DESKTOP_PATH)...'
	@mkdir -p $(CLIENT_DESKTOP_PATH)/dist
	@rm -f $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper

ifeq ($(OS),Windows_NT)
	@cp -r dist/server $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper.exe
else
	@cp -r dist/server $(CLIENT_DESKTOP_PATH)/dist/anytypeHelper
endif

	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/system*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/internal*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@echo 'Installed to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)'

# Install only generated protos (use CLIENT_DESKTOP_PATH and CLIENT_LIB_DIR to override)
install-protos-js: protos-js
	@echo 'Installing generated protos to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)...'
	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/system*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/internal*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@echo 'Protos installed successfully to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)'

install-protos-ts: protos-ts
	@echo 'Installing generated protos to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)...'
	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@cp -r dist/js/pb/* $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/system*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/internal*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@echo 'Protos installed successfully to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)'
