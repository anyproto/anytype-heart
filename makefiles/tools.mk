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
	@echo 'Removing package prefixes from definitions...'
	@jq '.components.schemas |= with_entries(.key |= (gsub("apimodel\\."; "") | gsub("apimodel_"; "") | gsub("pagination\\."; "") | gsub("pagination_"; "") | gsub("util\\."; "") | gsub("util_"; ""))) | walk(if type == "string" then (gsub("apimodel\\."; "") | gsub("apimodel_"; "") | gsub("pagination\\."; "") | gsub("pagination_"; "") | gsub("util\\."; "") | gsub("util_"; "")) else . end)' "$(OPENAPI_DOCS_DIR)/swagger.json" > "$(OPENAPI_DOCS_DIR)/openapi.json" && rm "$(OPENAPI_DOCS_DIR)/swagger.json"
	@sed -i '' 's/apimodel[._]//g; s/pagination[._]//g; s/util[._]//g' "$(OPENAPI_DOCS_DIR)/swagger.yaml" && mv "$(OPENAPI_DOCS_DIR)/swagger.yaml" "$(OPENAPI_DOCS_DIR)/openapi.yaml"
	@echo 'Formatting openapi docs...'
	@swag fmt -d core/api