install-dev-cli: setup-go build-server
	@echo 'Installing CLI server (dev-mode) in $(CLIENT_CLI_PATH)...'
	@rm -rf $(CLIENT_CLI_PATH)/dist/server
	@cp -r dist/server $(CLIENT_CLI_PATH)/dist/server
