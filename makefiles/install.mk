ifeq ($(OS),Windows_NT)
  BINARY_EXT := .exe
endif

define install-binary
	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_BIN_DIR)
	@rm -f $(CLIENT_DESKTOP_PATH)/$(CLIENT_BIN_DIR)/anytypeHelper$(BINARY_EXT)
	@cp -r dist/server $(CLIENT_DESKTOP_PATH)/$(CLIENT_BIN_DIR)/anytypeHelper$(BINARY_EXT)
endef

define install-protos
	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@cp -r $(PROTO_JS_OUT)/* $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)
	@mkdir -p $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/system*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
	@cp pkg/lib/bundle/internal*.json $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)/json/generated
endef

# Usage: make install-dev-js [CLIENT_DESKTOP_PATH=../anytype-bun] [CLIENT_LIB_DIR=middleware] [CLIENT_BIN_DIR=dist]
install-dev-js: setup-go build-server protos-js
	@echo 'Installing JS-server (dev-mode) in $(CLIENT_DESKTOP_PATH)...'
	$(install-binary)
	$(install-protos)
	@echo 'Installed to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)'

# Usage: make install-dev-ts [CLIENT_DESKTOP_PATH=../anytype-bun] [CLIENT_LIB_DIR=middleware] [CLIENT_BIN_DIR=dist]
install-dev-ts: setup-go build-server protos-ts
	@echo 'Installing TS-server (dev-mode) in $(CLIENT_DESKTOP_PATH)...'
	$(install-binary)
	$(install-protos)
	@echo 'Installed to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)'

# Install only generated protos (use CLIENT_DESKTOP_PATH and CLIENT_LIB_DIR to override)
install-protos-js: protos-js
	@echo 'Installing generated protos to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)...'
	$(install-protos)
	@echo 'Protos installed successfully to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)'

install-protos-ts: protos-ts
	@echo 'Installing generated protos to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)...'
	$(install-protos)
	@echo 'Protos installed successfully to $(CLIENT_DESKTOP_PATH)/$(CLIENT_LIB_DIR)'
