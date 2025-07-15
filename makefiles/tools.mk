fmt:
	@echo 'Formatting with prettier...'
	@npx prettier --write "./**" 2> /dev/null || true
	@echo 'Formatting with goimports...'
	@goimports -w -l `find . -type f -name '*.go' -not -path './vendor/*'`

lint:
	@echo 'Linting with prettier...'
	@npx prettier --check "./**" 2> /dev/null || true
	@echo 'Linting with golint...'
	@golint `go list ./... | grep -v /vendor/`

openapi:
	@echo 'Generating openapi docs...'
	@swag init --v3.1 -q -d core/api -g service.go -o $(OPENAPI_DOCS_DIR)
	@mv $(OPENAPI_DOCS_DIR)/swagger.yaml $(OPENAPI_DOCS_DIR)/openapi.yaml
	@mv $(OPENAPI_DOCS_DIR)/swagger.json $(OPENAPI_DOCS_DIR)/openapi.json
	@jq . "$(OPENAPI_DOCS_DIR)/openapi.json" > "$(OPENAPI_DOCS_DIR)/pretty.json" && mv "$(OPENAPI_DOCS_DIR)/pretty.json" "$(OPENAPI_DOCS_DIR)/openapi.json"
	@echo 'Formatting openapi docs...'
	@swag fmt -d core/api